package tui

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync/atomic"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/safedoor/ostui/internal/bus"
	"github.com/safedoor/ostui/internal/db"
	pb "github.com/safedoor/ostui/proto/protocol"
	"github.com/safedoor/ostui/internal/tui/components"
)

var ruleNameCounter atomic.Uint64

const matchTargetCount = 12

// Match target options for rule creation.
var matchTargets = [matchTargetCount]struct {
	label   string
	opType  string
	operand string
	dataFn  func(*pb.Connection) string
}{
	{"from this executable", "simple", "process.path", func(c *pb.Connection) string { return c.ProcessPath }},
	{"from this command line", "simple", "process.command", func(c *pb.Connection) string { return strings.Join(c.ProcessArgs, " ") }},
	{"from this CWD", "simple", "process.cwd", func(c *pb.Connection) string { return c.ProcessCwd }},
	{"to this port", "simple", "dest.port", func(c *pb.Connection) string { return fmt.Sprintf("%d", c.DstPort) }},
	{"to this IP", "simple", "dest.ip", func(c *pb.Connection) string { return c.DstIp }},
	{"to this host", "simple", "dest.host", func(c *pb.Connection) string { return c.DstHost }},
	{"from this source IP", "simple", "source.ip", func(c *pb.Connection) string { return c.SrcIp }},
	{"from this source port", "simple", "source.port", func(c *pb.Connection) string { return fmt.Sprintf("%d", c.SrcPort) }},
	{"via this protocol", "simple", "protocol", func(c *pb.Connection) string { return c.Protocol }},
	{"from this user", "simple", "user.id", func(c *pb.Connection) string { return fmt.Sprintf("%d", c.UserId) }},
	{"from this PID", "simple", "process.id", func(c *pb.Connection) string { return fmt.Sprintf("%d", c.ProcessId) }},
	{"matching SHA1", "simple", "process.hash.sha1", func(c *pb.Connection) string { return c.ProcessChecksums["sha1"] }},
}

var durations = []string{"once", "30s", "5m", "15m", "30m", "1h", "12h", "until restart", "always"}

// ruleCreatedMsg signals that a rule was created from the prompt and
// the rules list should be refreshed.
type ruleCreatedMsg struct{}

type promptModel struct {
	active    bool
	request   *bus.PromptRequest
	width     int
	height    int

	selectedDuration int
	targetCursor     int              // which target the cursor is on
	targetChecked    [matchTargetCount]bool // which targets are checked (multi-select)
	countdown        int
	showDetails      bool

	defaultAction   string
	defaultDuration string
	defaultTimeout  int

	database *db.DB
}

func newPromptModel(defaultAction, defaultDuration string, defaultTimeout int, database *db.DB) *promptModel {
	durIdx := 0
	for i, d := range durations {
		if d == defaultDuration {
			durIdx = i
			break
		}
	}
	return &promptModel{
		defaultAction:    defaultAction,
		defaultDuration:  defaultDuration,
		defaultTimeout:   defaultTimeout,
		selectedDuration: durIdx,
		database:         database,
	}
}

// Show activates the prompt with a new request.
func (m *promptModel) Show(req *bus.PromptRequest) {
	m.active = true
	m.request = req
	m.countdown = m.defaultTimeout
	m.targetChecked = [matchTargetCount]bool{}
	m.showDetails = false

	// Start cursor on the first visible target and check it by default.
	m.targetCursor = 0
	for i, t := range matchTargets {
		data := t.dataFn(req.Connection)
		if data != "" && data != "0" {
			m.targetCursor = i
			m.targetChecked[i] = true
			break
		}
	}

	// Reset duration to default.
	for i, d := range durations {
		if d == m.defaultDuration {
			m.selectedDuration = i
			break
		}
	}
}

// nextVisibleTarget returns the next target index (after current cursor) that
// has non-empty data for the current connection, wrapping around.
func (m *promptModel) nextVisibleTarget() int {
	if m.request == nil {
		return m.targetCursor
	}
	conn := m.request.Connection
	for step := 1; step < len(matchTargets); step++ {
		idx := (m.targetCursor + step) % len(matchTargets)
		data := matchTargets[idx].dataFn(conn)
		if data != "" && data != "0" {
			return idx
		}
	}
	return m.targetCursor
}

// prevVisibleTarget returns the previous target index (before current cursor)
// that has non-empty data for the current connection, wrapping around.
func (m *promptModel) prevVisibleTarget() int {
	if m.request == nil {
		return m.targetCursor
	}
	conn := m.request.Connection
	for step := 1; step < len(matchTargets); step++ {
		idx := (m.targetCursor - step + len(matchTargets)) % len(matchTargets)
		data := matchTargets[idx].dataFn(conn)
		if data != "" && data != "0" {
			return idx
		}
	}
	return m.targetCursor
}

