package prompt

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aaronflorey/bin/pkg/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss/v2"
)

type MultiSelectOption struct {
	Label string
	Value string
}

type MultiSelectItem struct {
	Value       string
	Label       string
	Description string
	Selected    bool
}

type multiSelectOption struct {
	value       string
	label       string
	description string
	selected    bool
}

type multiSelectModel struct {
	title   string
	hint    string
	options []multiSelectOption
	cursor  int
	abort   bool
	width   int
}

var (
	titleStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	hintStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	activeRowStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("24"))
	inactiveRowStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	descStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	footerStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
)

func MultiSelect(title string, options []MultiSelectOption) ([]string, error) {
	items := make([]MultiSelectItem, 0, len(options))
	for _, option := range options {
		items = append(items, MultiSelectItem{
			Value: option.Value,
			Label: option.Label,
		})
	}

	return MultiSelectItems(
		title,
		"up/down: move  space: toggle  a: toggle all  enter: confirm  q: abort",
		items,
	)
}

func MultiSelectItems(title, hint string, items []MultiSelectItem) ([]string, error) {
	if len(items) == 0 {
		return nil, nil
	}
	if !IsInteractive() {
		return nil, fmt.Errorf("interactive selection required")
	}

	resume := spinner.Pause()
	defer resume()

	model := multiSelectModel{title: title, hint: hint, options: make([]multiSelectOption, 0, len(items))}
	for _, item := range items {
		model.options = append(model.options, multiSelectOption{
			value:       item.Value,
			label:       item.Label,
			description: item.Description,
			selected:    item.Selected,
		})
	}

	p := tea.NewProgram(
		model,
		tea.WithInput(stdin),
		tea.WithOutput(os.Stdout),
		tea.WithAltScreen(),
	)

	finalModel, err := p.Run()
	if err != nil {
		if err == io.EOF {
			return nil, fmt.Errorf("command aborted")
		}
		return nil, err
	}

	result, ok := finalModel.(multiSelectModel)
	if !ok {
		return nil, fmt.Errorf("failed to collect selected items")
	}
	if result.abort {
		return nil, fmt.Errorf("command aborted")
	}

	selected := make([]string, 0, len(result.options))
	for _, option := range result.options {
		if option.selected {
			selected = append(selected, option.value)
		}
	}

	return selected, nil
}

func (m multiSelectModel) Init() tea.Cmd {
	return nil
}

func (m multiSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			m.abort = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.options)-1 {
				m.cursor++
			}
		case " ", "x":
			if len(m.options) > 0 {
				m.options[m.cursor].selected = !m.options[m.cursor].selected
			}
		case "a":
			allSelected := true
			for _, option := range m.options {
				if !option.selected {
					allSelected = false
					break
				}
			}
			for i := range m.options {
				m.options[i].selected = !allSelected
			}
		case "enter":
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m multiSelectModel) View() string {
	if len(m.options) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render(m.title))
	b.WriteString("\n")
	if m.hint != "" {
		b.WriteString(hintStyle.Render(m.hint))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	for i, option := range m.options {
		checkbox := "[ ]"
		if option.selected {
			checkbox = "[x]"
		}

		prefix := "  "
		if i == m.cursor {
			prefix = "> "
		}

		row := fmt.Sprintf("%s%s %s", prefix, checkbox, option.label)
		if option.description != "" {
			row = fmt.Sprintf("%s\n    %s", row, descStyle.Render(option.description))
		}

		style := inactiveRowStyle
		if i == m.cursor {
			style = activeRowStyle
		}

		if m.width > 0 {
			row = style.MaxWidth(m.width - 1).Render(row)
		} else {
			row = style.Render(row)
		}
		b.WriteString(row)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(footerStyle.Render("Selected items will be applied."))
	b.WriteString("\n")

	return b.String()
}
