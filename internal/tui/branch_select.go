package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ishaan812/devlog/internal/git"
)

// Branch selection styles
var (
	bsTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("39")).
			Padding(0, 0, 1, 2)

	bsSubtitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Padding(0, 0, 0, 2)

	bsItemStyle = lipgloss.NewStyle().
			Padding(0, 0, 0, 2)

	bsSelectedItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("39")).
				Padding(0, 0, 0, 2)

	bsCheckedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))

	bsUncheckedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	bsMainBranchStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("214")).
				Bold(true)

	bsHelpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Padding(1, 0, 0, 2)

	bsStepStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")).
			Bold(true).
			Padding(0, 0, 0, 2)
)

// BranchSelection is the result of branch selection
type BranchSelection struct {
	MainBranch       string
	SelectedBranches []string
	Cancelled        bool
}

// BranchSelectModel is the Bubbletea model for branch selection
type BranchSelectModel struct {
	branches        []git.BranchInfo
	cursor          int
	selected        map[int]bool
	mainBranchIdx   int
	detectedDefault string

	// Steps: 0 = select main, 1 = select feature branches
	step int

	width  int
	height int

	result BranchSelection
	done   bool
}

// NewBranchSelectModel creates a new branch selection model
func NewBranchSelectModel(branches []git.BranchInfo, detectedDefault string) BranchSelectModel {
	// Find the detected default branch index
	defaultIdx := 0
	for i, b := range branches {
		if b.Name == detectedDefault || b.IsDefault {
			defaultIdx = i
			break
		}
	}

	return BranchSelectModel{
		branches:        branches,
		cursor:          defaultIdx,
		selected:        make(map[int]bool),
		mainBranchIdx:   -1,
		detectedDefault: detectedDefault,
		step:            0,
	}
}

// keyMap defines the keybindings
type keyMap struct {
	Up     key.Binding
	Down   key.Binding
	Space  key.Binding
	Enter  key.Binding
	Quit   key.Binding
	All    key.Binding
	None   key.Binding
}

var keys = keyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "move up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "move down"),
	),
	Space: key.NewBinding(
		key.WithKeys(" "),
		key.WithHelp("space", "toggle"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "confirm"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "esc", "ctrl+c"),
		key.WithHelp("q/esc", "quit"),
	),
	All: key.NewBinding(
		key.WithKeys("a"),
		key.WithHelp("a", "select all"),
	),
	None: key.NewBinding(
		key.WithKeys("n"),
		key.WithHelp("n", "select none"),
	),
}

func (m BranchSelectModel) Init() tea.Cmd {
	return nil
}

func (m BranchSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Quit):
			m.result.Cancelled = true
			m.done = true
			return m, tea.Quit

		case key.Matches(msg, keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}

		case key.Matches(msg, keys.Down):
			if m.cursor < len(m.branches)-1 {
				m.cursor++
			}

		case key.Matches(msg, keys.Space):
			if m.step == 0 {
				// In main branch step, space selects
				m.mainBranchIdx = m.cursor
				// Move to step 1
				m.step = 1
				m.selected[m.mainBranchIdx] = true
				m.cursor = 0
			} else {
				// In feature branch step, toggle selection
				if m.cursor != m.mainBranchIdx {
					m.selected[m.cursor] = !m.selected[m.cursor]
				}
			}

		case key.Matches(msg, keys.Enter):
			if m.step == 0 {
				// Select main branch and move to step 1
				m.mainBranchIdx = m.cursor
				m.step = 1
				m.selected[m.mainBranchIdx] = true
				m.cursor = 0
			} else {
				// Confirm selection
				m.done = true
				m.result.MainBranch = m.branches[m.mainBranchIdx].Name
				m.result.SelectedBranches = []string{m.result.MainBranch}
				for i, b := range m.branches {
					if m.selected[i] && i != m.mainBranchIdx {
						m.result.SelectedBranches = append(m.result.SelectedBranches, b.Name)
					}
				}
				return m, tea.Quit
			}

		case key.Matches(msg, keys.All):
			if m.step == 1 {
				for i := range m.branches {
					m.selected[i] = true
				}
			}

		case key.Matches(msg, keys.None):
			if m.step == 1 {
				for i := range m.branches {
					if i != m.mainBranchIdx {
						m.selected[i] = false
					}
				}
			}
		}
	}

	return m, nil
}

