package tui

import (
	"charm.land/lipgloss/v2"
)

type firewallModel struct {
	width, height int
}

func newFirewallModel() *firewallModel {
	return &firewallModel{}
}

func (m *firewallModel) View() string {
	content := lipgloss.NewStyle().Foreground(colorDim).Render(
		"Firewall management - view and edit nftables chains and rules.\n\n" +
			"This view will show system firewall rules received from connected daemons.")

	return panelStyle.Width(m.width).Render(
		panelTitleStyle.Render("Firewall") + "\n" + content)
}
