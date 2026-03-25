package tui

// Layout holds computed panel dimensions based on terminal size.
type Layout struct {
	Width  int
	Height int

	// Top row: Stats (left) | Top-N (right).
	TopRowHeight int
	StatsWidth   int
	TopNWidth    int

	// Bottom: grouped recent connections.
	BottomHeight int

	// Tab bar and status bar.
	TabBarHeight    int
	StatusBarHeight int

	// Content area (excluding tab + status).
	ContentHeight int
}

// ComputeLayout calculates panel sizes for the given terminal dimensions.
func ComputeLayout(width, height int) Layout {
	l := Layout{
		Width:           width,
		Height:          height,
		TabBarHeight:    1,
		StatusBarHeight: 1,
	}

	l.ContentHeight = height - l.TabBarHeight - l.StatusBarHeight
	if l.ContentHeight < 10 {
		l.ContentHeight = 10
	}

	// Top row gets ~35% of content height.
	l.TopRowHeight = l.ContentHeight * 35 / 100
	if l.TopRowHeight < 8 {
		l.TopRowHeight = 8
	}

	// Bottom row gets the rest.
	l.BottomHeight = l.ContentHeight - l.TopRowHeight
	if l.BottomHeight < 6 {
		l.BottomHeight = 6
	}

	// Two-column split: stats ~40%, top-N ~60%.
	l.StatsWidth = width * 40 / 100
	l.TopNWidth = width - l.StatsWidth

	return l
}
