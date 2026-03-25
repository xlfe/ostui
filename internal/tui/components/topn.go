package components

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

var (
	topnBoxStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#3b4261")).
			Padding(0, 1)

	topnTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7aa2f7")).
			Bold(true)

	topnNameStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#c0caf5"))

	topnCountStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7dcfff")).
			Bold(true)
)

// TopNEntry is a name-count pair.
type TopNEntry struct {
	Name  string
	Count uint64
}

// RenderTopN renders a bordered panel showing top-N entries.
func RenderTopN(title string, width, height int, entries []TopNEntry) string {
	innerWidth := width - 4
	if innerWidth < 10 {
		innerWidth = 10
	}
	innerHeight := height - 2
	if innerHeight < 1 {
		innerHeight = 1
	}

	nameWidth := innerWidth - 10
	if nameWidth < 5 {
		nameWidth = 5
	}

	var rows []string
	for i, e := range entries {
		if i >= innerHeight {
			break
		}
		name := e.Name
		if len(name) > nameWidth {
			name = name[:nameWidth-1] + "…"
		}
		row := topnNameStyle.Render(fmt.Sprintf("%-*s", nameWidth, name)) + " " +
			topnCountStyle.Render(fmt.Sprintf("%8s", FormatCount(e.Count)))
		rows = append(rows, row)
	}

	for len(rows) < innerHeight {
		rows = append(rows, "")
	}

	content := strings.Join(rows, "\n")

	return topnBoxStyle.
		Width(width).
		Height(innerHeight).
		Render(topnTitleStyle.Render(title) + "\n" + content)
}

func FormatCount(n uint64) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}
