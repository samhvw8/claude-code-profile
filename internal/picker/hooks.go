package picker

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/samhoang/ccp/internal/config"
)

// HookItem represents a hook with its configuration
type HookItem struct {
	Name     string
	Type     config.HookType
	Selected bool
}

// HookPickerModel is the Bubble Tea model for hook configuration
type HookPickerModel struct {
	title      string
	items      []HookItem
	cursor     int
	editingType int // -1 if not editing, otherwise index
	done       bool
	quitting   bool
}

// NewHookPicker creates a new hook picker
func NewHookPicker(title string, hooks []HookItem) HookPickerModel {
	return HookPickerModel{
		title:       title,
		items:       hooks,
		editingType: -1,
	}
}

func (m HookPickerModel) Init() tea.Cmd {
	return nil
}

func (m HookPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, hookKeys.Quit):
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, hookKeys.Up):
			if m.cursor > 0 {
				m.cursor--
			}

		case key.Matches(msg, hookKeys.Down):
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}

		case key.Matches(msg, hookKeys.Toggle):
			if len(m.items) > 0 {
				m.items[m.cursor].Selected = !m.items[m.cursor].Selected
			}

		case key.Matches(msg, hookKeys.CycleType):
			if len(m.items) > 0 && m.items[m.cursor].Selected {
				// Cycle through hook types
				types := config.AllHookTypes()
				currentIdx := 0
				for i, t := range types {
					if t == m.items[m.cursor].Type {
						currentIdx = i
						break
					}
				}
				nextIdx := (currentIdx + 1) % len(types)
				m.items[m.cursor].Type = types[nextIdx]
			}

		case key.Matches(msg, hookKeys.Confirm):
			m.done = true
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m HookPickerModel) View() string {
	if m.done || m.quitting {
		return ""
	}

	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("69"))
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	cursorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
	typeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	dimStyle := lipgloss.NewStyle().Faint(true)

	b.WriteString(titleStyle.Render(m.title))
	b.WriteString("\n\n")

	for i, item := range m.items {
		cursor := "  "
		if i == m.cursor {
			cursor = cursorStyle.Render("> ")
		}

		checked := "[ ]"
		if item.Selected {
			checked = selectedStyle.Render("[x]")
		}

		typeStr := dimStyle.Render("(not selected)")
		if item.Selected {
			typeStr = typeStyle.Render(fmt.Sprintf("[%s]", item.Type))
		}

		b.WriteString(fmt.Sprintf("%s%s %s %s\n", cursor, checked, item.Name, typeStr))
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render("space: toggle • t: cycle hook type • enter: confirm • q: quit"))

	return b.String()
}

func (m HookPickerModel) IsQuitting() bool {
	return m.quitting
}

func (m HookPickerModel) GetSelectedHooks() []HookItem {
	var result []HookItem
	for _, item := range m.items {
		if item.Selected {
			result = append(result, item)
		}
	}
	return result
}

type hookKeyMap struct {
	Up        key.Binding
	Down      key.Binding
	Toggle    key.Binding
	CycleType key.Binding
	Confirm   key.Binding
	Quit      key.Binding
}

var hookKeys = hookKeyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
	),
	Toggle: key.NewBinding(
		key.WithKeys(" "),
	),
	CycleType: key.NewBinding(
		key.WithKeys("t"),
	),
	Confirm: key.NewBinding(
		key.WithKeys("enter"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
	),
}

// RunHookPicker runs the hook picker and returns selected hooks with types
func RunHookPicker(title string, hooks []HookItem) ([]HookItem, error) {
	m := NewHookPicker(title, hooks)
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}

	fm := finalModel.(HookPickerModel)
	if fm.IsQuitting() {
		return nil, nil
	}

	return fm.GetSelectedHooks(), nil
}
