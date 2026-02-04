package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
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

	bsSearchStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Padding(0, 0, 0, 2)

	bsScrollStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Padding(0, 0, 0, 2)
)

// BranchSelection is the result of branch selection
type BranchSelection struct {
	MainBranch       string
	SelectedBranches []string
	Canceled         bool
}

// BranchSelectModel is the Bubbletea model for branch selection
type BranchSelectModel struct {
	branches        []git.BranchInfo
	filtered        []int // indices into branches that match filter
	cursor          int   // cursor in filtered list
	selected        map[int]bool
	mainBranchIdx   int
	detectedDefault string

	// Search
	searchInput textinput.Model
	searching   bool

	// Scrolling
	viewportStart int
	maxVisible    int

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

	// Initialize all branch indices as filtered
	filtered := make([]int, len(branches))
	for i := range branches {
		filtered[i] = i
	}

	// Find cursor position in filtered list
	cursorPos := 0
	for i, idx := range filtered {
		if idx == defaultIdx {
			cursorPos = i
			break
		}
	}

	// Search input
	ti := textinput.New()
	ti.Placeholder = "Type to search branches..."
	ti.CharLimit = 50
	ti.Width = 30

	return BranchSelectModel{
		branches:        branches,
		filtered:        filtered,
		cursor:          cursorPos,
		selected:        make(map[int]bool),
		mainBranchIdx:   -1,
		detectedDefault: detectedDefault,
		step:            0,
		searchInput:     ti,
		searching:       false,
		maxVisible:      10, // Show max 10 branches at a time
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
	Search key.Binding
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
	Search: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "search"),
	),
}

func (m BranchSelectModel) Init() tea.Cmd {
	return nil
}

func (m *BranchSelectModel) filterBranches() {
	query := strings.ToLower(m.searchInput.Value())
	if query == "" {
		// No filter, show all
		m.filtered = make([]int, len(m.branches))
		for i := range m.branches {
			m.filtered[i] = i
		}
	} else {
		m.filtered = nil
		for i, b := range m.branches {
			if strings.Contains(strings.ToLower(b.Name), query) {
				m.filtered = append(m.filtered, i)
			}
		}
	}

	// Reset cursor if out of bounds
	if m.cursor >= len(m.filtered) {
		m.cursor = 0
	}
	m.viewportStart = 0
}

func (m *BranchSelectModel) ensureCursorVisible() {
	if m.cursor < m.viewportStart {
		m.viewportStart = m.cursor
	}
	if m.cursor >= m.viewportStart+m.maxVisible {
		m.viewportStart = m.cursor - m.maxVisible + 1
	}
}

