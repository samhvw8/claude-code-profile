package picker

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
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
	title       string
	items       []Item
	cursor      int
	offset      int // scroll offset
	selected    map[string]bool
	done        bool
	quitting    bool
	searchInput textinput.Model
	searching   bool
}

// New creates a new picker model
func New(title string, items []Item) Model {
	selected := make(map[string]bool)
	for _, item := range items {
		if item.Selected {
			selected[item.ID] = true
		}
	}

	// Initialize search input
	ti := textinput.New()
	ti.Placeholder = "Type to search..."
	ti.CharLimit = 50
	ti.Width = 40

	return Model{
		title:       title,
		items:       items,
		selected:    selected,
		searchInput: ti,
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

// getFilteredItems returns items matching the current search query
func (m Model) getFilteredItems() []Item {
	if !m.searching || m.searchInput.Value() == "" {
		return m.items
	}

	query := strings.ToLower(m.searchInput.Value())
	var filtered []Item
	for _, item := range m.items {
		if strings.Contains(strings.ToLower(item.Label), query) ||
			strings.Contains(strings.ToLower(item.ID), query) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// adjustScroll ensures cursor is visible within the viewport
func (m *Model) adjustScroll() {
	filteredItems := m.getFilteredItems()
	itemCount := len(filteredItems)

	// Clamp cursor to valid range
	if m.cursor >= itemCount {
		m.cursor = itemCount - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}

	// Adjust offset to keep cursor visible
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+maxVisibleItems {
		m.offset = m.cursor - maxVisibleItems + 1
	}

	// Clamp offset
	maxOffset := itemCount - maxVisibleItems
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.offset > maxOffset {
		m.offset = maxOffset
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

// Update implements tea.Model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Handle search mode
		if m.searching {
			switch msg.String() {
			case "esc":
				m.searching = false
				m.searchInput.SetValue("")
				m.searchInput.Blur()
				m.cursor = 0
				m.offset = 0
				return m, nil
			case "enter":
				m.searching = false
				m.searchInput.Blur()
				return m, nil
			default:
				m.searchInput, cmd = m.searchInput.Update(msg)
				m.cursor = 0
				m.offset = 0
				return m, cmd
			}
		}

		switch {
		case key.Matches(msg, keys.Quit):
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, keys.Search):
			m.searching = true
			m.searchInput.Focus()
			return m, textinput.Blink

		case key.Matches(msg, keys.Up):
			filteredItems := m.getFilteredItems()
			if m.cursor > 0 {
				m.cursor--
				m.adjustScroll()
			} else if len(filteredItems) > 0 {
				m.cursor = len(filteredItems) - 1
				m.adjustScroll()
			}

		case key.Matches(msg, keys.Down):
			filteredItems := m.getFilteredItems()
			if m.cursor < len(filteredItems)-1 {
				m.cursor++
				m.adjustScroll()
			} else if len(filteredItems) > 0 {
				m.cursor = 0
				m.offset = 0
			}

		case key.Matches(msg, keys.Toggle):
			filteredItems := m.getFilteredItems()
			if len(filteredItems) > 0 && m.cursor < len(filteredItems) {
				id := filteredItems[m.cursor].ID
				m.selected[id] = !m.selected[id]
			}

		case key.Matches(msg, keys.All):
			filteredItems := m.getFilteredItems()
			allSelected := true
			for _, item := range filteredItems {
				if !m.selected[item.ID] {
					allSelected = false
					break
				}
			}
			for _, item := range filteredItems {
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
	countStyle := lipgloss.NewStyle().Faint(true)
	scrollIndicatorStyle := lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("240"))

	// Count selected
	selectedCount := 0
	for _, item := range m.items {
		if m.selected[item.ID] {
			selectedCount++
		}
	}

	b.WriteString(titleStyle.Render(m.title))
	b.WriteString(" ")
	b.WriteString(countStyle.Render(fmt.Sprintf("(%d/%d selected)", selectedCount, len(m.items))))
	b.WriteString("\n")

	// Search bar
	if m.searching {
		b.WriteString("\n")
		b.WriteString("🔍 ")
		b.WriteString(m.searchInput.View())
		b.WriteString("\n")
	} else if m.searchInput.Value() != "" {
		b.WriteString("\n")
		b.WriteString(countStyle.Render(fmt.Sprintf("Filter: %s (press / to edit, esc to clear)", m.searchInput.Value())))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	filteredItems := m.getFilteredItems()

	if len(filteredItems) == 0 {
		if m.searchInput.Value() != "" {
			b.WriteString(lipgloss.NewStyle().Faint(true).Render("  (no matching items)"))
		} else {
			b.WriteString(lipgloss.NewStyle().Faint(true).Render("  (no items)"))
		}
		b.WriteString("\n")
	} else {
		// Show scroll indicator at top
		if m.offset > 0 {
			b.WriteString(scrollIndicatorStyle.Render(fmt.Sprintf("  ↑ %d more above", m.offset)))
			b.WriteString("\n")
		}

		// Calculate visible range
		start := m.offset
		end := start + maxVisibleItems
		if end > len(filteredItems) {
			end = len(filteredItems)
		}

		// Render visible items
		for i := start; i < end; i++ {
			item := filteredItems[i]
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

		// Show scroll indicator at bottom
		remaining := len(filteredItems) - end
		if remaining > 0 {
			b.WriteString(scrollIndicatorStyle.Render(fmt.Sprintf("  ↓ %d more below", remaining)))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render("↑/↓: navigate • space: toggle • a: all/none • /: search • enter: confirm • q: quit"))

	return b.String()
}

// KeyMap defines the key bindings
type keyMap struct {
	Up      key.Binding
	Down    key.Binding
	Toggle  key.Binding
	All     key.Binding
	Search  key.Binding
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
	Search: key.NewBinding(
		key.WithKeys("/"),
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
