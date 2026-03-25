package components

import (
	"fmt"

	"charm.land/lipgloss/v2"
)

var countdownStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#e0af68")).
	Bold(true)

// RenderCountdown renders a countdown timer display.
func RenderCountdown(seconds int) string {
	if seconds <= 5 {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color("#f7768e")).
			Bold(true).
			Render(fmt.Sprintf("%ds", seconds))
	}
	return countdownStyle.Render(fmt.Sprintf("%ds", seconds))
}
