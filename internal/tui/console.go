package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/ishaan812/devlog/internal/db"
)

// ── Data types passed from CLI ─────────────────────────────────────────────

// ConsoleCodebase holds codebase info for the console TUI.
type ConsoleCodebase struct {
	ID        string
	Name      string
	Path      string
	DateCount int
	Dates     []ConsoleDate
}

// ConsoleDate holds date info for the console TUI.
type ConsoleDate struct {
	EntryDate   time.Time
	EntryCount  int
	CommitCount int
	Additions   int
	Deletions   int
}

// ── Styles ─────────────────────────────────────────────────────────────────

var (
	// Panel borders
	activeBorderStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("86"))

	inactiveBorderStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("241"))

	// Title bar
	consoleTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("86")).
				Background(lipgloss.Color("236")).
				Padding(0, 1)

	consoleProfileStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("241")).
				Background(lipgloss.Color("236")).
				Padding(0, 1)

	// List items
	consoleSectionTitle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("86")).
				Padding(0, 1)

	consoleCursorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("86")).
				Bold(true)

	consoleItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252"))

	consoleDimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	consoleStatStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245"))

	// Help bar
	consoleHelpStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("241")).
				Background(lipgloss.Color("236")).
				Padding(0, 1)

	// Content panel
	consoleHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("86")).
				Padding(0, 1)

	consoleEmptyStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("241")).
				Italic(true).
				Padding(1, 2)

	consoleScrollStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("241")).
				Align(lipgloss.Right)

	// Logo styles
	logoMainStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("86")).
			Bold(true)

	logoSubStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	logoDimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))
)

// ── Pane constants ─────────────────────────────────────────────────────────

const (
	paneRepos = 0
	paneDates = 1
)

// ── Model ──────────────────────────────────────────────────────────────────

// ConsoleModel is the Bubbletea model for the full-screen console TUI.
type ConsoleModel struct {
	// Layout
	width  int
	height int

	// Active pane: 0=repos, 1=dates (cursor always on left side)
	activePane int

	// Data
	codebases   []ConsoleCodebase
	profileName string
	dbRepo      *db.SQLRepository

	// Repos pane
	repoCursor int
	repoScroll int

	// Dates pane
	dateCursor int
	dateScroll int

	// Content pane (read-only, no focus)
	viewport      viewport.Model
	contentReady  bool
	selectedRepo  int
	selectedDate  int
	contentHeader string

	// State
	quitting bool
}

// NewConsoleModel creates a new console model.
func NewConsoleModel(codebases []ConsoleCodebase, profileName string, dbRepo *db.SQLRepository) ConsoleModel {
	vp := viewport.New(0, 0)
	vp.SetContent("")

	selectedRepo := -1
	if len(codebases) > 0 {
		selectedRepo = 0
	}

	return ConsoleModel{
		codebases:    codebases,
		profileName:  profileName,
		dbRepo:       dbRepo,
		activePane:   paneRepos,
		repoCursor:   0,
		selectedRepo: selectedRepo,
		selectedDate: -1,
		viewport:     vp,
	}
}

func (m ConsoleModel) Init() tea.Cmd {
	return nil
}

// ── Update ─────────────────────────────────────────────────────────────────

