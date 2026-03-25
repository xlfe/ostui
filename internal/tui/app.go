package tui

import (
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/safedoor/ostui/internal/bus"
	"github.com/safedoor/ostui/internal/config"
	"github.com/safedoor/ostui/internal/db"
)

// View tabs.
const (
	tabDashboard = iota
	tabRules
	tabNodes
	tabFirewall
	tabAlerts
	tabCount
)

var tabNames = [tabCount]string{"Dashboard", "Rules", "Nodes", "Firewall", "Alerts"}

// Tea messages from the event bus bridge.
type (
	statsMsg    bus.StatsUpdate
	promptMsg   bus.PromptRequest
	alertMsg    bus.AlertEvent
	nodeMsg     bus.NodeEvent
	busCheckMsg struct{}
)

// quitMsg is used internally to signal quit via the bus.
type quitMsg struct{}

// App is the top-level bubbletea model.
type App struct {
	cfg      *config.Config
	bus      *bus.EventBus
	db       *db.DB
	program  *tea.Program

	width, height int
	activeTab     int
	showHelp      bool

	dashboard *dashboardModel
	rules     *rulesModel
	nodes     *nodesModel
	firewall  *firewallModel
	alerts    *alertsModel
	prompt    *promptModel
	status    statusBarModel
}

// New creates the TUI application.
func New(cfg *config.Config, eventBus *bus.EventBus, database *db.DB) *App {
	dash := newDashboardModel(cfg.GroupWindow)
	dash.loadFromDB(database)

	return &App{
		cfg:       cfg,
		bus:       eventBus,
		db:        database,
		activeTab: tabDashboard,
		dashboard: dash,
		rules:     newRulesModel(database, eventBus),
		nodes:     newNodesModel(database, eventBus),
		firewall:  newFirewallModel(),
		alerts:    newAlertsModel(database),
		prompt:    newPromptModel(cfg.DefaultAction, cfg.DefaultDuration, cfg.DefaultTimeout),
	}
}

// Run starts the bubbletea program (blocks on main goroutine).
func (a *App) Run() error {
	a.program = tea.NewProgram(a)

	// Bridge goroutine: forward bus events to tea program.
	go a.bridgeEvents()

	_, err := a.program.Run()
	return err
}

func (a *App) bridgeEvents() {
	for {
		select {
		case <-a.bus.Done:
			if a.program != nil {
				a.program.Send(quitMsg{})
			}
			return
		case s := <-a.bus.StatsUpdate:
			if a.program != nil {
				a.program.Send(statsMsg(s))
			}
		case p := <-a.bus.PromptReq:
			if a.program != nil {
				a.program.Send(promptMsg(p))
			}
		case al := <-a.bus.AlertEvent:
			if a.program != nil {
				a.program.Send(alertMsg(al))
			}
		case n := <-a.bus.NodeEvent:
			if a.program != nil {
				a.program.Send(nodeMsg(n))
			}
		}
	}
}

// --- tea.Model interface ---

func (a *App) Init() tea.Cmd {
	return tea.Batch(tickCmd(), a.pollBus())
}

func (a *App) pollBus() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return busCheckMsg{}
	})
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case quitMsg:
		return a, tea.Quit

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.updateChildSizes()
		return a, nil

	case statsMsg:
		a.dashboard.updateStats(msg.Stats)
		a.status.daemonVersion = msg.Stats.DaemonVersion
		a.status.uptime = msg.Stats.Uptime
		return a, nil

	case promptMsg:
		req := bus.PromptRequest(msg)
		a.prompt.Show(&req)
		return a, tickCmd()

	case alertMsg:
		if msg.Alert != nil {
			a.status.alert = msg.Alert.What.String()
			a.status.alertTime = time.Now()
		}
		a.alerts.loadAlerts()
		return a, nil

	case nodeMsg:
		switch msg.Type {
		case bus.NodeAdded:
			a.status.nodesOnline++
			a.status.nodesTotal++
		case bus.NodeRemoved:
			a.status.nodesOnline--
			if a.status.nodesOnline < 0 {
				a.status.nodesOnline = 0
			}
		}
		a.nodes.loadNodes()
		return a, nil

	case tickMsg:
		a.dashboard.tickCount++
		if a.prompt.active {
			handled, cmd := a.prompt.Update(msg)
			if handled {
				return a, cmd
			}
		}
		return a, tickCmd()

	case busCheckMsg:
		return a, a.pollBus()

	case tea.KeyMsg:
		// Prompt gets priority for key handling.
		if a.prompt.active {
			handled, cmd := a.prompt.Update(msg)
			if handled {
				return a, cmd
			}
		}

		// When a sub-view is in editing mode, forward keys to it directly
		// (skip global shortcuts).
		if a.activeTab == tabRules && a.rules.editing != modeNone {
			cmd := a.rules.Update(msg)
			return a, cmd
		}

		// Help overlay toggle.
		if a.showHelp {
			a.showHelp = false
			return a, nil
		}

		switch {
		case key.Matches(msg, keys.Quit):
			return a, tea.Quit
		case key.Matches(msg, keys.Help):
			a.showHelp = !a.showHelp
			return a, nil
		case key.Matches(msg, keys.Tab1):
			a.activeTab = tabDashboard
			return a, nil
		case key.Matches(msg, keys.Tab2):
			a.activeTab = tabRules
			a.rules.loadRules()
			return a, nil
		case key.Matches(msg, keys.Tab3):
			a.activeTab = tabNodes
			a.nodes.loadNodes()
			return a, nil
		case key.Matches(msg, keys.Tab4):
			a.activeTab = tabFirewall
			return a, nil
		case key.Matches(msg, keys.Tab5):
			a.activeTab = tabAlerts
			a.alerts.loadAlerts()
			return a, nil
		}

		// Forward to active view.
		switch a.activeTab {
		case tabRules:
			cmd := a.rules.Update(msg)
			return a, cmd
		case tabNodes:
			cmd := a.nodes.Update(msg)
			return a, cmd
		case tabAlerts:
			cmd := a.alerts.Update(msg)
			return a, cmd
		case tabDashboard:
			switch msg.String() {
			case "j", "down":
				a.dashboard.selectedConn++
				// Auto-scroll to keep selection visible.
				layout := ComputeLayout(a.width, a.height-2)
				visible := layout.BottomHeight - 4
				if visible > 0 && a.dashboard.selectedConn >= a.dashboard.scrollOffset+visible {
					a.dashboard.scrollOffset = a.dashboard.selectedConn - visible + 1
				}
			case "k", "up":
				if a.dashboard.selectedConn > 0 {
					a.dashboard.selectedConn--
				}
				if a.dashboard.selectedConn < a.dashboard.scrollOffset {
					a.dashboard.scrollOffset = a.dashboard.selectedConn
				}
			case "t":
				a.dashboard.topNPage = (a.dashboard.topNPage + 1) % topNCount
				a.dashboard.topNScroll = 0
			case "r":
				a.dashboard.cycleGroupWindow()
			case "a", "enter":
				if g := a.dashboard.selectedGroup(); g != nil {
					a.activeTab = tabRules
					a.rules.loadRules()
					a.rules.startAddFromConnection(g)
				}
			}
			return a, nil
		}
	}

	return a, nil
}

