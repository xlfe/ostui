package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/safedoor/ostui/internal/bus"
	pb "github.com/safedoor/ostui/proto/protocol"
	"github.com/safedoor/ostui/internal/tui/components"
)

// Match target options for rule creation.
var matchTargets = []struct {
	label   string
	opType  string
	operand string
	dataFn  func(*pb.Connection) string
}{
	{"from this executable", "simple", "process.path", func(c *pb.Connection) string { return c.ProcessPath }},
	{"from this command line", "simple", "process.command", func(c *pb.Connection) string { return strings.Join(c.ProcessArgs, " ") }},
	{"to this port", "simple", "dest.port", func(c *pb.Connection) string { return fmt.Sprintf("%d", c.DstPort) }},
	{"to this IP", "simple", "dest.ip", func(c *pb.Connection) string { return c.DstIp }},
	{"to this host", "simple", "dest.host", func(c *pb.Connection) string { return c.DstHost }},
	{"from this user", "simple", "user.id", func(c *pb.Connection) string { return fmt.Sprintf("%d", c.UserId) }},
	{"from this PID", "simple", "process.id", func(c *pb.Connection) string { return fmt.Sprintf("%d", c.ProcessId) }},
}

var durations = []string{"once", "30s", "5m", "15m", "30m", "1h", "12h", "until restart", "always"}

type promptModel struct {
	active    bool
	request   *bus.PromptRequest
	width     int
	height    int

	selectedDuration int
	selectedTarget   int
	countdown        int
	showDetails      bool

	defaultAction   string
	defaultDuration string
	defaultTimeout  int
}

func newPromptModel(defaultAction, defaultDuration string, defaultTimeout int) *promptModel {
	durIdx := 0
	for i, d := range durations {
		if d == defaultDuration {
			durIdx = i
			break
		}
	}
	return &promptModel{
		defaultAction:   defaultAction,
		defaultDuration: defaultDuration,
		defaultTimeout:  defaultTimeout,
		selectedDuration: durIdx,
	}
}

// Show activates the prompt with a new request.
func (m *promptModel) Show(req *bus.PromptRequest) {
	m.active = true
	m.request = req
	m.countdown = m.defaultTimeout
	m.selectedTarget = 0
	m.showDetails = false

	// Reset duration to default.
	for i, d := range durations {
		if d == m.defaultDuration {
			m.selectedDuration = i
			break
		}
	}
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
			m.respond(m.defaultAction)
			return true, nil
		}
		return true, tickCmd()

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("a"))):
			m.respond("allow")
			return true, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("d"))):
			m.respond("deny")
			return true, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
			m.respond("reject")
			return true, nil
		case key.Matches(msg, keys.Tab):
			m.selectedDuration = (m.selectedDuration + 1) % len(durations)
			return true, nil
		case key.Matches(msg, keys.ShiftTab):
			m.selectedDuration = (m.selectedDuration - 1 + len(durations)) % len(durations)
			return true, nil
		case key.Matches(msg, keys.Up):
			m.selectedTarget = (m.selectedTarget - 1 + len(matchTargets)) % len(matchTargets)
			return true, nil
		case key.Matches(msg, keys.Down):
			m.selectedTarget = (m.selectedTarget + 1) % len(matchTargets)
			return true, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("i"))):
			m.showDetails = !m.showDetails
			return true, nil
		case key.Matches(msg, keys.Esc):
			m.respond(m.defaultAction)
			return true, nil
		}
		return true, nil
	}

	return false, nil
}

func (m *promptModel) respond(action string) {
	if m.request == nil || m.request.ResponseCh == nil {
		m.active = false
		return
	}

	conn := m.request.Connection
	target := matchTargets[m.selectedTarget]
	dur := durations[m.selectedDuration]

	rule := &pb.Rule{
		Created:  time.Now().Unix(),
		Name:     generateRuleName(action, conn),
		Enabled:  true,
		Action:   action,
		Duration: dur,
		Operator: &pb.Operator{
			Type:    target.opType,
			Operand: target.operand,
			Data:    target.dataFn(conn),
		},
	}

	select {
	case m.request.ResponseCh <- rule:
	default:
	}
	m.active = false
	m.request = nil
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

	// Match target selector.
	var targetLines []string
	for i, t := range matchTargets {
		data := t.dataFn(conn)
		if data == "" || data == "0" {
			continue
		}
		prefix := "  "
		label := t.label
		if i == m.selectedTarget {
			prefix = promptSelectedStyle.Render("▸ ")
			label = promptSelectedStyle.Render(label)
		} else {
			prefix = lipgloss.NewStyle().Foreground(colorDim).Render("  ")
			label = lipgloss.NewStyle().Foreground(colorDim).Render(label)
		}
		targetLines = append(targetLines, prefix+label)
	}
	matchSection := "  Match:\n" + strings.Join(targetLines, "\n")

	// Footer.
	footer := "  " +
		lipgloss.NewStyle().Foreground(colorDim).Render("[i] details   [tab] duration   [↑↓] match") +
		"    " +
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
	return fmt.Sprintf("%s-%s-%d-%d",
		action, proc, conn.DstPort, time.Now().UnixNano()%100000)
}
