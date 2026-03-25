package components

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

var (
	boxBorder = lipgloss.RoundedBorder()

	boxStyle = lipgloss.NewStyle().
			BorderStyle(boxBorder).
			BorderForeground(lipgloss.Color("#3b4261")).
			Padding(0, 1)

	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7aa2f7")).
			Bold(true)

	labelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#565f89"))

	valueStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#c0caf5")).
			Bold(true)

	greenStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9ece6a"))

	redStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#f7768e"))

	yellowStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#e0af68"))
)

// StatLine represents a label-value pair with optional bar.
type StatLine struct {
	Label string
	Value string
	Style lipgloss.Style
	Ratio float64 // 0.0-1.0 for bar rendering
}

// RenderStatBox renders a bordered box with a title and stat lines.
func RenderStatBox(title string, width, height int, lines []StatLine) string {
	innerWidth := width - 4 // borders + padding
	if innerWidth < 10 {
		innerWidth = 10
	}
	innerHeight := height - 2 // borders
	if innerHeight < 1 {
		innerHeight = 1
	}

	barWidth := 8
	var rows []string

	for i, line := range lines {
		if i >= innerHeight {
			break
		}
		label := labelStyle.Render(fmt.Sprintf("%-10s", line.Label))

		valStr := line.Style.Render(fmt.Sprintf("%8s", line.Value))

		bar := ""
		if line.Ratio > 0 {
			bar = " " + renderBar(line.Ratio, barWidth, line.Style)
		}

		row := label + valStr + bar
		rows = append(rows, row)
	}

	// Pad to fill height.
	for len(rows) < innerHeight {
		rows = append(rows, "")
	}

	content := strings.Join(rows, "\n")

	return boxStyle.
		Width(width).
		Height(innerHeight).
		Render(titleStyle.Render(title) + "\n" + content)
}

func renderBar(ratio float64, width int, style lipgloss.Style) string {
	if ratio > 1.0 {
		ratio = 1.0
	}
	filled := int(ratio * float64(width))
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	return style.Render(bar)
}
