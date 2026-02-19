package tui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// FolderInfo holds path and file count for folder selection
type FolderInfo struct {
	Path      string
	FileCount int
}

// FolderSelection is the result of folder selection
type FolderSelection struct {
	SelectedFolders []string
	Canceled        bool
}

type folderNode struct {
	Path      string
	Name      string
	FileCount int
	Parent    *folderNode
	Children  []*folderNode
}

type visibleFolderNode struct {
	Node  *folderNode
	Depth int
}

// FolderSelectModel is the Bubbletea model for folder selection
type FolderSelectModel struct {
	root          *folderNode
	visible       []visibleFolderNode
	cursor        int
	selected      map[string]bool
	expanded      map[string]bool
	viewportStart int
	maxVisible    int
	result        FolderSelection
	done          bool
	width         int
	height        int
}

var (
	fsTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("39")).
			Padding(0, 0, 1, 2)
	fsSubtitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Padding(0, 0, 0, 2)
	fsItemStyle = lipgloss.NewStyle().
			Padding(0, 0, 0, 2)
	fsSelectedItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("39")).
				Padding(0, 0, 0, 2)
	fsCheckedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	fsUncheckedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	fsHelpStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Padding(1, 0, 0, 2)
)

var fsKeys = struct {
	Up    key.Binding
	Down  key.Binding
	Left  key.Binding
	Right key.Binding
	Space key.Binding
	Enter key.Binding
	Quit  key.Binding
	All   key.Binding
	None  key.Binding
}{
	Up:    key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:  key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	Left:  key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "collapse")),
	Right: key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "expand")),
	Space: key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "toggle")),
	Enter: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "confirm")),
	Quit:  key.NewBinding(key.WithKeys("q", "esc", "ctrl+c"), key.WithHelp("q/esc", "quit")),
	All:   key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "select all")),
	None:  key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "select none")),
}

// NewFolderSelectModel creates a new folder selection model
func NewFolderSelectModel(folders []FolderInfo) FolderSelectModel {
	root := buildFolderTree(folders)
	model := FolderSelectModel{
		root:          root,
		cursor:        0,
		selected:      make(map[string]bool),
		expanded:      map[string]bool{".": true},
		viewportStart: 0,
		maxVisible:    12,
	}
	model.rebuildVisible()
	return model
}

func buildFolderTree(folders []FolderInfo) *folderNode {
	nodes := map[string]*folderNode{
		".": {Path: ".", Name: "(root)"},
	}

	getOrCreate := func(path string) *folderNode {
		if n, ok := nodes[path]; ok {
			return n
		}
		name := filepath.Base(path)
		if path == "." {
			name = "(root)"
		}
		n := &folderNode{Path: path, Name: name}
		nodes[path] = n
		return n
	}

	for _, f := range folders {
		path := filepath.Clean(strings.TrimSpace(f.Path))
		if path == "" {
			path = "."
		}
		n := getOrCreate(path)
		n.FileCount = f.FileCount
	}

	paths := make([]string, 0, len(nodes))
	for p := range nodes {
		paths = append(paths, p)
	}
	sort.Slice(paths, func(i, j int) bool {
		if paths[i] == "." {
			return true
		}
		if paths[j] == "." {
			return false
		}
		return paths[i] < paths[j]
	})

	for _, path := range paths {
		if path == "." {
			continue
		}
		parentPath := filepath.Dir(path)
		if parentPath == "." || parentPath == "" {
			parentPath = "."
		}
		parent := getOrCreate(parentPath)
		child := getOrCreate(path)
		child.Parent = parent
		parent.Children = append(parent.Children, child)
	}

	var sortChildren func(*folderNode)
	sortChildren = func(n *folderNode) {
		sort.Slice(n.Children, func(i, j int) bool {
			return n.Children[i].Name < n.Children[j].Name
		})
		for _, c := range n.Children {
			sortChildren(c)
		}
	}
	sortChildren(nodes["."])

	return nodes["."]
}

