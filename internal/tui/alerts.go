package tui

import (
	"fmt"
	"log"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/safedoor/ostui/internal/db"
)

type alertsModel struct {
	width, height int
	alerts        []db.AlertRow
	cursor        int
	scrollOffset  int
	database      *db.DB
}

func newAlertsModel(database *db.DB) *alertsModel {
	return &alertsModel{database: database}
}

func (m *alertsModel) loadAlerts() {
	alerts, err := m.database.GetAlerts(200)
	if err != nil {
		log.Printf("ERROR loadAlerts: %v", err)
		return
	}
	m.alerts = alerts
}

func (m *alertsModel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
		case key.Matches(msg, keys.Down):
			if m.cursor < len(m.alerts)-1 {
				m.cursor++
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
			m.loadAlerts()
		}
	}
	return nil
}

func (m *alertsModel) View() string {
	innerHeight := m.height - 4
	if innerHeight < 1 {
		innerHeight = 1
	}

	if len(m.alerts) == 0 {
		content := lipgloss.NewStyle().Foreground(colorDim).Render(
			"No alerts. Alerts appear when the daemon reports errors, warnings, or kernel events.\n\n" +
				"Press [r] to refresh.")
		return panelStyle.Width(m.width).Render(
			panelTitleStyle.Render("Alerts (0)") + "\n\n" + content)
	}

	// Dynamic columns.
	fixedW := 42 // type(10) + priority(8) + what(12) + spacing
	timeW := 20
	bodyW := m.width - fixedW - timeW - 8
	if bodyW < 10 {
		bodyW = 10
	}

	header := tableHeaderStyle.Render(fmt.Sprintf(
		" %-*s %-10s %-8s %-12s %s",
		timeW, "TIME", "TYPE", "PRIORITY", "WHAT", "BODY",
	))

	// Keep cursor in the visible window.
	if m.cursor >= m.scrollOffset+innerHeight {
		m.scrollOffset = m.cursor - innerHeight + 1
	}
	if m.cursor < m.scrollOffset {
		m.scrollOffset = m.cursor
	}

	var rows []string
	end := m.scrollOffset + innerHeight
	if end > len(m.alerts) {
		end = len(m.alerts)
	}
	for i := m.scrollOffset; i < end; i++ {
		a := m.alerts[i]

		t := a.Time
		if len(t) > timeW {
			t = t[:timeW-1] + "…"
		}
		body := a.Body
		if len(body) > bodyW {
			body = body[:bodyW-1] + "…"
		}

		typeStyle := lipgloss.NewStyle().Foreground(colorFg)
		switch a.Type {
		case "ERROR":
			typeStyle = lipgloss.NewStyle().Foreground(colorRed).Bold(true)
		case "WARNING":
			typeStyle = lipgloss.NewStyle().Foreground(colorYellow)
		case "INFO":
			typeStyle = lipgloss.NewStyle().Foreground(colorCyan)
		}

		prioStyle := lipgloss.NewStyle().Foreground(colorDim)
		switch a.Priority {
		case "HIGH":
			prioStyle = lipgloss.NewStyle().Foreground(colorRed).Bold(true)
		case "MEDIUM":
			prioStyle = lipgloss.NewStyle().Foreground(colorYellow)
		}

		row := fmt.Sprintf(" %-*s ", timeW, t) +
			typeStyle.Render(fmt.Sprintf("%-10s", a.Type)) +
			prioStyle.Render(fmt.Sprintf("%-8s", a.Priority)) +
			fmt.Sprintf(" %-12s %s", a.What, body)

		if i == m.cursor {
			rows = append(rows, tableSelectedStyle.Width(m.width-4).Render(row))
		} else if i%2 == 0 {
			rows = append(rows, tableRowStyle.Render(row))
		} else {
			rows = append(rows, tableRowAltStyle.Render(row))
		}
	}

	for len(rows) < innerHeight {
		rows = append(rows, "")
	}

	footer := lipgloss.NewStyle().Foreground(colorDim).Render(
		"  [r]efresh  [↑↓] navigate")

	content := header + "\n" + strings.Join(rows, "\n") + "\n" + footer

	return panelStyle.Width(m.width).Render(
		panelTitleStyle.Render(fmt.Sprintf("Alerts (%d)", len(m.alerts))) + "\n" + content)
}
