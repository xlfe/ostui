package tui

import (
	"fmt"
	"strings"
	"time"

	"log"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/safedoor/ostui/internal/bus"
	"github.com/safedoor/ostui/internal/db"
	pb "github.com/safedoor/ostui/proto/protocol"
)

type nodesModel struct {
	width, height int
	nodes         []db.NodeRow
	cursor        int
	database      *db.DB
	eventBus      *bus.EventBus
	confirmDel    bool

	// Track interception state per node addr.
	intercepting map[string]bool
}

func newNodesModel(database *db.DB, eventBus *bus.EventBus) *nodesModel {
	return &nodesModel{
		database:     database,
		eventBus:     eventBus,
		intercepting: make(map[string]bool),
	}
}

func (m *nodesModel) loadNodes() {
	nodes, err := m.database.GetNodes()
	if err != nil {
		log.Printf("ERROR loadNodes: %v", err)
		return
	}
	m.nodes = nodes
	// Default new nodes to intercepting=true (daemon default).
	for _, n := range m.nodes {
		if _, ok := m.intercepting[n.Addr]; !ok {
			m.intercepting[n.Addr] = true
		}
	}
}

func (m *nodesModel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.confirmDel {
			if msg.String() == "y" || msg.String() == "Y" {
				m.deleteNode()
			}
			m.confirmDel = false
			return nil
		}

		switch {
		case key.Matches(msg, keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
		case key.Matches(msg, keys.Down):
			if m.cursor < len(m.nodes)-1 {
				m.cursor++
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("i"))):
			m.toggleInterception()
		case key.Matches(msg, keys.Delete):
			if len(m.nodes) > 0 {
				m.confirmDel = true
			}
		}
	}
	return nil
}

func (m *nodesModel) toggleInterception() {
	if m.cursor >= len(m.nodes) || m.eventBus == nil {
		return
	}
	n := m.nodes[m.cursor]

	currently := m.intercepting[n.Addr]
	var action pb.Action
	if currently {
		action = pb.Action_DISABLE_INTERCEPTION
		m.intercepting[n.Addr] = false
	} else {
		action = pb.Action_ENABLE_INTERCEPTION
		m.intercepting[n.Addr] = true
	}

	notif := &pb.Notification{
		Id:   uint64(time.Now().UnixNano()),
		Type: action,
	}
	select {
	case m.eventBus.NotifOut <- bus.OutgoingNotification{NodeAddr: n.Addr, Notification: notif}:
	case <-time.After(5 * time.Second):
		log.Fatalf("FATAL: NotifOut channel blocked for 5s, interception toggle for %s lost", n.Addr)
	}
}

func (m *nodesModel) deleteNode() {
	if m.cursor >= len(m.nodes) {
		return
	}
	n := m.nodes[m.cursor]
	log.Printf("Deleting node %s (and its rules/connections)", n.Addr)
	if err := m.database.DeleteNode(n.Addr); err != nil {
		log.Printf("ERROR deleteNode(%s): %v", n.Addr, err)
	}
	if m.eventBus != nil {
		notif := &pb.Notification{
			Id:   uint64(time.Now().UnixNano()),
			Type: pb.Action_STOP,
		}
		select {
		case m.eventBus.NotifOut <- bus.OutgoingNotification{NodeAddr: n.Addr, Notification: notif}:
		case <-time.After(5 * time.Second):
			log.Fatalf("FATAL: NotifOut channel blocked for 5s, STOP for node %s lost", n.Addr)
		}
	}
	delete(m.intercepting, n.Addr)
	m.loadNodes()
}

