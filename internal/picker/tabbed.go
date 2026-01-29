package picker

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Tab represents a tab with items to select
type Tab struct {
	Name     string
	Items    []Item
	cursor   int
	selected map[string]bool
}

// TabbedModel is the Bubble Tea model for multi-tab multi-select picker
type TabbedModel struct {
	tabs       []Tab
	currentTab int
	done       bool
	quitting   bool
}

// NewTabbed creates a new tabbed picker model
func NewTabbed(tabs []Tab) TabbedModel {
	// Initialize selected maps for each tab
	for i := range tabs {
		tabs[i].selected = make(map[string]bool)
		for _, item := range tabs[i].Items {
			if item.Selected {
				tabs[i].selected[item.ID] = true
			}
		}
	}

	return TabbedModel{
		tabs: tabs,
	}
}

// GetTabSelections returns the selected items for each tab by name
func (m TabbedModel) GetTabSelections() map[string][]string {
	result := make(map[string][]string)
	for _, tab := range m.tabs {
		var selected []string
		for _, item := range tab.Items {
			if tab.selected[item.ID] {
				selected = append(selected, item.ID)
			}
		}
		result[tab.Name] = selected
	}
	return result
}

// IsQuitting returns true if the user quit without confirming
func (m TabbedModel) IsQuitting() bool {
	return m.quitting
}

// Init implements tea.Model
func (m TabbedModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model
func (m TabbedModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, tabbedKeys.Quit):
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, tabbedKeys.Left):
			if m.currentTab > 0 {
				m.currentTab--
			}

		case key.Matches(msg, tabbedKeys.Right):
			if m.currentTab < len(m.tabs)-1 {
				m.currentTab++
			}

		case key.Matches(msg, tabbedKeys.Up):
			tab := &m.tabs[m.currentTab]
			if tab.cursor > 0 {
				tab.cursor--
			}

		case key.Matches(msg, tabbedKeys.Down):
			tab := &m.tabs[m.currentTab]
			if tab.cursor < len(tab.Items)-1 {
				tab.cursor++
			}

		case key.Matches(msg, tabbedKeys.Toggle):
			tab := &m.tabs[m.currentTab]
			if len(tab.Items) > 0 {
				id := tab.Items[tab.cursor].ID
				tab.selected[id] = !tab.selected[id]
			}

		case key.Matches(msg, tabbedKeys.All):
			tab := &m.tabs[m.currentTab]
			allSelected := true
			for _, item := range tab.Items {
				if !tab.selected[item.ID] {
					allSelected = false
					break
				}
			}
			for _, item := range tab.Items {
				tab.selected[item.ID] = !allSelected
			}

		case key.Matches(msg, tabbedKeys.Confirm):
			m.done = true
			return m, tea.Quit
		}
	}

	return m, nil
}

// View implements tea.Model
func (m TabbedModel) View() string {
	if m.done || m.quitting {
		return ""
	}

	var b strings.Builder

	// Styles
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("69"))
	activeTabStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")).Underline(true)
	inactiveTabStyle := lipgloss.NewStyle().Faint(true)
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	cursorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
	countStyle := lipgloss.NewStyle().Faint(true)

	// Title
	b.WriteString(titleStyle.Render("Create Profile - Select Items"))
	b.WriteString("\n\n")

	// Tab bar
	var tabNames []string
	for i, tab := range m.tabs {
		// Count selected items
		count := 0
		for _, item := range tab.Items {
			if tab.selected[item.ID] {
				count++
			}
		}
		countStr := countStyle.Render(fmt.Sprintf("(%d/%d)", count, len(tab.Items)))

		if i == m.currentTab {
			tabNames = append(tabNames, activeTabStyle.Render(tab.Name)+" "+countStr)
		} else {
			tabNames = append(tabNames, inactiveTabStyle.Render(tab.Name)+" "+countStr)
		}
	}
	b.WriteString(strings.Join(tabNames, "  │  "))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", 60))
	b.WriteString("\n\n")

	// Current tab items
	tab := m.tabs[m.currentTab]
	if len(tab.Items) == 0 {
		b.WriteString(lipgloss.NewStyle().Faint(true).Render("  (no items)"))
		b.WriteString("\n")
	} else {
		for i, item := range tab.Items {
			cursor := "  "
			if i == tab.cursor {
				cursor = cursorStyle.Render("> ")
			}

			checked := "[ ]"
			if tab.selected[item.ID] {
				checked = selectedStyle.Render("[x]")
			}

			b.WriteString(fmt.Sprintf("%s%s %s\n", cursor, checked, item.Label))
		}
	}

	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render("←/→: switch tab • ↑/↓: navigate • space: toggle • a: all/none • enter: confirm • q: quit"))

	return b.String()
}

// TabbedKeyMap defines the key bindings for tabbed picker
type tabbedKeyMap struct {
	Up      key.Binding
	Down    key.Binding
	Left    key.Binding
	Right   key.Binding
	Toggle  key.Binding
	All     key.Binding
	Confirm key.Binding
	Quit    key.Binding
}

var tabbedKeys = tabbedKeyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
	),
	Left: key.NewBinding(
		key.WithKeys("left", "h"),
	),
	Right: key.NewBinding(
		key.WithKeys("right", "l"),
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

// RunTabbed runs the tabbed picker and returns selected items per tab
func RunTabbed(tabs []Tab) (map[string][]string, error) {
	m := NewTabbed(tabs)
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}

	fm := finalModel.(TabbedModel)
	if fm.IsQuitting() {
		return nil, nil
	}

	return fm.GetTabSelections(), nil
}
