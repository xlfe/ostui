package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

type statusBarModel struct {
	width         int
	nodesOnline   int
	nodesTotal    int
	daemonVersion string
	uptime        uint64
	alert         string
	alertTime     time.Time
}

func (m *statusBarModel) View() string {
	left := ""
	if m.nodesOnline > 0 {
		left += statusOnlineStyle.Render("● online")
	} else {
		left += statusOfflineStyle.Render("● offline")
	}
	left += statusBarStyle.Render(fmt.Sprintf(" nodes: %d/%d", m.nodesOnline, m.nodesTotal))

	if m.daemonVersion != "" {
		left += statusBarStyle.Render(" │ v" + m.daemonVersion)
	}
	if m.uptime > 0 {
		left += statusBarStyle.Render(" │ up: " + formatUptime(m.uptime))
	}

	// Show recent alert briefly.
	if m.alert != "" && time.Since(m.alertTime) < 10*time.Second {
		left += statusBarStyle.Render(" │ ") +
			lipgloss.NewStyle().Foreground(colorYellow).Render("⚠ "+m.alert)
	}

	right := statusBarStyle.Render("?:help  q:quit")

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		gap = 0
	}

	return left + strings.Repeat(" ", gap) + right
}

func formatUptime(seconds uint64) string {
	d := seconds / 86400
	h := (seconds % 86400) / 3600
	m := (seconds % 3600) / 60
	if d > 0 {
		return fmt.Sprintf("%dd%dh", d, h)
	}
	if h > 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}