type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m *promptModel) Update(msg tea.Msg) (bool, tea.Cmd) {
	if !m.active {
		return false, nil
	}

	switch msg := msg.(type) {
	case tickMsg:
		m.countdown--
		if m.countdown <= 0 {
			cmd := m.respond(m.defaultAction)
			return true, cmd
		}
		return true, tickCmd()

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("a"))):
			cmd := m.respond("allow")
			return true, cmd
		case key.Matches(msg, key.NewBinding(key.WithKeys("d"))):
			cmd := m.respond("deny")
			return true, cmd
		case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
			cmd := m.respond("reject")
			return true, cmd
		case key.Matches(msg, keys.Tab):
			m.selectedDuration = (m.selectedDuration + 1) % len(durations)
			return true, nil
		case key.Matches(msg, keys.ShiftTab):
			m.selectedDuration = (m.selectedDuration - 1 + len(durations)) % len(durations)
			return true, nil
		case key.Matches(msg, keys.Up):
			m.targetCursor = m.prevVisibleTarget()
			return true, nil
		case key.Matches(msg, keys.Down):
			m.targetCursor = m.nextVisibleTarget()
			return true, nil
		case key.Matches(msg, keys.Space):
			m.targetChecked[m.targetCursor] = !m.targetChecked[m.targetCursor]
			return true, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("i"))):
			m.showDetails = !m.showDetails
			return true, nil
		case key.Matches(msg, keys.Esc):
			cmd := m.respond(m.defaultAction)
			return true, cmd
		}
		return true, nil
	}

	return false, nil
}

// checkedTargets returns the indices of all checked match targets that have
// non-empty data for the current connection.
func (m *promptModel) checkedTargets() []int {
	var result []int
	for i, checked := range m.targetChecked {
		if !checked {
			continue
		}
		data := matchTargets[i].dataFn(m.request.Connection)
		if data == "" || data == "0" {
			continue
		}
		result = append(result, i)
	}
	// If nothing checked (or all checked targets had empty data), fall back to
	// whichever target the cursor is on.
	if len(result) == 0 {
		result = []int{m.targetCursor}
	}
	return result
}

func (m *promptModel) respond(action string) tea.Cmd {
	if m.request == nil || m.request.ResponseCh == nil {
		m.active = false
		return nil
	}

	conn := m.request.Connection
	dur := durations[m.selectedDuration]
	checked := m.checkedTargets()

	var op *pb.Operator
	var dbOpType, dbOpOperand, dbOpData string

	if len(checked) == 1 {
		// Simple rule — single condition.
		t := matchTargets[checked[0]]
		op = &pb.Operator{
			Type:    t.opType,
			Operand: t.operand,
			Data:    t.dataFn(conn),
		}
		dbOpType = t.opType
		dbOpOperand = t.operand
		dbOpData = t.dataFn(conn)
	} else {
		// Compound rule — list of conditions (AND).
		var subs []*pb.Operator
		var jsonSubs []subOperator
		for _, idx := range checked {
			t := matchTargets[idx]
			data := t.dataFn(conn)
			subs = append(subs, &pb.Operator{
				Type:    t.opType,
				Operand: t.operand,
				Data:    data,
			})
			jsonSubs = append(jsonSubs, subOperator{
				Type:    t.opType,
				Operand: t.operand,
				Data:    data,
			})
		}
		op = &pb.Operator{
			Type:    "list",
			Operand: "list",
			List:    subs,
		}
		jsonBytes, _ := json.Marshal(jsonSubs)
		dbOpType = "list"
		dbOpOperand = "list"
		dbOpData = string(jsonBytes)
	}

	rule := &pb.Rule{
		Created:  time.Now().Unix(),
		Name:     generateRuleName(action, conn),
		Enabled:  true,
		Action:   action,
		Duration: dur,
		Operator: op,
	}

	select {
	case m.request.ResponseCh <- rule:
	case <-time.After(5 * time.Second):
		log.Fatalf("FATAL: prompt ResponseCh blocked for 5s, rule %s lost — daemon will not receive user decision", rule.Name)
	}

	// Persist rule to local database so it appears in the rules list.
	nodeAddr := m.request.NodeAddr
	if nodeAddr == "" {
		nodeAddr = "unix:local"
	}
	created := time.Now().Format(time.RFC3339)
	if m.database != nil {
		if err := m.database.InsertRule(
			nodeAddr, rule.Name, "true", "false",
			rule.Action, rule.Duration, dbOpType, "false",
			dbOpOperand, dbOpData, "", "false", created,
		); err != nil {
			log.Printf("ERROR prompt InsertRule(%s): %v", rule.Name, err)
		} else {
			log.Printf("Prompt rule persisted: %s (action=%s duration=%s node=%s)", rule.Name, rule.Action, rule.Duration, nodeAddr)
		}
	}

	m.active = false
	m.request = nil

	return func() tea.Msg { return ruleCreatedMsg{} }
}