func (m BranchSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Adjust max visible based on terminal height
		m.maxVisible = min(10, msg.Height-10)
		if m.maxVisible < 3 {
			m.maxVisible = 3
		}
		return m, nil

	case tea.KeyMsg:
		// Handle search mode
		if m.searching {
			switch msg.String() {
			case "enter", "esc":
				m.searching = false
				m.searchInput.Blur()
				return m, nil
			default:
				m.searchInput, cmd = m.searchInput.Update(msg)
				m.filterBranches()
				return m, cmd
			}
		}

		switch {
		case key.Matches(msg, keys.Quit):
			m.result.Canceled = true
			m.done = true
			return m, tea.Quit

		case key.Matches(msg, keys.Search):
			m.searching = true
			m.searchInput.Focus()
			return m, textinput.Blink

		case key.Matches(msg, keys.Up):
			if m.cursor > 0 {
				m.cursor--
				m.ensureCursorVisible()
			}

		case key.Matches(msg, keys.Down):
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
				m.ensureCursorVisible()
			}

		case key.Matches(msg, keys.Space):
			if len(m.filtered) == 0 {
				return m, nil
			}
			realIdx := m.filtered[m.cursor]

			if m.step == 0 {
				// In main branch step, space selects
				m.mainBranchIdx = realIdx
				// Move to step 1
				m.step = 1
				m.selected[m.mainBranchIdx] = true
				m.cursor = 0
				m.viewportStart = 0
				m.searchInput.SetValue("")
				m.filterBranches()
			} else {
				// In feature branch step, toggle selection
				if realIdx != m.mainBranchIdx {
					m.selected[realIdx] = !m.selected[realIdx]
				}
			}

		case key.Matches(msg, keys.Enter):
			if len(m.filtered) == 0 {
				return m, nil
			}

			if m.step == 0 {
				// Select main branch and move to step 1
				m.mainBranchIdx = m.filtered[m.cursor]
				m.step = 1
				m.selected[m.mainBranchIdx] = true
				m.cursor = 0
				m.viewportStart = 0
				m.searchInput.SetValue("")
				m.filterBranches()
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
		b.WriteString("\n\n")
	} else {
		// Step 2: Select additional branches
		b.WriteString(bsStepStyle.Render("Step 2/2: Select branches to ingest"))
		b.WriteString("\n")
		b.WriteString(bsSubtitleStyle.Render(fmt.Sprintf("Main branch: %s (always included)", bsMainBranchStyle.Render(m.branches[m.mainBranchIdx].Name))))
		b.WriteString("\n\n")
	}

	// Search box
	if m.searching {
		b.WriteString(bsSearchStyle.Render("Search: "))
		b.WriteString(m.searchInput.View())
		b.WriteString("\n\n")
	} else if m.searchInput.Value() != "" {
		b.WriteString(bsSearchStyle.Render(fmt.Sprintf("Filter: %s (/ to edit, esc to clear)", m.searchInput.Value())))
		b.WriteString("\n\n")
	}

	// Show scroll indicator if needed
	if len(m.filtered) > m.maxVisible {
		b.WriteString(bsScrollStyle.Render(fmt.Sprintf("Showing %d-%d of %d branches", m.viewportStart+1, min(m.viewportStart+m.maxVisible, len(m.filtered)), len(m.filtered))))
		b.WriteString("\n\n")
	}

	// Branch list
	if len(m.filtered) == 0 {
		b.WriteString(bsSubtitleStyle.Render("  No branches match your search"))
		b.WriteString("\n")
	} else {
		// Calculate visible range
		start := m.viewportStart
		end := min(start+m.maxVisible, len(m.filtered))

		// Show scroll up indicator
		if start > 0 {
			b.WriteString(bsScrollStyle.Render("  ↑ more branches above"))
			b.WriteString("\n")
		}

		for i := start; i < end; i++ {
			branchIdx := m.filtered[i]
			branch := m.branches[branchIdx]

			cursor := "  "
			if m.cursor == i {
				cursor = "▸ "
			}

			if m.step == 0 {
				// Main branch selection
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
			} else {
				// Feature branch selection with checkboxes
				checkbox := bsUncheckedStyle.Render("[ ]")
				if m.selected[branchIdx] {
					checkbox = bsCheckedStyle.Render("[✓]")
				}

				name := branch.Name
				if branchIdx == m.mainBranchIdx {
					name = bsMainBranchStyle.Render(name + " (main)")
				}

				line := fmt.Sprintf("%s%s %s", cursor, checkbox, name)
				if m.cursor == i {
					b.WriteString(bsSelectedItemStyle.Render(line))
				} else {
					b.WriteString(bsItemStyle.Render(line))
				}
			}
			b.WriteString("\n")
		}

		// Show scroll down indicator
		if end < len(m.filtered) {
			b.WriteString(bsScrollStyle.Render("  ↓ more branches below"))
			b.WriteString("\n")
		}
	}

	// Help text
	b.WriteString("\n")
	if m.searching {
		b.WriteString(bsHelpStyle.Render("Type to filter • enter: done searching • esc: clear"))
	} else if m.step == 0 {
		b.WriteString(bsHelpStyle.Render("↑/↓: navigate • /: search • enter/space: select • q: cancel"))
	} else {
		b.WriteString(bsHelpStyle.Render("↑/↓: navigate • /: search • space: toggle • a: all • n: none • enter: confirm"))
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

	// Don't use alternate screen - stay in same terminal
	p := tea.NewProgram(model)
	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}

	result := finalModel.(BranchSelectModel).Result()
	if result.Canceled {
		return nil, fmt.Errorf("branch selection canceled")
	}

	return &result, nil
}

// RunBranchSelectionWithPreselected runs the TUI with pre-selected branches for modification
func RunBranchSelectionWithPreselected(branches []git.BranchInfo, mainBranch string, selectedBranches []string) (*BranchSelection, error) {
	model := NewBranchSelectModelWithPreselected(branches, mainBranch, selectedBranches)

	// Don't use alternate screen - stay in same terminal
	p := tea.NewProgram(model)
	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}

	result := finalModel.(BranchSelectModel).Result()
	if result.Canceled {
		return nil, fmt.Errorf("branch selection canceled")
	}

	return &result, nil
}

// NewBranchSelectModelWithPreselected creates a model with pre-selected branches (for modify mode)
func NewBranchSelectModelWithPreselected(branches []git.BranchInfo, mainBranch string, selectedBranches []string) BranchSelectModel {
	// Find the main branch index
	mainIdx := 0
	for i, b := range branches {
		if b.Name == mainBranch {
			mainIdx = i
			break
		}
	}

	// Initialize all branch indices as filtered
	filtered := make([]int, len(branches))
	for i := range branches {
		filtered[i] = i
	}

	// Build selected map from pre-selected branches
	selectedMap := make(map[int]bool)
	selectedSet := make(map[string]bool)
	for _, b := range selectedBranches {
		selectedSet[b] = true
	}
	for i, b := range branches {
		if selectedSet[b.Name] {
			selectedMap[i] = true
		}
	}

	// Search input
	ti := textinput.New()
	ti.Placeholder = "Type to search branches..."
	ti.CharLimit = 50
	ti.Width = 30

	return BranchSelectModel{
		branches:        branches,
		filtered:        filtered,
		cursor:          0,
		selected:        selectedMap,
		mainBranchIdx:   mainIdx,
		detectedDefault: mainBranch,
		step:            1, // Start at step 1 (modify mode) - skip main branch selection
		searchInput:     ti,
		searching:       false,
		maxVisible:      10,
	}
}
