package tui

import "charm.land/lipgloss/v2"

// btop-inspired color palette.
var (
	colorBg        = lipgloss.Color("#1a1b26")
	colorFg        = lipgloss.Color("#c0caf5")
	colorDim       = lipgloss.Color("#565f89")
	colorBorder    = lipgloss.Color("#3b4261")
	colorAccent    = lipgloss.Color("#7aa2f7")
	colorGreen     = lipgloss.Color("#9ece6a")
	colorRed       = lipgloss.Color("#f7768e")
	colorYellow    = lipgloss.Color("#e0af68")
	colorCyan      = lipgloss.Color("#7dcfff")
	colorMagenta   = lipgloss.Color("#bb9af7")
	colorOrange    = lipgloss.Color("#ff9e64")
	colorWhite     = lipgloss.Color("#ffffff")
	colorBlack     = lipgloss.Color("#15161e")
	colorTabActive = lipgloss.Color("#7aa2f7")
	colorTabInact  = lipgloss.Color("#3b4261")
)

// Panel styles.
var (
	panelStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1)

	panelTitleStyle = lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true)

	panelActiveStyle = panelStyle.
				BorderForeground(colorAccent)
)

// Status bar.
var (
	statusBarStyle = lipgloss.NewStyle().
			Foreground(colorDim).
			Padding(0, 1)

	statusOnlineStyle = lipgloss.NewStyle().
				Foreground(colorGreen).
				Bold(true)

	statusOfflineStyle = lipgloss.NewStyle().
				Foreground(colorRed).
				Bold(true)
)

// Tab bar.
var (
	tabActiveStyle = lipgloss.NewStyle().
			Foreground(colorBlack).
			Background(colorTabActive).
			Padding(0, 1).
			Bold(true)

	tabInactiveStyle = lipgloss.NewStyle().
				Foreground(colorFg).
				Background(colorTabInact).
				Padding(0, 1)
)

// Table styles.
var (
	tableHeaderStyle = lipgloss.NewStyle().
				Foreground(colorAccent).
				Bold(true)

	tableRowStyle = lipgloss.NewStyle().
			Foreground(colorFg)

	tableRowAltStyle = lipgloss.NewStyle().
				Foreground(colorDim)

	tableSelectedStyle = lipgloss.NewStyle().
				Foreground(colorBlack).
				Background(colorAccent).
				Bold(true)
)

// Stat display.
var (
	statLabelStyle = lipgloss.NewStyle().
			Foreground(colorDim)

	statValueStyle = lipgloss.NewStyle().
			Foreground(colorFg).
			Bold(true)

	statAcceptStyle = lipgloss.NewStyle().
			Foreground(colorGreen)

	statDropStyle = lipgloss.NewStyle().
			Foreground(colorRed)

	statIgnoreStyle = lipgloss.NewStyle().
			Foreground(colorYellow)
)

// Prompt modal.
var (
	promptStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.DoubleBorder()).
			BorderForeground(colorYellow).
			Padding(1, 2).
			Width(66)

	promptTitleStyle = lipgloss.NewStyle().
				Foreground(colorYellow).
				Bold(true)

	promptLabelStyle = lipgloss.NewStyle().
				Foreground(colorDim).
				Width(10)

	promptValueStyle = lipgloss.NewStyle().
				Foreground(colorFg)

	promptActionAllowStyle = lipgloss.NewStyle().
				Foreground(colorGreen).
				Bold(true)

	promptActionDenyStyle = lipgloss.NewStyle().
				Foreground(colorRed).
				Bold(true)

	promptActionRejectStyle = lipgloss.NewStyle().
				Foreground(colorOrange).
				Bold(true)

	promptSelectedStyle = lipgloss.NewStyle().
				Foreground(colorBlack).
				Background(colorAccent)

	promptCountdownStyle = lipgloss.NewStyle().
				Foreground(colorYellow).
				Bold(true)
)

// Bar chart characters for sparkline-style stat bars.
var barChars = []rune{'▏', '▎', '▍', '▌', '▋', '▊', '▉', '█'}

// renderBar renders a proportional bar with the given width.
func renderBar(ratio float64, width int, style lipgloss.Style) string {
	if width <= 0 {
		return ""
	}
	if ratio > 1.0 {
		ratio = 1.0
	}
	if ratio < 0 {
		ratio = 0
	}
	filled := int(ratio * float64(width))
	bar := ""
	for i := 0; i < filled; i++ {
		bar += string(barChars[7])
	}
	for i := filled; i < width; i++ {
		bar += " "
	}
	return style.Render(bar)
}

// Help overlay.
var (
	helpStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(colorAccent).
			Padding(1, 2)

	helpKeyStyle = lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true).
			Width(14)

	helpDescStyle = lipgloss.NewStyle().
			Foreground(colorFg)
)
