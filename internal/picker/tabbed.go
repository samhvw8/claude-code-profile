package picker

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const maxVisibleItems = 10 // Maximum items to show before scrolling

// Tab represents a tab with items to select
type Tab struct {
	Name     string
	Items    []Item
	cursor   int
	offset   int // scroll offset
	selected map[string]bool
}

// TabbedModel is the Bubble Tea model for multi-tab multi-select picker
type TabbedModel struct {
	tabs        []Tab
	currentTab  int
	done        bool
	quitting    bool
	searchInput textinput.Model
	searching   bool
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

	// Initialize search input
	ti := textinput.New()
	ti.Placeholder = "Type to search..."
	ti.CharLimit = 50
	ti.Width = 40

	return TabbedModel{
		tabs:        tabs,
		searchInput: ti,
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

// getFilteredItems returns items matching the current search query
func (m TabbedModel) getFilteredItems(tab *Tab) []Item {
	if m.searchInput.Value() == "" {
		return tab.Items
	}

	query := strings.ToLower(m.searchInput.Value())
	var filtered []Item
	for _, item := range tab.Items {
		if strings.Contains(strings.ToLower(item.Label), query) ||
			strings.Contains(strings.ToLower(item.ID), query) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// adjustScroll ensures cursor is visible within the viewport
func (m *TabbedModel) adjustScroll() {
	tab := &m.tabs[m.currentTab]
	filteredItems := m.getFilteredItems(tab)
	itemCount := len(filteredItems)

	// Clamp cursor to valid range
	if tab.cursor >= itemCount {
		tab.cursor = itemCount - 1
	}
	if tab.cursor < 0 {
		tab.cursor = 0
	}

	// Adjust offset to keep cursor visible
	if tab.cursor < tab.offset {
		tab.offset = tab.cursor
	}
	if tab.cursor >= tab.offset+maxVisibleItems {
		tab.offset = tab.cursor - maxVisibleItems + 1
	}

	// Clamp offset
	maxOffset := itemCount - maxVisibleItems
	if maxOffset < 0 {
		maxOffset = 0
	}
	if tab.offset > maxOffset {
		tab.offset = maxOffset
	}
	if tab.offset < 0 {
		tab.offset = 0
	}
}

// Update implements tea.Model
func (m TabbedModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
				// Reset cursor and offset when exiting search
				tab := &m.tabs[m.currentTab]
				tab.cursor = 0
				tab.offset = 0
				return m, nil
			case "enter":
				// Exit search mode but keep filter
				m.searching = false
				m.searchInput.Blur()
				return m, nil
			default:
				m.searchInput, cmd = m.searchInput.Update(msg)
				// Reset cursor when search changes
				tab := &m.tabs[m.currentTab]
				tab.cursor = 0
				tab.offset = 0
				return m, cmd
			}
		}

		switch {
		case key.Matches(msg, tabbedKeys.Quit):
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, tabbedKeys.Search):
			m.searching = true
			m.searchInput.Focus()
			return m, textinput.Blink

		case key.Matches(msg, tabbedKeys.Left):
			if m.currentTab > 0 {
				m.currentTab--
				m.searchInput.SetValue("") // Clear search when switching tabs
			}

		case key.Matches(msg, tabbedKeys.Right):
			if m.currentTab < len(m.tabs)-1 {
				m.currentTab++
				m.searchInput.SetValue("") // Clear search when switching tabs
			}

		case key.Matches(msg, tabbedKeys.Up):
			tab := &m.tabs[m.currentTab]
			filteredItems := m.getFilteredItems(tab)
			if tab.cursor > 0 {
				tab.cursor--
				m.adjustScroll()
			} else if len(filteredItems) > 0 {
				// Wrap to bottom
				tab.cursor = len(filteredItems) - 1
				m.adjustScroll()
			}

		case key.Matches(msg, tabbedKeys.Down):
			tab := &m.tabs[m.currentTab]
			filteredItems := m.getFilteredItems(tab)
			if tab.cursor < len(filteredItems)-1 {
				tab.cursor++
				m.adjustScroll()
			} else if len(filteredItems) > 0 {
				// Wrap to top
				tab.cursor = 0
				tab.offset = 0
			}

		case key.Matches(msg, tabbedKeys.Toggle):
			tab := &m.tabs[m.currentTab]
			filteredItems := m.getFilteredItems(tab)
			if len(filteredItems) > 0 && tab.cursor < len(filteredItems) {
				id := filteredItems[tab.cursor].ID
				tab.selected[id] = !tab.selected[id]
			}

		case key.Matches(msg, tabbedKeys.All):
			tab := &m.tabs[m.currentTab]
			filteredItems := m.getFilteredItems(tab)
			allSelected := true
			for _, item := range filteredItems {
				if !tab.selected[item.ID] {
					allSelected = false
					break
				}
			}
			for _, item := range filteredItems {
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
	scrollIndicatorStyle := lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("240"))

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
	b.WriteString("\n")

	// Search bar (show when searching or has value)
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

	// Current tab items with scrolling
	tab := m.tabs[m.currentTab]
	filteredItems := m.getFilteredItems(&tab)

	if len(filteredItems) == 0 {
		if m.searchInput.Value() != "" {
			b.WriteString(lipgloss.NewStyle().Faint(true).Render("  (no matching items)"))
		} else {
			b.WriteString(lipgloss.NewStyle().Faint(true).Render("  (no items)"))
		}
		b.WriteString("\n")
	} else {
		// Show scroll indicator at top
		if tab.offset > 0 {
			b.WriteString(scrollIndicatorStyle.Render(fmt.Sprintf("  ↑ %d more above", tab.offset)))
			b.WriteString("\n")
		}

		// Calculate visible range
		start := tab.offset
		end := start + maxVisibleItems
		if end > len(filteredItems) {
			end = len(filteredItems)
		}

		// Render visible items
		for i := start; i < end; i++ {
			item := filteredItems[i]
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

		// Show scroll indicator at bottom
		remaining := len(filteredItems) - end
		if remaining > 0 {
			b.WriteString(scrollIndicatorStyle.Render(fmt.Sprintf("  ↓ %d more below", remaining)))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	helpText := "←/→: switch tab • ↑/↓: navigate • space: toggle • a: all/none • /: search • enter: confirm • q: quit"
	b.WriteString(lipgloss.NewStyle().Faint(true).Render(helpText))

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
	Search  key.Binding
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
