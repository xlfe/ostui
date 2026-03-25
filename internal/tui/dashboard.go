package tui

import (
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	pb "github.com/safedoor/ostui/proto/protocol"

	"github.com/safedoor/ostui/internal/db"
	"github.com/safedoor/ostui/internal/tui/components"
)

var nixStoreRe = regexp.MustCompile(`^/nix/store/[a-z0-9]+-([^/]+)/`)

func extractProcessName(path string) string {
	if path == "" {
		return path
	}
	if strings.HasPrefix(path, "/nix/store/") {
		if idx := strings.LastIndex(path, "/"); idx >= 0 && idx < len(path)-1 {
			return path[idx+1:]
		}
		if m := nixStoreRe.FindStringSubmatch(path); len(m) > 1 {
			return m[1]
		}
	}
	if idx := strings.LastIndex(path, "/"); idx >= 0 && idx < len(path)-1 {
		return path[idx+1:]
	}
	return path
}

const maxRecentEvents = 500

const (
	topNProcesses = iota
	topNDestinations
	topNPorts
	topNCount
)

var topNLabels = [topNCount]string{"Top Processes", "Top Destinations", "Top Ports"}

type dashboardModel struct {
	width, height int
	stats         *pb.Statistics

	recentEvents []*pb.Event
	seenNanos    map[int64]struct{}
	scrollOffset int
	topNPage     int
	topNScroll   int
	groupWindow  int
	tickCount    int // increments each second for marquee scrolling
	selectedConn int // cursor in grouped connections table
	cachedGroups []connGroup // cached for selection
}

func newDashboardModel(groupWindow int) *dashboardModel {
	if groupWindow <= 0 {
		groupWindow = 60
	}
	return &dashboardModel{
		seenNanos:   make(map[int64]struct{}),
		groupWindow: groupWindow,
	}
}

// loadFromDB seeds the recent events from the database on startup.
// Temporarily widens the group window if needed so loaded events are visible.
func (m *dashboardModel) loadFromDB(database *db.DB) {
	rows, err := database.GetConnections(maxRecentEvents)
	if err != nil {
		log.Printf("ERROR loadFromDB: %v", err)
		return
	}
	log.Printf("Loaded %d connections from DB", len(rows))
	// Rows come newest-first; we want oldest-first in our slice.
	for i := len(rows) - 1; i >= 0; i-- {
		r := rows[i]
		dstPort := parseUint32(r.DstPort)
		srcPort := parseUint32(r.SrcPort)
		uid := parseUint32(r.UID)
		pid := parseUint32(r.PID)

		// Try multiple time formats the DB might have.
		var t time.Time
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
			if parsed, err := time.Parse(layout, r.Time); err == nil {
				t = parsed
				break
			}
		}

		// Use index as sub-second discriminator to avoid dedup collisions.
		nano := t.UnixNano() + int64(i)

		ev := &pb.Event{
			Time:     r.Time,
			Unixnano: nano,
			Connection: &pb.Connection{
				Protocol:    r.Protocol,
				SrcIp:       r.SrcIP,
				SrcPort:     srcPort,
				DstIp:       r.DstIP,
				DstHost:     r.DstHost,
				DstPort:     dstPort,
				UserId:      uid,
				ProcessId:   pid,
				ProcessPath: r.Process,
				ProcessArgs: strings.Fields(r.ProcessArgs),
				ProcessCwd:  r.ProcessCwd,
			},
			Rule: &pb.Rule{
				Name:   r.Rule,
				Action: r.Action,
			},
		}
		m.recentEvents = append(m.recentEvents, ev)
	}

	// If we loaded events, ensure the group window covers them.
	if len(m.recentEvents) > 0 {
		oldest := m.recentEvents[0]
		age := time.Since(time.Unix(0, oldest.Unixnano))
		ageSec := int(age.Seconds()) + 10 // small buffer
		if ageSec > m.groupWindow {
			// Snap to the smallest preset that covers the data.
			for _, w := range groupWindows {
				if w >= ageSec {
					m.groupWindow = w
					break
				}
			}
			// If none big enough, use the actual age.
			if m.groupWindow < ageSec {
				m.groupWindow = ageSec
			}
		}
	}
}

func parseUint32(s string) uint32 {
	var v uint32
	fmt.Sscanf(s, "%d", &v)
	return v
}

var groupWindows = []int{60, 300, 3600} // 60s, 5m, 60m

func (m *dashboardModel) cycleGroupWindow() {
	for i, w := range groupWindows {
		if m.groupWindow == w {
			m.groupWindow = groupWindows[(i+1)%len(groupWindows)]
			return
		}
	}
	m.groupWindow = groupWindows[0]
}

