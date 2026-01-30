package picker

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SingleModel is the Bubble Tea model for single-select picker
type SingleModel struct {
	title       string
	items       []Item
	cursor      int
	offset      int // scroll offset
	done        bool
	quitting    bool
	searchInput textinput.Model
	searching   bool
}

// NewSingle creates a new single-select picker model
func NewSingle(title string, items []Item) SingleModel {
	// Initialize search input
	ti := textinput.New()
	ti.Placeholder = "Type to search..."
	ti.CharLimit = 50
	ti.Width = 40

	// Find initially selected item
	cursor := 0
	for i, item := range items {
		if item.Selected {
			cursor = i
			break
		}
	}

	return SingleModel{
		title:       title,
		items:       items,
		cursor:      cursor,
		searchInput: ti,
	}
}

// Selected returns the ID of the selected item
func (m SingleModel) Selected() string {
	filteredItems := m.getFilteredItems()
	if len(filteredItems) > 0 && m.cursor < len(filteredItems) {
		return filteredItems[m.cursor].ID
	}
	return ""
}

// IsQuitting returns true if the user quit without confirming
func (m SingleModel) IsQuitting() bool {
	return m.quitting
}

// Init implements tea.Model
func (m SingleModel) Init() tea.Cmd {
	return nil
}

// getFilteredItems returns items matching the current search query
func (m SingleModel) getFilteredItems() []Item {
	if m.searchInput.Value() == "" {
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
func (m *SingleModel) adjustScroll() {
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
func (m SingleModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		case key.Matches(msg, singleKeys.Quit):
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, singleKeys.Search):
			m.searching = true
			m.searchInput.Focus()
			return m, textinput.Blink

		case key.Matches(msg, singleKeys.Up):
			filteredItems := m.getFilteredItems()
			if m.cursor > 0 {
				m.cursor--
				m.adjustScroll()
			} else if len(filteredItems) > 0 {
				m.cursor = len(filteredItems) - 1
				m.adjustScroll()
			}

		case key.Matches(msg, singleKeys.Down):
			filteredItems := m.getFilteredItems()
			if m.cursor < len(filteredItems)-1 {
				m.cursor++
				m.adjustScroll()
			} else if len(filteredItems) > 0 {
				m.cursor = 0
				m.offset = 0
			}

		case key.Matches(msg, singleKeys.Confirm):
			m.done = true
			return m, tea.Quit
		}
	}

	return m, nil
}

// View implements tea.Model
func (m SingleModel) View() string {
	if m.done || m.quitting {
		return ""
	}

	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("69"))
	cursorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	countStyle := lipgloss.NewStyle().Faint(true)
	scrollIndicatorStyle := lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("240"))

	b.WriteString(titleStyle.Render(m.title))
	b.WriteString("\n")

	// Search bar
	if m.searching {
		b.WriteString("\n")
		b.WriteString("🔍 ")
		b.WriteString(m.searchInput.View())
		b.WriteString("\n")
	} else if m.searchInput.Value() != "" {
		b.WriteString("\n")
		b.WriteString(countStyle.Render("Filter: " + m.searchInput.Value() + " (press / to edit, esc to clear)"))
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
			if i == m.cursor {
				b.WriteString(cursorStyle.Render("> "))
				b.WriteString(selectedStyle.Render(item.Label))
			} else {
				b.WriteString("  ")
				b.WriteString(item.Label)
			}
			b.WriteString("\n")
		}

		// Show scroll indicator at bottom
		remaining := len(filteredItems) - end
		if remaining > 0 {
			b.WriteString(scrollIndicatorStyle.Render(fmt.Sprintf("  ↓ %d more below", remaining)))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render("↑/↓: navigate • /: search • enter: select • q: quit"))

	return b.String()
}

// singleKeyMap defines the key bindings for single select
type singleKeyMap struct {
	Up      key.Binding
	Down    key.Binding
	Search  key.Binding
	Confirm key.Binding
	Quit    key.Binding
}

var singleKeys = singleKeyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
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

// RunSingle runs the single-select picker and returns selected item ID
func RunSingle(title string, items []Item) (string, error) {
	m := NewSingle(title, items)
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return "", err
	}

	fm := finalModel.(SingleModel)
	if fm.IsQuitting() {
		return "", nil
	}

	return fm.Selected(), nil
}