func (m ConsoleModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateViewportSize()
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit

		// Tab toggles between repos and dates only (left side)
		case "tab":
			if m.activePane == paneRepos {
				m.activePane = paneDates
			} else {
				// Going back to repos pane clears the content
				m.activePane = paneRepos
				m.selectedDate = -1
				m.contentReady = false
				m.viewport.SetContent("")
				m.contentHeader = ""
			}
			return m, nil

		case "shift+tab":
			if m.activePane == paneDates {
				// Going back to repos pane clears the content
				m.activePane = paneRepos
				m.selectedDate = -1
				m.contentReady = false
				m.viewport.SetContent("")
				m.contentHeader = ""
			} else {
				m.activePane = paneDates
			}
			return m, nil

		case "up", "k":
			switch m.activePane {
			case paneRepos:
				if m.repoCursor > 0 {
					m.repoCursor--
					m.ensureRepoVisible()
				}
			case paneDates:
				if m.dateCursor > 0 {
					m.dateCursor--
					m.ensureDateVisible()
				}
			}
			return m, nil

		case "down", "j":
			switch m.activePane {
			case paneRepos:
				if m.repoCursor < len(m.codebases)-1 {
					m.repoCursor++
					m.ensureRepoVisible()
				}
			case paneDates:
				dates := m.currentDates()
				if m.dateCursor < len(dates)-1 {
					m.dateCursor++
					m.ensureDateVisible()
				}
			}
			return m, nil

		case "enter":
			switch m.activePane {
			case paneRepos:
				if m.repoCursor >= 0 && m.repoCursor < len(m.codebases) {
					m.selectedRepo = m.repoCursor
					m.dateCursor = 0
					m.dateScroll = 0
					m.selectedDate = -1
					m.contentReady = false
					m.viewport.SetContent("")
					m.contentHeader = ""
					// Move to dates pane but stay on left
					m.activePane = paneDates
				}
			case paneDates:
				// Select date and load content on right, cursor stays here
				dates := m.currentDates()
				if m.dateCursor >= 0 && m.dateCursor < len(dates) {
					m.selectedDate = m.dateCursor
					m.loadContent()
				}
			}
			return m, nil

		// Right-side scrolling always works when content is loaded
		case "pgup":
			if m.contentReady {
				var cmd tea.Cmd
				m.viewport, cmd = m.viewport.Update(msg)
				return m, cmd
			}
			return m, nil

		case "pgdown":
			if m.contentReady {
				var cmd tea.Cmd
				m.viewport, cmd = m.viewport.Update(msg)
				return m, cmd
			}
			return m, nil

		// Ctrl+U / Ctrl+D for half-page scroll of content
		case "ctrl+u":
			if m.contentReady {
				m.viewport.HalfViewUp()
			}
			return m, nil

		case "ctrl+d":
			if m.contentReady {
				m.viewport.HalfViewDown()
			}
			return m, nil
		}
	}

	return m, nil
}

// ── Helpers ────────────────────────────────────────────────────────────────

func (m *ConsoleModel) currentDates() []ConsoleDate {
	if m.selectedRepo >= 0 && m.selectedRepo < len(m.codebases) {
		return m.codebases[m.selectedRepo].Dates
	}
	return nil
}

func (m *ConsoleModel) leftPanelWidth() int {
	w := m.width * 30 / 100
	if w < 28 {
		w = 28
	}
	if w > 45 {
		w = 45
	}
	return w
}

func (m *ConsoleModel) updateViewportSize() {
	leftW := m.leftPanelWidth()
	rightW := m.width - leftW - 5
	if rightW < 20 {
		rightW = 20
	}
	vpHeight := m.height - 6
	if vpHeight < 5 {
		vpHeight = 5
	}
	m.viewport.Width = rightW
	m.viewport.Height = vpHeight

	// Re-render content if we have it to adjust word wrap
	if m.contentReady && m.selectedDate >= 0 {
		m.loadContent()
	}
}

func (m *ConsoleModel) ensureRepoVisible() {
	maxVis := m.maxVisibleRepos()
	if m.repoCursor < m.repoScroll {
		m.repoScroll = m.repoCursor
	}
	if m.repoCursor >= m.repoScroll+maxVis {
		m.repoScroll = m.repoCursor - maxVis + 1
	}
}

func (m *ConsoleModel) ensureDateVisible() {
	maxVis := m.maxVisibleDates()
	if m.dateCursor < m.dateScroll {
		m.dateScroll = m.dateCursor
	}
	if m.dateCursor >= m.dateScroll+maxVis {
		m.dateScroll = m.dateCursor - maxVis + 1
	}
}

func (m *ConsoleModel) maxVisibleRepos() int {
	h := (m.height - 8) / 2
	if h < 3 {
		h = 3
	}
	return h
}