func (m *dashboardModel) groupWindowLabel() string {
	switch m.groupWindow {
	case 60:
		return "1m"
	case 300:
		return "5m"
	case 3600:
		return "60m"
	default:
		return fmt.Sprintf("%ds", m.groupWindow)
	}
}

func (m *dashboardModel) updateStats(stats *pb.Statistics) {
	m.stats = stats
	for _, ev := range stats.Events {
		if ev == nil {
			continue
		}
		if _, seen := m.seenNanos[ev.Unixnano]; seen {
			continue
		}
		m.seenNanos[ev.Unixnano] = struct{}{}
		m.recentEvents = append(m.recentEvents, ev)
	}
	if len(m.recentEvents) > maxRecentEvents {
		trim := len(m.recentEvents) - maxRecentEvents
		for _, ev := range m.recentEvents[:trim] {
			delete(m.seenNanos, ev.Unixnano)
		}
		m.recentEvents = m.recentEvents[trim:]
	}
}

func (m *dashboardModel) View() string {
	if m.stats == nil {
		return panelStyle.Width(m.width).Render(
			lipgloss.NewStyle().Foreground(colorDim).Render("Waiting for daemon connection..."))
	}

	layout := ComputeLayout(m.width, m.height)

	statsBox := m.renderCombinedStats(layout.StatsWidth, layout.TopRowHeight)
	topNBox := m.renderTopNPanel(layout.TopNWidth, layout.TopRowHeight)
	topRow := lipgloss.JoinHorizontal(lipgloss.Top, statsBox, topNBox)

	bottomBox := m.renderGroupedConnections(m.width, layout.BottomHeight)

	return lipgloss.JoinVertical(lipgloss.Left, topRow, bottomBox)
}

func (m *dashboardModel) renderCombinedStats(width, height int) string {
	s := m.stats
	total := s.Connections
	var acceptRatio, dropRatio, ignoreRatio float64
	if total > 0 {
		acceptRatio = float64(s.Accepted) / float64(total)
		dropRatio = float64(s.Dropped) / float64(total)
		ignoreRatio = float64(s.Ignored) / float64(total)
	}

	lines := []components.StatLine{
		{Label: "Connections:", Value: fmt.Sprintf("%d", total), Style: statValueStyle},
		{Label: "Accepted:", Value: fmt.Sprintf("%d", s.Accepted), Style: statAcceptStyle, Ratio: acceptRatio},
		{Label: "Dropped:", Value: fmt.Sprintf("%d", s.Dropped), Style: statDropStyle, Ratio: dropRatio},
		{Label: "Ignored:", Value: fmt.Sprintf("%d", s.Ignored), Style: statIgnoreStyle, Ratio: ignoreRatio},
		{Label: "", Value: "", Style: statValueStyle},
		{Label: "Rules:", Value: fmt.Sprintf("%d", s.Rules), Style: statValueStyle},
		{Label: "Hits:", Value: fmt.Sprintf("%d", s.RuleHits), Style: statAcceptStyle},
		{Label: "Misses:", Value: fmt.Sprintf("%d", s.RuleMisses), Style: statDropStyle},
		{Label: "DNS Resp:", Value: fmt.Sprintf("%d", s.DnsResponses), Style: statValueStyle},
	}

	return components.RenderStatBox("Stats", width, height, lines)
}

