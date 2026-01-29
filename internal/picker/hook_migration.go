package picker

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/samhoang/ccp/internal/config"
)

// HookMigrationChoice represents user's choice for handling an outside hook
type HookMigrationChoice int

const (
	HookMigrationCopy HookMigrationChoice = iota // Copy to hub
	HookMigrationSkip                            // Skip (remove from settings)
	HookMigrationKeep                            // Keep external reference
)

// HookMigrationItem represents an outside hook pending user decision
type HookMigrationItem struct {
	Name       string
	FilePath   string
	HookType   config.HookType
	Matcher    string
	ParentDirs []string
	Choice     HookMigrationChoice
	Selected   bool
}

// HookMigrationModel is the Bubble Tea model for hook migration TUI
type HookMigrationModel struct {
	title    string
	items    []HookMigrationItem
	cursor   int
	done     bool
	quitting bool
}

// NewHookMigrationPicker creates a new hook migration picker
func NewHookMigrationPicker(title string, items []HookMigrationItem) HookMigrationModel {
	return HookMigrationModel{
		title: title,
		items: items,
	}
}

func (m HookMigrationModel) Init() tea.Cmd {
	return nil
}

func (m HookMigrationModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, hookMigrationKeys.Quit):
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, hookMigrationKeys.Up):
			if m.cursor > 0 {
				m.cursor--
			}

		case key.Matches(msg, hookMigrationKeys.Down):
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}

		case key.Matches(msg, hookMigrationKeys.Toggle):
			if len(m.items) > 0 {
				m.items[m.cursor].Selected = !m.items[m.cursor].Selected
			}

		case key.Matches(msg, hookMigrationKeys.Copy):
			m.applyChoiceToSelected(HookMigrationCopy)

		case key.Matches(msg, hookMigrationKeys.Skip):
			m.applyChoiceToSelected(HookMigrationSkip)

		case key.Matches(msg, hookMigrationKeys.Keep):
			m.applyChoiceToSelected(HookMigrationKeep)

		case key.Matches(msg, hookMigrationKeys.SelectAll):
			for i := range m.items {
				m.items[i].Selected = true
			}

		case key.Matches(msg, hookMigrationKeys.Confirm):
			m.done = true
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m *HookMigrationModel) applyChoiceToSelected(choice HookMigrationChoice) {
	hasSelected := false
	for _, item := range m.items {
		if item.Selected {
			hasSelected = true
			break
		}
	}

	if hasSelected {
		// Apply to all selected items
		for i := range m.items {
			if m.items[i].Selected {
				m.items[i].Choice = choice
			}
		}
	} else if len(m.items) > 0 {
		// Apply to current item only
		m.items[m.cursor].Choice = choice
	}
}

func (m HookMigrationModel) View() string {
	if m.done || m.quitting {
		return ""
	}

	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("69"))
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	cursorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
	pathStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	typeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	warningStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	dimStyle := lipgloss.NewStyle().Faint(true)
	copyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	skipStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	keepStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))

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

		// Choice indicator
		var choiceStr string
		switch item.Choice {
		case HookMigrationCopy:
			choiceStr = copyStyle.Render("[COPY]")
		case HookMigrationSkip:
			choiceStr = skipStyle.Render("[SKIP]")
		case HookMigrationKeep:
			choiceStr = keepStyle.Render("[KEEP]")
		}

		typeStr := typeStyle.Render(fmt.Sprintf("[%s]", item.HookType))
		if item.Matcher != "" {
			typeStr = typeStyle.Render(fmt.Sprintf("[%s:%s]", item.HookType, item.Matcher))
		}

		b.WriteString(fmt.Sprintf("%s%s %s %s %s\n",
			cursor, checked, pathStyle.Render(item.FilePath), typeStr, choiceStr))

		// Show warning about parent dirs
		if len(item.ParentDirs) > 0 {
			parentDir := filepath.Dir(item.FilePath)
			baseName := filepath.Base(item.FilePath)
			b.WriteString(fmt.Sprintf("      %s Will copy: %s (from: %s)\n",
				warningStyle.Render("⚠"),
				baseName,
				parentDir))
		}
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render("space: toggle • a: select all • c: copy • s: skip • k: keep • enter: confirm • q: quit"))

	return b.String()
}

func (m HookMigrationModel) IsQuitting() bool {
	return m.quitting
}

func (m HookMigrationModel) GetDecisions() []HookMigrationItem {
	return m.items
}

type hookMigrationKeyMap struct {
	Up        key.Binding
	Down      key.Binding
	Toggle    key.Binding
	Copy      key.Binding
	Skip      key.Binding
	Keep      key.Binding
	SelectAll key.Binding
	Confirm   key.Binding
	Quit      key.Binding
}

var hookMigrationKeys = hookMigrationKeyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
	),
	Toggle: key.NewBinding(
		key.WithKeys(" "),
	),
	Copy: key.NewBinding(
		key.WithKeys("c"),
	),
	Skip: key.NewBinding(
		key.WithKeys("s"),
	),
	Keep: key.NewBinding(
		key.WithKeys("k"),
	),
	SelectAll: key.NewBinding(
		key.WithKeys("a"),
	),
	Confirm: key.NewBinding(
		key.WithKeys("enter"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
	),
}

// RunHookMigrationPicker runs the hook migration picker and returns user decisions
func RunHookMigrationPicker(title string, items []HookMigrationItem) ([]HookMigrationItem, error) {
	m := NewHookMigrationPicker(title, items)
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}

	fm := finalModel.(HookMigrationModel)
	if fm.IsQuitting() {
		return nil, nil
	}

	return fm.GetDecisions(), nil
}