func (m *nodesModel) View() string {
	listHeight := (m.height * 55) / 100
	if listHeight < 5 {
		listHeight = 5
	}
	detailHeight := m.height - listHeight - 1
	if detailHeight < 4 {
		detailHeight = 4
	}
	tableRows := listHeight - 3
	if tableRows < 1 {
		tableRows = 1
	}

	// --- Node list ---
	header := tableHeaderStyle.Render(fmt.Sprintf(
		" %-24s %-14s %-10s %-10s %-10s %-8s %-8s",
		"ADDRESS", "HOSTNAME", "VERSION", "STATUS", "INTERCEPT", "RULES", "CONNS",
	))

	var rows []string
	for i, n := range m.nodes {
		if i >= tableRows {
			break
		}

		statusStyle := statAcceptStyle
		statusLabel := n.Status
		if n.Status != "online" {
			statusStyle = statDropStyle
		}

		intercept := "ON"
		interceptStyle := lipgloss.NewStyle().Foreground(colorGreen).Bold(true)
		if !m.intercepting[n.Addr] {
			intercept = "OFF"
			interceptStyle = lipgloss.NewStyle().Foreground(colorRed).Bold(true)
		}

		row := fmt.Sprintf(" %-24s %-14s %-10s %s %s %-8s %-8s",
			n.Addr, n.Hostname, n.DaemonVersion,
			statusStyle.Render(fmt.Sprintf("%-10s", statusLabel)),
			interceptStyle.Render(fmt.Sprintf("%-10s", intercept)),
			n.DaemonRules, n.Cons,
		)

		if i == m.cursor {
			rows = append(rows, tableSelectedStyle.Width(m.width-4).Render(row))
		} else if i%2 == 0 {
			rows = append(rows, tableRowStyle.Render(row))
		} else {
			rows = append(rows, tableRowAltStyle.Render(row))
		}
	}
	for len(rows) < tableRows {
		rows = append(rows, "")
	}

	listContent := header + "\n" + strings.Join(rows, "\n")
	listPanel := panelStyle.Width(m.width).Render(
		panelTitleStyle.Render(fmt.Sprintf("Nodes (%d)", len(m.nodes))) + "\n" + listContent)

	// --- Detail card ---
	detailPanel := m.renderDetailCard(detailHeight)

	// --- Footer ---
	footer := lipgloss.NewStyle().Foreground(colorDim).Render(
		"  [i] toggle interception  [d] delete node  [↑↓] navigate")
	if m.confirmDel && m.cursor < len(m.nodes) {
		footer = lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render(
			fmt.Sprintf("  Delete node '%s'? [y] confirm / any key cancel", m.nodes[m.cursor].Addr))
	}

	return lipgloss.JoinVertical(lipgloss.Left, listPanel, detailPanel, footer)
}

func (m *nodesModel) renderDetailCard(height int) string {
	if len(m.nodes) == 0 || m.cursor >= len(m.nodes) {
		return panelStyle.Width(m.width).Height(height).Render(
			panelTitleStyle.Render("Node Details") + "\n" +
				lipgloss.NewStyle().Foreground(colorDim).Render("No node selected"))
	}

	n := m.nodes[m.cursor]
	lbl := lipgloss.NewStyle().Foreground(colorDim).Width(16)
	val := lipgloss.NewStyle().Foreground(colorFg)

	statusStyle := statAcceptStyle
	if n.Status != "online" {
		statusStyle = statDropStyle
	}

	interceptLabel := "ON - rules are being enforced"
	interceptStyle := lipgloss.NewStyle().Foreground(colorGreen).Bold(true)
	if !m.intercepting[n.Addr] {
		interceptLabel = "OFF - all traffic allowed"
		interceptStyle = lipgloss.NewStyle().Foreground(colorRed).Bold(true)
	}

	lines := []string{
		lbl.Render("Address:") + val.Render(n.Addr),
		lbl.Render("Hostname:") + val.Render(n.Hostname),
		lbl.Render("Version:") + val.Render(n.DaemonVersion) +
			"    " + lbl.Render("Uptime:") + val.Render(n.DaemonUptime),
		lbl.Render("Status:") + statusStyle.Render(n.Status) +
			"    " + lbl.Render("Rules:") + val.Render(n.DaemonRules) +
			"    " + lbl.Render("Connections:") + val.Render(n.Cons),
		lbl.Render("Dropped:") + val.Render(n.ConsDropped),
		lbl.Render("Interception:") + interceptStyle.Render(interceptLabel),
		lbl.Render("Last Seen:") + lipgloss.NewStyle().Foreground(colorDim).Render(n.LastConnection),
	}

	content := strings.Join(lines, "\n")
	return panelActiveStyle.Width(m.width).Height(height).Render(
		panelTitleStyle.Render("Node Details") + "\n" + content)
}
