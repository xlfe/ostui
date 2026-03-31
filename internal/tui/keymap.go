package tui

import "charm.land/bubbles/v2/key"

type keyMap struct {
	Tab1     key.Binding
	Tab2     key.Binding
	Tab3     key.Binding
	Tab4     key.Binding
	Tab5     key.Binding
	Help     key.Binding
	Quit     key.Binding
	Up       key.Binding
	Down     key.Binding
	Enter    key.Binding
	Esc      key.Binding
	Filter   key.Binding
	Add      key.Binding
	Edit     key.Binding
	Delete   key.Binding
	Toggle   key.Binding
	Tab      key.Binding
	ShiftTab key.Binding
	Space    key.Binding
	Export   key.Binding
}

var keys = keyMap{
	Tab1:     key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "Dashboard")),
	Tab2:     key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "Rules")),
	Tab3:     key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "Nodes")),
	Tab4:     key.NewBinding(key.WithKeys("4"), key.WithHelp("4", "Firewall")),
	Tab5:     key.NewBinding(key.WithKeys("5"), key.WithHelp("5", "Alerts")),
	Help:     key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "Help")),
	Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "Quit")),
	Up:       key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("k/up", "Up")),
	Down:     key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("j/down", "Down")),
	Enter:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "Select")),
	Esc:      key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "Back")),
	Filter:   key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "Filter")),
	Add:      key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "Add")),
	Edit:     key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "Edit")),
	Delete:   key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "Delete")),
	Toggle:   key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "Toggle")),
	Tab:      key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "Next")),
	ShiftTab: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "Prev")),
	Space:    key.NewBinding(key.WithKeys("space", " "), key.WithHelp("space", "Select")),
	Export:   key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "Export Nix")),
}