func (m *FolderSelectModel) rebuildVisible() {
	visible := make([]visibleFolderNode, 0)
	var walk func(*folderNode, int)
	walk = func(n *folderNode, depth int) {
		visible = append(visible, visibleFolderNode{Node: n, Depth: depth})
		if !m.expanded[n.Path] {
			return
		}
		for _, c := range n.Children {
			walk(c, depth+1)
		}
	}
	walk(m.root, 0)
	m.visible = visible
	if m.cursor >= len(m.visible) {
		m.cursor = len(m.visible) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *FolderSelectModel) currentNode() *folderNode {
	if len(m.visible) == 0 || m.cursor < 0 || m.cursor >= len(m.visible) {
		return nil
	}
	return m.visible[m.cursor].Node
}

func (m *FolderSelectModel) setSubtreeSelected(n *folderNode, selected bool) {
	m.selected[n.Path] = selected
	for _, c := range n.Children {
		m.setSubtreeSelected(c, selected)
	}
}

func (m *FolderSelectModel) hasSelectedDescendant(n *folderNode) bool {
	for _, c := range n.Children {
		if m.selected[c.Path] || m.hasSelectedDescendant(c) {
			return true
		}
	}
	return false
}

func (m *FolderSelectModel) checkboxFor(n *folderNode) string {
	if m.selected[n.Path] {
		return fsCheckedStyle.Render("[x]")
	}
	if m.hasSelectedDescendant(n) {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("[~]")
	}
	return fsUncheckedStyle.Render("[ ]")
}

func (m *FolderSelectModel) moveToNode(target *folderNode) {
	for i, row := range m.visible {
		if row.Node == target {
			m.cursor = i
			m.ensureCursorVisible()
			return
		}
	}
}

func (m *FolderSelectModel) selectedFoldersCompact() []string {
	paths := make([]string, 0)
	for path, selected := range m.selected {
		if selected {
			paths = append(paths, path)
		}
	}
	sort.Slice(paths, func(i, j int) bool {
		if paths[i] == "." {
			return true
		}
		if paths[j] == "." {
			return false
		}
		return paths[i] < paths[j]
	})

	result := make([]string, 0, len(paths))
	isCoveredByAncestor := func(path string) bool {
		if path == "." {
			return false
		}
		parent := filepath.Dir(path)
		for parent != "." && parent != "" {
			if m.selected[parent] {
				return true
			}
			parent = filepath.Dir(parent)
		}
		return m.selected["."]
	}
	for _, path := range paths {
		if isCoveredByAncestor(path) {
			continue
		}
		result = append(result, path)
	}
	return result
}

func (m FolderSelectModel) Init() tea.Cmd {
	return nil
}

func (m *FolderSelectModel) ensureCursorVisible() {
	if m.cursor < m.viewportStart {
		m.viewportStart = m.cursor
	}
	if m.cursor >= m.viewportStart+m.maxVisible {
		m.viewportStart = m.cursor - m.maxVisible + 1
	}
}

func (m FolderSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.maxVisible = min(12, msg.Height-8)
		if m.maxVisible < 3 {
			m.maxVisible = 3
		}
		return m, nil

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, fsKeys.Quit):
			m.result.Canceled = true
			m.done = true
			return m, tea.Quit

		case key.Matches(msg, fsKeys.Enter):
			m.result.SelectedFolders = m.selectedFoldersCompact()
			m.done = true
			return m, tea.Quit

		case key.Matches(msg, fsKeys.Up):
			if m.cursor > 0 {
				m.cursor--
				m.ensureCursorVisible()
			}

		case key.Matches(msg, fsKeys.Down):
			if m.cursor < len(m.visible)-1 {
				m.cursor++
				m.ensureCursorVisible()
			}

		case key.Matches(msg, fsKeys.Right):
			node := m.currentNode()
			if node != nil && len(node.Children) > 0 {
				if !m.expanded[node.Path] {
					m.expanded[node.Path] = true
					m.rebuildVisible()
				} else {
					m.moveToNode(node.Children[0])
				}
			}

		case key.Matches(msg, fsKeys.Left):
			node := m.currentNode()
			if node != nil {
				if m.expanded[node.Path] && len(node.Children) > 0 {
					m.expanded[node.Path] = false
					m.rebuildVisible()
					m.ensureCursorVisible()
				} else if node.Parent != nil {
					m.moveToNode(node.Parent)
				}
			}

		case key.Matches(msg, fsKeys.Space):
			node := m.currentNode()
			if node != nil {
				next := !m.selected[node.Path]
				m.setSubtreeSelected(node, next)
			}

		case key.Matches(msg, fsKeys.All):
			if m.root != nil {
				m.setSubtreeSelected(m.root, true)
			}

		case key.Matches(msg, fsKeys.None):
			if m.root != nil {
				m.setSubtreeSelected(m.root, false)
			}
		}
	}

	return m, nil
}

func (m FolderSelectModel) View() string {
	if m.done {
		return ""
	}

	var b strings.Builder
	b.WriteString(fsTitleStyle.Render("Select folders to index"))
	b.WriteString("\n")
	b.WriteString(fsSubtitleStyle.Render("Traverse folders and select subtrees (Space=toggle subtree, → expand, ← collapse, Enter=confirm)"))
	b.WriteString("\n\n")

	selectedCount := 0
	for _, v := range m.selected {
		if v {
			selectedCount++
		}
	}
	b.WriteString(fsSubtitleStyle.Render(fmt.Sprintf("%d selected nodes", selectedCount)))
	b.WriteString("\n\n")

	if len(m.visible) == 0 {
		b.WriteString(fsSubtitleStyle.Render("  No folders to select"))
		b.WriteString("\n")
		return b.String()
	}

	start := m.viewportStart
	end := min(start+m.maxVisible, len(m.visible))

	for i := start; i < end; i++ {
		row := m.visible[i]
		n := row.Node
		cursor := "  "
		if m.cursor == i {
			cursor = "▸ "
		}
		expandIcon := " "
		if len(n.Children) > 0 {
			if m.expanded[n.Path] {
				expandIcon = "▾"
			} else {
				expandIcon = "▸"
			}
		}
		indent := strings.Repeat("  ", row.Depth)
		line := fmt.Sprintf("%s%s %s%s %s  %d files", cursor, m.checkboxFor(n), indent, expandIcon, n.Name, n.FileCount)
		if m.cursor == i {
			b.WriteString(fsSelectedItemStyle.Render(line))
		} else {
			b.WriteString(fsItemStyle.Render(line))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(fsHelpStyle.Render(
		fsKeys.Space.Help().Key + " toggle subtree  " +
			fsKeys.Right.Help().Key + " expand  " +
			fsKeys.Left.Help().Key + " collapse  " +
			fsKeys.Enter.Help().Key + " confirm  " +
			fsKeys.Quit.Help().Key + " cancel"))

	return b.String()
}

func (m FolderSelectModel) Result() FolderSelection {
	return m.result
}

func (m FolderSelectModel) Done() bool {
	return m.done
}

// RunFolderSelection runs the interactive folder selection TUI
func RunFolderSelection(folders []FolderInfo) (*FolderSelection, error) {
	model := NewFolderSelectModel(folders)
	p := tea.NewProgram(model)
	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}
	result := finalModel.(FolderSelectModel).Result()
	if result.Canceled {
		return nil, fmt.Errorf("folder selection canceled")
	}
	return &result, nil
}
