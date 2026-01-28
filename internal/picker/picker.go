package picker

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Item represents a selectable item
type Item struct {
	ID       string
	Label    string
	Selected bool
}

// Model is the Bubble Tea model for multi-select picker
type Model struct {
	title    string
	items    []Item
	cursor   int
	selected map[string]bool
	done     bool
	quitting bool
}

// New creates a new picker model
func New(title string, items []Item) Model {
	selected := make(map[string]bool)
	for _, item := range items {
		if item.Selected {
			selected[item.ID] = true
		}
	}

	return Model{
		title:    title,
		items:    items,
		selected: selected,
	}
}

// Selected returns the IDs of selected items
func (m Model) Selected() []string {
	var result []string
	for _, item := range m.items {
		if m.selected[item.ID] {
			result = append(result, item.ID)
		}
	}
	return result
}

// IsQuitting returns true if the user quit without confirming
func (m Model) IsQuitting() bool {
	return m.quitting
}

// Init implements tea.Model
func (m Model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Quit):
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}

		case key.Matches(msg, keys.Down):
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}

		case key.Matches(msg, keys.Toggle):
			if len(m.items) > 0 {
				id := m.items[m.cursor].ID
				m.selected[id] = !m.selected[id]
			}

		case key.Matches(msg, keys.All):
			// Toggle all
			allSelected := true
			for _, item := range m.items {
				if !m.selected[item.ID] {
					allSelected = false
					break
				}
			}
			for _, item := range m.items {
				m.selected[item.ID] = !allSelected
			}

		case key.Matches(msg, keys.Confirm):
			m.done = true
			return m, tea.Quit
		}
	}

	return m, nil
}

// View implements tea.Model
func (m Model) View() string {
	if m.done || m.quitting {
		return ""
	}

	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("69"))
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	cursorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("212"))

	b.WriteString(titleStyle.Render(m.title))
	b.WriteString("\n\n")

	for i, item := range m.items {
		cursor := "  "
		if i == m.cursor {
			cursor = cursorStyle.Render("> ")
		}

		checked := "[ ]"
		if m.selected[item.ID] {
			checked = selectedStyle.Render("[x]")
		}

		b.WriteString(fmt.Sprintf("%s%s %s\n", cursor, checked, item.Label))
	}

	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render("space: toggle • a: all/none • enter: confirm • q: quit"))

	return b.String()
}

// KeyMap defines the key bindings
type keyMap struct {
	Up      key.Binding
	Down    key.Binding
	Toggle  key.Binding
	All     key.Binding
	Confirm key.Binding
	Quit    key.Binding
}

var keys = keyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
	),
	Toggle: key.NewBinding(
		key.WithKeys(" "),
	),
	All: key.NewBinding(
		key.WithKeys("a"),
	),
	Confirm: key.NewBinding(
		key.WithKeys("enter"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
	),
}

// Run runs the picker and returns selected item IDs
func Run(title string, items []Item) ([]string, error) {
	m := New(title, items)
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}

	fm := finalModel.(Model)
	if fm.IsQuitting() {
		return nil, nil
	}

	return fm.Selected(), nil
}