func (m BranchSelectModel) View() string {
	if m.done {
		return ""
	}

	var b strings.Builder

	// Title
	b.WriteString(bsTitleStyle.Render("Branch Selection"))
	b.WriteString("\n")

	if m.step == 0 {
		// Step 1: Select main branch
		b.WriteString(bsStepStyle.Render("Step 1/2: Select the main branch"))
		b.WriteString("\n")
		b.WriteString(bsSubtitleStyle.Render("This is the primary branch (usually main or master)."))
		b.WriteString("\n")
		b.WriteString(bsSubtitleStyle.Render("Other branches will be compared against it."))
		b.WriteString("\n\n")

		for i, branch := range m.branches {
			cursor := "  "
			if m.cursor == i {
				cursor = "▸ "
			}

			name := branch.Name
			if branch.Name == m.detectedDefault {
				name = bsMainBranchStyle.Render(name + " (detected)")
			}

			line := fmt.Sprintf("%s%s", cursor, name)
			if m.cursor == i {
				b.WriteString(bsSelectedItemStyle.Render(line))
			} else {
				b.WriteString(bsItemStyle.Render(line))
			}
			b.WriteString("\n")
		}
	} else {
		// Step 2: Select additional branches
		b.WriteString(bsStepStyle.Render("Step 2/2: Select branches to ingest"))
		b.WriteString("\n")
		b.WriteString(bsSubtitleStyle.Render(fmt.Sprintf("Main branch: %s (always included)", bsMainBranchStyle.Render(m.branches[m.mainBranchIdx].Name))))
		b.WriteString("\n")
		b.WriteString(bsSubtitleStyle.Render("Select additional branches below."))
		b.WriteString("\n\n")

		for i, branch := range m.branches {
			cursor := "  "
			if m.cursor == i {
				cursor = "▸ "
			}

			// Checkbox
			checkbox := bsUncheckedStyle.Render("[ ]")
			if m.selected[i] {
				checkbox = bsCheckedStyle.Render("[✓]")
			}

			name := branch.Name
			if i == m.mainBranchIdx {
				name = bsMainBranchStyle.Render(name + " (main)")
			}

			line := fmt.Sprintf("%s%s %s", cursor, checkbox, name)
			if m.cursor == i {
				b.WriteString(bsSelectedItemStyle.Render(line))
			} else {
				b.WriteString(bsItemStyle.Render(line))
			}
			b.WriteString("\n")
		}
	}

	// Help text
	if m.step == 0 {
		b.WriteString(bsHelpStyle.Render("↑/↓: navigate • enter/space: select • q: cancel"))
	} else {
		b.WriteString(bsHelpStyle.Render("↑/↓: navigate • space: toggle • a: all • n: none • enter: confirm • q: cancel"))
	}

	return b.String()
}

// Result returns the selection result
func (m BranchSelectModel) Result() BranchSelection {
	return m.result
}

// Done returns whether selection is complete
func (m BranchSelectModel) Done() bool {
	return m.done
}

// RunBranchSelection runs the interactive branch selection TUI
func RunBranchSelection(branches []git.BranchInfo, detectedDefault string) (*BranchSelection, error) {
	model := NewBranchSelectModel(branches, detectedDefault)

	p := tea.NewProgram(model)
	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}

	result := finalModel.(BranchSelectModel).Result()
	if result.Cancelled {
		return nil, fmt.Errorf("branch selection cancelled")
	}

	return &result, nil
}
