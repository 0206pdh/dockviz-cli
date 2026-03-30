// keymap.go declares all keyboard shortcuts used in the TUI.
// Keeping keybindings in one place makes them easy to document and modify.
package tui

import "github.com/charmbracelet/bubbles/key"

// KeyMap holds all named key bindings.
type KeyMap struct {
	Up      key.Binding
	Down    key.Binding
	Enter   key.Binding
	Back    key.Binding
	Tab     key.Binding
	Refresh key.Binding
	Toggle  key.Binding // start/stop container
	Logs    key.Binding
	Delete  key.Binding // force-remove a container
	Chart   key.Binding // full-screen stats history chart
	Exec    key.Binding // open interactive shell in container
	Quit    key.Binding
}

// DefaultKeyMap returns the default key bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "move up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "move down"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "detail"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "switch panel"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
		Toggle: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "start/stop"),
		),
		Logs: key.NewBinding(
			key.WithKeys("l"),
			key.WithHelp("l", "logs"),
		),
		Delete: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "delete container"),
		),
		Chart: key.NewBinding(
			key.WithKeys("g"),
			key.WithHelp("g", "stats chart"),
		),
		Exec: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "exec shell"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
	}
}