func (m *promptModel) View() string {
	if !m.active || m.request == nil {
		return ""
	}

	conn := m.request.Connection

	// Build description.
	proc := extractProcessName(conn.ProcessPath)
	dest := conn.DstHost
	if dest == "" {
		dest = conn.DstIp
	}

	desc := promptTitleStyle.Render(fmt.Sprintf(
		"%s is connecting to %s on %s port %d",
		proc, dest, strings.ToUpper(conn.Protocol), conn.DstPort,
	))

	// Process info.
	info := strings.Join([]string{
		promptLabelStyle.Render("Process:") + "  " + promptValueStyle.Render(conn.ProcessPath),
		promptLabelStyle.Render("Args:") + "     " + promptValueStyle.Render(strings.Join(conn.ProcessArgs, " ")),
		promptLabelStyle.Render("PID:") + "      " + promptValueStyle.Render(fmt.Sprintf("%d", conn.ProcessId)) +
			"    " + promptLabelStyle.Render("UID:") + " " + promptValueStyle.Render(fmt.Sprintf("%d", conn.UserId)),
		promptLabelStyle.Render("Dst:") + "      " + promptValueStyle.Render(fmt.Sprintf("%s (%s)", conn.DstIp, conn.DstHost)),
		promptLabelStyle.Render("Port:") + "     " + promptValueStyle.Render(fmt.Sprintf("%d", conn.DstPort)) +
			"    " + promptLabelStyle.Render("Proto:") + " " + promptValueStyle.Render(conn.Protocol),
	}, "\n")

	if m.showDetails {
		info += "\n" + promptLabelStyle.Render("CWD:") + "      " + promptValueStyle.Render(conn.ProcessCwd)
		info += "\n" + promptLabelStyle.Render("Src:") + "      " + promptValueStyle.Render(fmt.Sprintf("%s:%d", conn.SrcIp, conn.SrcPort))
		for k, v := range conn.ProcessChecksums {
			info += "\n" + promptLabelStyle.Render(k+":") + "     " + promptValueStyle.Render(v)
		}
	}

	// Action line.
	actionLine := "  Action:   " +
		promptActionAllowStyle.Render("(a) Allow") + "   " +
		promptActionDenyStyle.Render("(d) Deny") + "   " +
		promptActionRejectStyle.Render("(r) Reject")

	// Duration selector.
	var durParts []string
	for i, d := range durations {
		if i == m.selectedDuration {
			durParts = append(durParts, promptSelectedStyle.Render(" "+d+" "))
		} else {
			durParts = append(durParts, lipgloss.NewStyle().Foreground(colorDim).Render(d))
		}
	}
	durLine := "  Duration: " + strings.Join(durParts, " │ ")

	// Match target selector (multi-select with checkboxes).
	var targetLines []string
	for i, t := range matchTargets {
		data := t.dataFn(conn)
		if data == "" || data == "0" {
			continue
		}
		check := "[ ]"
		if m.targetChecked[i] {
			check = "[x]"
		}
		isCursor := i == m.targetCursor
		if isCursor {
			cursor := promptSelectedStyle.Render("▸ " + check)
			label := promptSelectedStyle.Render(" " + t.label)
			targetLines = append(targetLines, cursor+label)
		} else if m.targetChecked[i] {
			targetLines = append(targetLines, lipgloss.NewStyle().Foreground(colorFg).Render("  "+check+" "+t.label))
		} else {
			targetLines = append(targetLines, lipgloss.NewStyle().Foreground(colorDim).Render("  "+check+" "+t.label))
		}
	}
	matchSection := "  Match (combine with space):\n" + strings.Join(targetLines, "\n")

	// Footer.
	footer := "  " +
		lipgloss.NewStyle().Foreground(colorDim).Render("[i] details   [tab] duration   [↑↓] navigate   [space] toggle match") +
		"\n  " +
		promptCountdownStyle.Render("Timeout: "+components.RenderCountdown(m.countdown)) +
		"  " +
		lipgloss.NewStyle().Foreground(colorDim).Render("[esc] default")

	content := strings.Join([]string{
		"",
		desc,
		"",
		info,
		"",
		actionLine,
		"",
		durLine,
		"",
		matchSection,
		"",
		footer,
	}, "\n")

	modal := promptStyle.Render(
		promptTitleStyle.Render("New Connection") + "\n" + content,
	)

	// Center the modal.
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, modal)
}

func generateRuleName(action string, conn *pb.Connection) string {
	proc := extractProcessName(conn.ProcessPath)
	seq := ruleNameCounter.Add(1)
	return fmt.Sprintf("%s-%s-%d-%d-%d",
		action, proc, conn.DstPort, time.Now().UnixNano()%100000, seq)
}