func (m *dashboardModel) renderTopNPanel(width, height int) string {
	var data map[string]uint64
	switch m.topNPage {
	case topNProcesses:
		data = m.stats.ByExecutable
	case topNDestinations:
		data = m.stats.ByHost
	case topNPorts:
		data = m.stats.ByPort
	}

	entries := sortedTopN(data, len(data))
	// Format port entries with service names.
	if m.topNPage == topNPorts {
		for i := range entries {
			entries[i].Name = formatPortStr(entries[i].Name)
		}
	}
	title := topNLabels[m.topNPage]

	var dots []string
	for i := 0; i < topNCount; i++ {
		if i == m.topNPage {
			dots = append(dots, lipgloss.NewStyle().Foreground(colorAccent).Render("●"))
		} else {
			dots = append(dots, lipgloss.NewStyle().Foreground(colorDim).Render("○"))
		}
	}

	innerHeight := height - 3 // border + title + header
	if innerHeight < 1 {
		innerHeight = 1
	}

	// Table header.
	nameCol := width - 16 // border+padding+count column
	if nameCol < 10 {
		nameCol = 10
	}
	headerLine := lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(
		fmt.Sprintf(" %-*s  %8s", nameCol-2, "NAME", "COUNT"))

	// Table rows with scrolling.
	var rows []string
	start := m.topNScroll
	if start >= len(entries) {
		start = 0
	}
	for i := start; i < len(entries) && len(rows) < innerHeight; i++ {
		e := entries[i]
		name := e.Name
		if len(name) > nameCol-2 {
			name = name[:nameCol-3] + "…"
		}
		countStr := components.FormatCount(e.Count)
		nameStyled := lipgloss.NewStyle().Foreground(colorFg).Render(fmt.Sprintf(" %-*s", nameCol-2, name))
		countStyled := lipgloss.NewStyle().Foreground(colorCyan).Bold(true).Render(fmt.Sprintf("%8s", countStr))

		if i%2 == 0 {
			rows = append(rows, nameStyled+"  "+countStyled)
		} else {
			rows = append(rows, lipgloss.NewStyle().Foreground(colorDim).Render(
				fmt.Sprintf(" %-*s", nameCol-2, name))+"  "+countStyled)
		}
	}
	for len(rows) < innerHeight {
		rows = append(rows, "")
	}

	titleLine := title + "  " + strings.Join(dots, " ") + "  " +
		lipgloss.NewStyle().Foreground(colorDim).Render("(t) cycle")
	content := headerLine + "\n" + strings.Join(rows, "\n")

	return panelStyle.
		Width(width).
		Height(height).
		Render(panelTitleStyle.Render(titleLine) + "\n" + content)
}

type connGroup struct {
	Process  string
	Dest     string
	Port     uint32
	Proto    string
	Action   string
	Count    int
	LastNano int64 // unixnano of most recent event
}

func (m *dashboardModel) renderGroupedConnections(width, height int) string {
	innerHeight := height - 2
	if innerHeight < 1 {
		innerHeight = 1
	}

	// Fixed columns: PORT(6) PROTO(5) ACTION(7) COUNT(6) LAST(8) + separators(10)
	fixedW := 42
	flexW := width - fixedW - 4 // border + padding
	if flexW < 20 {
		flexW = 20
	}
	// Process gets 30%, destination gets 70% of flexible space.
	procW := flexW * 30 / 100
	if procW < 8 {
		procW = 8
	}
	destW := flexW - procW
	if destW < 10 {
		destW = 10
	}

	portW := 12
	fixedW = fixedW + portW - 6 // adjust for wider port column
	flexW = width - fixedW - 4
	if flexW < 20 {
		flexW = 20
	}
	procW = flexW * 30 / 100
	if procW < 8 {
		procW = 8
	}
	destW = flexW - procW
	if destW < 10 {
		destW = 10
	}

	hdr := fmt.Sprintf("%-*s %-*s %-*s %-5s %-7s %6s %-8s",
		procW, "PROCESS", destW, "DESTINATION", portW, "PORT", "PROTO", "ACTION", "COUNT", "AGO")
	header := tableHeaderStyle.Render(hdr)

	groups := m.groupEvents()
	m.cachedGroups = groups

	// Clamp cursor.
	if m.selectedConn >= len(groups) {
		m.selectedConn = len(groups) - 1
	}
	if m.selectedConn < 0 {
		m.selectedConn = 0
	}

	var rows []string
	start := m.scrollOffset
	if start >= len(groups) {
		start = 0
	}
	for i := start; i < len(groups) && len(rows) < innerHeight-1; i++ {
		g := groups[i]

		proc := marquee(g.Process, procW, m.tickCount)
		dest := marquee(g.Dest, destW, m.tickCount)
		agoStr := relativeTime(g.LastNano)
		portLabel := formatPort(g.Port)

		if i == m.selectedConn {
			// Selected: plain text, uniform highlight.
			row := fmt.Sprintf("%-*s %-*s %-*s %-5s %-7s %6d %-8s",
				procW, proc, destW, dest, portW, portLabel, g.Proto, g.Action, g.Count, agoStr)
			rows = append(rows, tableSelectedStyle.Width(width-4).Render(row))
		} else {
			actionStr := fmt.Sprintf("%-7s", g.Action)
			switch g.Action {
			case "allow":
				actionStr = statAcceptStyle.Render(actionStr)
			case "deny", "reject":
				actionStr = statDropStyle.Render(actionStr)
			default:
				actionStr = tableRowStyle.Render(actionStr)
			}

			countStyled := lipgloss.NewStyle().Foreground(colorCyan).Bold(true)
			if g.Count > 10 {
				countStyled = lipgloss.NewStyle().Foreground(colorYellow).Bold(true)
			}
			if g.Count > 100 {
				countStyled = lipgloss.NewStyle().Foreground(colorOrange).Bold(true)
			}

			row := fmt.Sprintf("%-*s %-*s %-*s %-5s ", procW, proc, destW, dest, portW, portLabel, g.Proto) +
				actionStr + " " +
				countStyled.Render(fmt.Sprintf("%6d", g.Count)) + " " +
				lipgloss.NewStyle().Foreground(colorDim).Render(fmt.Sprintf("%-8s", agoStr))

			if i%2 == 0 {
				rows = append(rows, tableRowStyle.Render(row))
			} else {
				rows = append(rows, tableRowAltStyle.Render(row))
			}
		}
	}

	for len(rows) < innerHeight-1 {
		rows = append(rows, "")
	}

	windowLabel := fmt.Sprintf("Recent Connections  grouped: %s  (r) cycle", m.groupWindowLabel())
	content := header + "\n" + strings.Join(rows, "\n")

	return panelStyle.
		Width(width).
		Height(innerHeight).
		Render(panelTitleStyle.Render(windowLabel) + "\n" + content)
}