func (m *ConsoleModel) maxVisibleDates() int {
	h := (m.height - 8) / 2
	if h < 3 {
		h = 3
	}
	return h
}

func (m *ConsoleModel) loadContent() {
	if m.selectedRepo < 0 || m.selectedRepo >= len(m.codebases) {
		return
	}
	dates := m.currentDates()
	if m.selectedDate < 0 || m.selectedDate >= len(dates) {
		return
	}

	cb := m.codebases[m.selectedRepo]
	date := dates[m.selectedDate]

	ctx := context.Background()
	entries, err := m.dbRepo.ListWorklogEntriesByDate(ctx, cb.ID, m.profileName, date.EntryDate)
	if err != nil || len(entries) == 0 {
		m.contentReady = true
		m.contentHeader = date.EntryDate.Format("Monday, January 2, 2006")
		m.viewport.SetContent(consoleEmptyStyle.Render("No worklog entries for this date.\nRun 'devlog worklog' to generate worklogs."))
		return
	}

	md := renderDayMarkdown(entries, date.EntryDate)

	width := m.viewport.Width - 2
	if width < 20 {
		width = 20
	}
	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)

	var rendered string
	if err == nil {
		rendered, err = renderer.Render(md)
	}
	if err != nil {
		rendered = md
	}

	m.contentReady = true
	m.contentHeader = fmt.Sprintf("%s  -  %s", date.EntryDate.Format("Monday, January 2, 2006"), cb.Name)
	m.viewport.SetContent(rendered)
	m.viewport.GotoTop()
}

func renderDayMarkdown(entries []db.WorklogEntry, date time.Time) string {
	var md strings.Builder
	md.WriteString(fmt.Sprintf("# %s\n\n", date.Format("Monday, January 2, 2006")))
	for _, e := range entries {
		name := e.BranchName
		if name == "" {
			name = "unknown"
		}
		md.WriteString(fmt.Sprintf("## Branch: %s\n\n", name))
		md.WriteString(e.Content)
		md.WriteString("\n\n")
	}
	return md.String()
}

// ── View ───────────────────────────────────────────────────────────────────

func (m ConsoleModel) View() string {
	if m.quitting {
		return ""
	}
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	var b strings.Builder

	titleBar := m.renderTitleBar()
	b.WriteString(titleBar)
	b.WriteString("\n")

	leftPanel := m.renderLeftPanel()
	rightPanel := m.renderRightPanel()

	mainContent := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)
	b.WriteString(mainContent)
	b.WriteString("\n")

	helpBar := m.renderHelpBar()
	b.WriteString(helpBar)

	return b.String()
}

func (m ConsoleModel) renderTitleBar() string {
	title := consoleTitleStyle.Render("  DevLog Console")
	profile := consoleProfileStyle.Render(fmt.Sprintf("profile: %s  ", m.profileName))

	titleLen := lipgloss.Width(title)
	profileLen := lipgloss.Width(profile)
	spacerLen := m.width - titleLen - profileLen
	if spacerLen < 0 {
		spacerLen = 0
	}
	spacer := lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Render(strings.Repeat(" ", spacerLen))

	return title + spacer + profile
}

func (m ConsoleModel) renderLeftPanel() string {
	leftW := m.leftPanelWidth()
	innerW := leftW - 2

	reposSection := m.renderReposSection(innerW)
	datesSection := m.renderDatesSection(innerW)

	content := reposSection + "\n" + datesSection

	panelHeight := m.height - 3

	return activeBorderStyle.
		Width(leftW).
		Height(panelHeight).
		Render(content)
}

