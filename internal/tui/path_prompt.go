package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type pathPromptModel struct {
	title       string
	help        string
	input       textinput.Model
	value       string
	done        bool
	canceled    bool
	errorText   string
	initialPath string
}

func newPathPromptModel(title, help, initialPath string) pathPromptModel {
	ti := textinput.New()
	ti.Placeholder = "/path/to/obsidian/vault"
	ti.Prompt = "> "
	ti.SetValue(initialPath)
	ti.Focus()
	ti.CharLimit = 1024
	ti.Width = 70

	return pathPromptModel{
		title:       title,
		help:        help,
		input:       ti,
		initialPath: initialPath,
	}
}

func (m pathPromptModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m pathPromptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.canceled = true
			m.done = true
			return m, tea.Quit
		case "enter":
			value := strings.TrimSpace(m.input.Value())
			if value == "" {
				m.errorText = "Vault path is required."
				return m, nil
			}
			m.value = value
			m.done = true
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m pathPromptModel) View() string {
	if m.done {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(titleStyle.Render(m.title))
	b.WriteString("\n\n")
	b.WriteString(inputStyle.Render(m.input.View()))
	b.WriteString("\n\n")
	if m.errorText != "" {
		b.WriteString(errorStyle.Render("  " + m.errorText))
		b.WriteString("\n\n")
	}
	b.WriteString(dimStyle.Render(m.help))
	b.WriteString("\n")
	return b.String()
}

// RunPathPrompt shows an interactive text input prompt and returns the entered path.
func RunPathPrompt(title, help, initialPath string) (string, error) {
	model := newPathPromptModel(title, help, initialPath)
	p := tea.NewProgram(model)
	finalModel, err := p.Run()
	if err != nil {
		return "", err
	}
	result := finalModel.(pathPromptModel)
	if result.canceled {
		return "", fmt.Errorf("prompt canceled")
	}
	return strings.TrimSpace(result.value), nil
}