// selectedGroup returns the currently selected connection group, or nil.
func (m *dashboardModel) selectedGroup() *connGroup {
	if m.selectedConn >= 0 && m.selectedConn < len(m.cachedGroups) {
		g := m.cachedGroups[m.selectedConn]
		return &g
	}
	return nil
}

// relativeTime formats a unixnano timestamp as "Xs ago", "Xm ago", etc.
func relativeTime(unixnano int64) string {
	if unixnano <= 0 {
		return ""
	}
	d := time.Since(time.Unix(0, unixnano))
	switch {
	case d < time.Second:
		return "now"
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
}

// marquee scrolls text that's longer than maxW at 1 char/tick.
// Short text is returned padded. Long text slides left showing a moving window.
func marquee(text string, maxW, tick int) string {
	if maxW <= 0 {
		return ""
	}
	if len(text) <= maxW {
		return text
	}
	// Add padding so it scrolls smoothly with a gap.
	padded := text + "   " + text
	offset := tick % len(padded)
	end := offset + maxW
	if end > len(padded) {
		// Wrap around.
		return padded[offset:] + padded[:end-len(padded)]
	}
	return padded[offset:end]
}

func (m *dashboardModel) groupEvents() []connGroup {
	if len(m.recentEvents) == 0 {
		return nil
	}

	now := time.Now()
	cutoff := now.Add(-time.Duration(m.groupWindow) * time.Second)

	type groupKey struct {
		proc, dest, action string
		port               uint32
		proto              string
	}

	groups := make(map[groupKey]*connGroup)
	var order []groupKey

	for i := len(m.recentEvents) - 1; i >= 0; i-- {
		ev := m.recentEvents[i]
		if ev == nil || ev.Connection == nil || ev.Rule == nil {
			continue
		}
		if ev.Unixnano > 0 {
			evTime := time.Unix(0, ev.Unixnano)
			if evTime.Before(cutoff) {
				continue
			}
		}

		c := ev.Connection
		proc := extractProcessName(c.ProcessPath)
		dest := c.DstHost
		if dest == "" {
			dest = c.DstIp
		}

		k := groupKey{proc: proc, dest: dest, port: c.DstPort, proto: c.Protocol, action: ev.Rule.Action}
		if g, ok := groups[k]; ok {
			g.Count++
			if ev.Unixnano > g.LastNano {
				g.LastNano = ev.Unixnano
			}
		} else {
			groups[k] = &connGroup{
				Process: proc, Dest: dest, Port: c.DstPort,
				Proto: c.Protocol, Action: ev.Rule.Action,
				Count: 1, LastNano: ev.Unixnano,
			}
			order = append(order, k)
		}
	}

	result := make([]connGroup, 0, len(order))
	for _, k := range order {
		result = append(result, *groups[k])
	}

	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Count != result[j].Count {
			return result[i].Count > result[j].Count
		}
		return result[i].LastNano > result[j].LastNano
	})

	return result
}

func sortedTopN(data map[string]uint64, n int) []components.TopNEntry {
	entries := make([]components.TopNEntry, 0, len(data))
	for k, v := range data {
		entries = append(entries, components.TopNEntry{Name: extractProcessName(k), Count: v})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Count != entries[j].Count {
			return entries[i].Count > entries[j].Count
		}
		return entries[i].Name < entries[j].Name
	})
	if len(entries) > n {
		entries = entries[:n]
	}
	return entries
}