func (m ConsoleModel) renderReposSection(width int) string {
	var b strings.Builder

	sectionLabel := "Repositories"
	if m.activePane == paneRepos {
		b.WriteString(consoleSectionTitle.Render(sectionLabel))
	} else {
		b.WriteString(consoleDimStyle.Bold(true).Render(" " + sectionLabel))
	}
	b.WriteString("\n")
	b.WriteString(consoleDimStyle.Render(" " + strings.Repeat("─", width-2)))
	b.WriteString("\n")

	if len(m.codebases) == 0 {
		b.WriteString(consoleDimStyle.Render(" No repos indexed"))
		b.WriteString("\n")
		return b.String()
	}

	maxVis := m.maxVisibleRepos()
	start := m.repoScroll
	end := start + maxVis
	if end > len(m.codebases) {
		end = len(m.codebases)
	}

	if start > 0 {
		b.WriteString(consoleDimStyle.Render(" ↑ more"))
		b.WriteString("\n")
	}

	for i := start; i < end; i++ {
		cb := m.codebases[i]
		isSelected := i == m.selectedRepo
		isCursor := i == m.repoCursor && m.activePane == paneRepos

		cursor := "  "
		if isCursor {
			cursor = "> "
		}

		name := cb.Name
		if len(name) > width-10 {
			name = name[:width-13] + "..."
		}

		badge := ""
		if cb.DateCount > 0 {
			badge = consoleStatStyle.Render(fmt.Sprintf(" %dd", cb.DateCount))
		}

		var line string
		if isCursor {
			line = consoleCursorStyle.Render(cursor + name)
		} else if isSelected {
			line = consoleItemStyle.Bold(true).Render(cursor + name)
		} else {
			line = consoleItemStyle.Render(cursor + name)
		}

		b.WriteString(line + badge)
		b.WriteString("\n")
	}

	if end < len(m.codebases) {
		b.WriteString(consoleDimStyle.Render(" ↓ more"))
		b.WriteString("\n")
	}

	return b.String()
}

func (m ConsoleModel) renderDatesSection(width int) string {
	var b strings.Builder

	sectionLabel := "Dates"
	if m.activePane == paneDates {
		b.WriteString(consoleSectionTitle.Render(sectionLabel))
	} else {
		b.WriteString(consoleDimStyle.Bold(true).Render(" " + sectionLabel))
	}
	b.WriteString("\n")
	b.WriteString(consoleDimStyle.Render(" " + strings.Repeat("─", width-2)))
	b.WriteString("\n")

	dates := m.currentDates()
	if len(dates) == 0 {
		if m.selectedRepo >= 0 {
			b.WriteString(consoleDimStyle.Render(" No cached worklogs"))
			b.WriteString("\n")
			b.WriteString(consoleDimStyle.Render(" Run 'devlog worklog'"))
			b.WriteString("\n")
		} else {
			b.WriteString(consoleDimStyle.Render(" Select a repo first"))
			b.WriteString("\n")
		}
		return b.String()
	}

	maxVis := m.maxVisibleDates()
	start := m.dateScroll
	end := start + maxVis
	if end > len(dates) {
		end = len(dates)
	}

	if start > 0 {
		b.WriteString(consoleDimStyle.Render(" ↑ more"))
		b.WriteString("\n")
	}

	for i := start; i < end; i++ {
		d := dates[i]
		isSelected := i == m.selectedDate
		isCursor := i == m.dateCursor && m.activePane == paneDates

		cursor := "  "
		if isCursor {
			cursor = "> "
		}

		dateStr := d.EntryDate.Format("Mon, Jan 2")
		stats := consoleStatStyle.Render(fmt.Sprintf(" +%d/-%d", d.Additions, d.Deletions))

		var line string
		if isCursor {
			line = consoleCursorStyle.Render(cursor + dateStr)
		} else if isSelected {
			line = consoleItemStyle.Bold(true).Render(cursor + dateStr)
		} else {
			line = consoleItemStyle.Render(cursor + dateStr)
		}

		b.WriteString(line + stats)
		b.WriteString("\n")
	}

	if end < len(dates) {
		b.WriteString(consoleDimStyle.Render(" ↓ more"))
		b.WriteString("\n")
	}

	return b.String()
}