func (a *App) View() tea.View {
	v := tea.NewView("")
	v.AltScreen = true

	if a.width == 0 || a.height == 0 {
		v.SetContent("Initializing...")
		return v
	}

	// Tab bar.
	tabBar := a.renderTabBar()

	// Active view content.
	var content string
	switch a.activeTab {
	case tabDashboard:
		content = a.dashboard.View()
	case tabRules:
		content = a.rules.View()
	case tabNodes:
		content = a.nodes.View()
	case tabFirewall:
		content = a.firewall.View()
	case tabAlerts:
		content = a.alerts.View()
	}

	// Status bar.
	a.status.width = a.width
	statusBar := a.status.View()

	// Compose.
	result := tabBar + "\n" + content + "\n" + statusBar

	// Overlay prompt if active.
	if a.prompt.active {
		result = a.prompt.View()
	}

	// Help overlay.
	if a.showHelp {
		result = a.renderHelp()
	}

	v.SetContent(result)
	return v
}

func (a *App) renderTabBar() string {
	var tabs []string
	for i := 0; i < tabCount; i++ {
		label := tabNames[i]
		prefix := string(rune('1' + i))
		text := prefix + ":" + label
		if i == a.activeTab {
			tabs = append(tabs, tabActiveStyle.Render(text))
		} else {
			tabs = append(tabs, tabInactiveStyle.Render(text))
		}
	}
	bar := strings.Join(tabs, "")
	// Fill remaining width.
	barWidth := lipgloss.Width(bar)
	if barWidth < a.width {
		bar += lipgloss.NewStyle().
			Background(lipgloss.Color("#3b4261")).
			Render(strings.Repeat(" ", a.width-barWidth))
	}
	return bar
}

func (a *App) renderHelp() string {
	helpEntries := []struct{ key, desc string }{
		{"1-5", "Switch view"},
		{"q / Ctrl+C", "Quit"},
		{"?", "Toggle help"},
		{"j/k, up/down", "Navigate"},
		{"Enter", "Select"},
		{"Esc", "Back / Default"},
		{"/", "Filter"},
		{"", ""},
		{"Dashboard:", ""},
		{"j/k", "Scroll events"},
		{"", ""},
		{"Rules:", ""},
		{"t", "Toggle enable/disable"},
		{"d", "Delete rule"},
		{"", ""},
		{"Prompt:", ""},
		{"a/d/r", "Allow / Deny / Reject"},
		{"Tab", "Cycle duration"},
		{"up/down", "Cycle match target"},
		{"i", "Toggle details"},
		{"Esc", "Apply default action"},
	}

	var rows []string
	for _, e := range helpEntries {
		if e.key == "" && e.desc == "" {
			rows = append(rows, "")
			continue
		}
		if e.desc == "" {
			rows = append(rows, lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(e.key))
			continue
		}
		rows = append(rows, helpKeyStyle.Render(e.key)+" "+helpDescStyle.Render(e.desc))
	}

	content := strings.Join(rows, "\n")
	modal := helpStyle.Render(
		lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("Key Bindings") + "\n\n" + content,
	)

	return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center, modal)
}

func (a *App) updateChildSizes() {
	contentHeight := a.height - 2 // tab bar + status bar
	if contentHeight < 5 {
		contentHeight = 5
	}

	a.dashboard.width = a.width
	a.dashboard.height = contentHeight
	a.rules.width = a.width
	a.rules.height = contentHeight
	a.nodes.width = a.width
	a.nodes.height = contentHeight
	a.firewall.width = a.width
	a.firewall.height = contentHeight
	a.alerts.width = a.width
	a.alerts.height = contentHeight
	a.prompt.width = a.width
	a.prompt.height = a.height
}