func (m ConsoleModel) renderRightPanel() string {
	leftW := m.leftPanelWidth()
	rightW := m.width - leftW - 3
	if rightW < 20 {
		rightW = 20
	}
	panelHeight := m.height - 3

	var content string
	if !m.contentReady {
		content = m.renderLogo(rightW, panelHeight)
	} else {
		header := consoleHeaderStyle.Render(m.contentHeader)
		divider := consoleDimStyle.Render(" " + strings.Repeat("─", rightW-4))

		scrollPct := ""
		if m.viewport.TotalLineCount() > m.viewport.Height {
			pct := int(m.viewport.ScrollPercent() * 100)
			scrollPct = consoleScrollStyle.Width(rightW - 4).Render(fmt.Sprintf("%d%%", pct))
		}

		content = header + "\n" + divider + "\n" + m.viewport.View()
		if scrollPct != "" {
			content += "\n" + scrollPct
		}
	}

	return inactiveBorderStyle.
		Width(rightW).
		Height(panelHeight).
		Render(content)
}

func (m ConsoleModel) renderLogo(width, height int) string {
	logo := []string{
		"    ____            __              ",
		"   / __ \\___ _   __/ /   ____  ____ ",
		"  / / / / _ \\ | / / /   / __ \\/ __ \\",
		" / /_/ /  __/ |/ / /___/ /_/ / /_/ /",
		"/_____/\\___/|___/_____/\\____/\\__, / ",
		"                            /____/  ",
	}

	var b strings.Builder

	// Center the logo vertically
	logoHeight := len(logo) + 6 // logo lines + spacing + subtitle + hints
	topPad := (height - logoHeight) / 2
	if topPad < 2 {
		topPad = 2
	}

	for i := 0; i < topPad; i++ {
		b.WriteString("\n")
	}

	// Render logo lines centered
	for _, line := range logo {
		pad := (width - len(line)) / 2
		if pad < 0 {
			pad = 0
		}
		b.WriteString(strings.Repeat(" ", pad))
		b.WriteString(logoMainStyle.Render(line))
		b.WriteString("\n")
	}

	b.WriteString("\n")

	// Subtitle
	subtitle := "Track & analyze your development activity"
	subPad := (width - len(subtitle)) / 2
	if subPad < 0 {
		subPad = 0
	}
	b.WriteString(strings.Repeat(" ", subPad))
	b.WriteString(logoSubStyle.Render(subtitle))
	b.WriteString("\n\n")

	// Author
	author := "by ishaan812"
	authorPad := (width - len(author)) / 2
	if authorPad < 0 {
		authorPad = 0
	}
	b.WriteString(strings.Repeat(" ", authorPad))
	b.WriteString(logoDimStyle.Render(author))
	b.WriteString("\n\n")

	// Hint
	hint := "Select a repo and date to view worklogs"
	hintPad := (width - len(hint)) / 2
	if hintPad < 0 {
		hintPad = 0
	}
	b.WriteString(strings.Repeat(" ", hintPad))
	b.WriteString(consoleDimStyle.Italic(true).Render(hint))

	return b.String()
}

func (m ConsoleModel) renderHelpBar() string {
	var help string
	switch m.activePane {
	case paneRepos:
		help = "  ↑/↓ navigate  enter select repo  tab dates  pgup/pgdn scroll content  q quit"
	case paneDates:
		help = "  ↑/↓ navigate  enter view worklog  tab repos  pgup/pgdn scroll content  q quit"
	}

	helpText := consoleHelpStyle.Render(help)
	spacerLen := m.width - lipgloss.Width(helpText)
	if spacerLen < 0 {
		spacerLen = 0
	}
	spacer := lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Render(strings.Repeat(" ", spacerLen))

	return helpText + spacer
}

// ── Runner ─────────────────────────────────────────────────────────────────

// RunConsole launches the full-screen console TUI.
func RunConsole(codebases []ConsoleCodebase, profileName string, dbRepo *db.SQLRepository) error {
	model := NewConsoleModel(codebases, profileName, dbRepo)
	p := tea.NewProgram(model, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
