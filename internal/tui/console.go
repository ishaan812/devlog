package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
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
	ID          string
	Name        string
	Path        string
	DateCount   int
	Dates       []ConsoleDate
	Weeks       []ConsoleWeek
	Months      []ConsoleMonth
	CommitCount int  // Total commits ingested
	IsIngested  bool // Whether ingest has been run
}

// ConsoleDate holds date info for the console TUI.
type ConsoleDate struct {
	EntryDate   time.Time
	EntryCount  int
	CommitCount int
	Additions   int
	Deletions   int
}

// ConsoleWeek holds week info for the console TUI.
type ConsoleWeek struct {
	WeekStart   time.Time
	WeekEnd     time.Time
	DateCount   int
	EntryCount  int
	CommitCount int
	Additions   int
	Deletions   int
	Dates       []ConsoleDate
}

// ConsoleMonth holds month info for the console TUI.
type ConsoleMonth struct {
	MonthStart  time.Time
	MonthEnd    time.Time
	DateCount   int
	WeekCount   int
	EntryCount  int
	CommitCount int
	Additions   int
	Deletions   int
	Weeks       []ConsoleWeek
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

	// Help bar key badge styles
	helpKeyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("239")).
			Bold(true).
			Padding(0, 1)

	helpDescStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("248")).
			Background(lipgloss.Color("236")).
			Padding(0, 1, 0, 0)

	helpSepStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("238")).
			Background(lipgloss.Color("236"))

	helpBarBg = lipgloss.NewStyle().
			Background(lipgloss.Color("236"))
)

// ── Pane constants ─────────────────────────────────────────────────────────

const (
	paneRepos   = 0
	paneDates   = 1
	paneContent = 2
)

// ── Model ──────────────────────────────────────────────────────────────────

// DateItem represents an item in the hierarchical date view
type DateItem struct {
	Type        string    // "month", "week", or "day"
	Date        time.Time // The date (month start, week start, or day)
	DisplayText string
	Stats       string
	Indent      int
	IsExpanded  bool
	Children    []DateItem
}

// ── Operation messages ─────────────────────────────────────────────────────

type operationCompleteMsg struct {
	opType string
	repoID string
	err    error
	output string
}

// ConsoleModel is the Bubbletea model for the full-screen console TUI.
type ConsoleModel struct {
	// Layout
	width  int
	height int

	// Active pane: 0=repos, 1=dates, 2=content
	activePane int

	// Data
	codebases   []ConsoleCodebase
	profileName string
	dbRepo      *db.SQLRepository

	// Repos pane
	repoCursor int
	repoScroll int

	// Dates pane
	dateCursor     int
	dateScroll     int
	dateItems      []DateItem // Flattened hierarchical view of dates
	expandedMonths map[string]bool
	expandedWeeks  map[string]bool

	// Content pane (read-only, no focus)
	viewport      viewport.Model
	contentReady  bool
	selectedRepo  int
	selectedDate  int
	contentHeader string

	// Operation state
	operationRunning bool
	operationType    string // "ingest", "worklog", or "ingest+worklog"
	operationRepo    string // repo ID
	operationError   string
	operationOutput  string // Full output from operation

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
		codebases:      codebases,
		profileName:    profileName,
		dbRepo:         dbRepo,
		activePane:     paneRepos,
		repoCursor:     0,
		selectedRepo:   selectedRepo,
		selectedDate:   -1,
		viewport:       vp,
		expandedMonths: make(map[string]bool),
		expandedWeeks:  make(map[string]bool),
	}
}

func (m ConsoleModel) Init() tea.Cmd {
	return nil
}

// ── Update ─────────────────────────────────────────────────────────────────

func (m ConsoleModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		if m.operationRunning {
			return m, tea.Tick(time.Millisecond*100, func(t time.Time) tea.Msg {
				return tickMsg(t)
			})
		}
		return m, nil

	case operationCompleteMsg:
		m.operationRunning = false
		m.operationOutput = msg.output
		if msg.err != nil {
			m.operationError = msg.err.Error()
		} else {
			m.operationError = ""
		}
		// Always reconnect and reload data (db was closed before operation)
		return m, reloadConsoleDataCmd(m.profileName)

	case reloadDataMsg:
		m.codebases = msg.codebases
		if msg.dbRepo != nil {
			m.dbRepo = msg.dbRepo
		}
		// Adjust cursor if needed
		if m.repoCursor >= len(m.codebases) {
			m.repoCursor = len(m.codebases) - 1
		}
		if m.repoCursor < 0 {
			m.repoCursor = 0
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateViewportSize()
		return m, nil

	case tea.KeyMsg:
		// Clear error and output on any keypress
		if m.operationError != "" || m.operationOutput != "" {
			m.operationError = ""
			m.operationOutput = ""
			return m, nil
		}

		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			// Close database connection on exit
			closeReadOnlyConnection(m.dbRepo)
			return m, tea.Quit

		// ── Operations on selected repo ────────────────────────────
		case "G": // Shift+G - Run ingest + worklog on selected repo
			if m.activePane == paneRepos && m.repoCursor >= 0 && m.repoCursor < len(m.codebases) && !m.operationRunning {
				repo := m.codebases[m.repoCursor]
				m.operationRunning = true
				m.operationType = "ingest+worklog"
				m.operationRepo = repo.ID
				m.operationError = ""
				m.operationOutput = ""
				// Capture current dbRepo and nil it out (persists via returned m)
				oldDB := m.dbRepo
				m.dbRepo = nil
				return m, tea.Batch(
					tea.Tick(time.Millisecond*100, func(t time.Time) tea.Msg { return tickMsg(t) }),
					runIngestAndWorklogCmd(repo, oldDB, m.profileName),
				)
			}
			return m, nil

		case "I": // Shift+I - Run ingest on selected repo
			if m.activePane == paneRepos && m.repoCursor >= 0 && m.repoCursor < len(m.codebases) && !m.operationRunning {
				repo := m.codebases[m.repoCursor]
				m.operationRunning = true
				m.operationType = "ingest"
				m.operationRepo = repo.ID
				m.operationError = ""
				m.operationOutput = ""
				// Capture current dbRepo and nil it out (persists via returned m)
				oldDB := m.dbRepo
				m.dbRepo = nil
				return m, tea.Batch(
					tea.Tick(time.Millisecond*100, func(t time.Time) tea.Msg { return tickMsg(t) }),
					runIngestCmd(repo, oldDB, m.profileName),
				)
			}
			return m, nil

		case "W": // Shift+W - Generate worklog on selected repo
			if m.activePane == paneRepos && m.repoCursor >= 0 && m.repoCursor < len(m.codebases) && !m.operationRunning {
				repo := m.codebases[m.repoCursor]
				m.operationRunning = true
				m.operationType = "worklog"
				m.operationRepo = repo.ID
				m.operationError = ""
				m.operationOutput = ""
				// Capture current dbRepo and nil it out (persists via returned m)
				oldDB := m.dbRepo
				m.dbRepo = nil
				return m, tea.Batch(
					tea.Tick(time.Millisecond*100, func(t time.Time) tea.Msg { return tickMsg(t) }),
					runWorklogCmd(repo, oldDB, m.profileName),
				)
			}
			return m, nil

		// ── Panel switching ────────────────────────────────────────

		// Tab cycles left-side panes: repos <-> dates
		case "tab":
			if m.activePane == paneContent {
				// from content, go back to dates
				m.activePane = paneDates
			} else if m.activePane == paneRepos {
				m.activePane = paneDates
			} else {
				m.activePane = paneRepos
				m.selectedDate = -1
				m.contentReady = false
				m.viewport.SetContent("")
				m.contentHeader = ""
			}
			return m, nil

		case "shift+tab":
			if m.activePane == paneContent {
				m.activePane = paneDates
			} else if m.activePane == paneDates {
				m.activePane = paneRepos
				m.selectedDate = -1
				m.contentReady = false
				m.viewport.SetContent("")
				m.contentHeader = ""
			} else {
				m.activePane = paneDates
			}
			return m, nil

		// Right arrow: move from left panes into content (if loaded)
		case "right", "l":
			if m.activePane != paneContent && m.contentReady {
				m.activePane = paneContent
			}
			return m, nil

		// Left arrow: move from content back to dates
		case "left", "h":
			if m.activePane == paneContent {
				m.activePane = paneDates
			}
			return m, nil

		// ── Navigation ─────────────────────────────────────────────

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
			case paneContent:
				var cmd tea.Cmd
				m.viewport.LineUp(1)
				return m, cmd
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
				if len(m.dateItems) == 0 {
					m.dateItems = m.buildDateHierarchy()
				}
				if m.dateCursor < len(m.dateItems)-1 {
					m.dateCursor++
					m.ensureDateVisible()
				}
			case paneContent:
				var cmd tea.Cmd
				m.viewport.LineDown(1)
				return m, cmd
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
					m.dateItems = m.buildDateHierarchy() // Rebuild hierarchy
					m.activePane = paneDates
				}
			case paneDates:
				// Rebuild date items if needed
				if len(m.dateItems) == 0 {
					m.dateItems = m.buildDateHierarchy()
				}

				if m.dateCursor >= 0 && m.dateCursor < len(m.dateItems) {
					item := m.dateItems[m.dateCursor]

					switch item.Type {
					case "month":
						// Load month content immediately on enter
						m.selectedDate = m.dateCursor
						m.loadContent()
						// Also toggle expansion
						monthKey := item.Date.Format("2006-01")
						m.expandedMonths[monthKey] = !m.expandedMonths[monthKey]
						m.dateItems = m.buildDateHierarchy()
						m.ensureDateVisible()

					case "week":
						// Load week content immediately on enter
						m.selectedDate = m.dateCursor
						m.loadContent()
						// Also toggle expansion
						weekKey := item.Date.Format("2006-W01-02")
						m.expandedWeeks[weekKey] = !m.expandedWeeks[weekKey]
						m.dateItems = m.buildDateHierarchy()
						m.ensureDateVisible()

					case "day":
						// Load day content
						m.selectedDate = m.dateCursor
						m.loadContent()
					}
				}
			}
			return m, nil

		case "esc":
			switch m.activePane {
			case paneDates:
				if m.dateCursor >= 0 && m.dateCursor < len(m.dateItems) {
					item := m.dateItems[m.dateCursor]
					if item.Indent > 0 {
						// Find parent
						parentIndex := -1
						for i := m.dateCursor; i >= 0; i-- {
							if m.dateItems[i].Indent < item.Indent {
								parentIndex = i
								break
							}
						}

						if parentIndex != -1 {
							// Collapse the current level if it was expanded
							if item.Type == "day" {
								// Find the parent week and collapse it
								weekKey := ""
								for i := m.dateCursor; i >= 0; i-- {
									if m.dateItems[i].Type == "week" {
										weekKey = m.dateItems[i].Date.Format("2006-W01-02")
										break
									}
								}
								if weekKey != "" && m.expandedWeeks[weekKey] {
									m.expandedWeeks[weekKey] = false
								}
							} else if item.Type == "week" {
								monthKey := item.Date.Format("2006-01")
								if m.expandedMonths[monthKey] {
									m.expandedMonths[monthKey] = false
								}
							}

							m.dateCursor = parentIndex
							m.dateItems = m.buildDateHierarchy()
							m.ensureDateVisible()
						}
					}
				}
			}
			return m, nil

		// ── Content scrolling (works from any pane) ────────────────

		case "pgup":
			if m.contentReady {
				m.viewport.HalfViewUp()
			}
			return m, nil

		case "pgdown":
			if m.contentReady {
				m.viewport.HalfViewDown()
			}
			return m, nil

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

// ── Operation runners ──────────────────────────────────────────────────────

// runIngestCmd creates a command that closes the db, runs ingest, and returns the result.
// Uses standalone function (not pointer receiver) to avoid Bubbletea value-receiver issues.
func runIngestCmd(repo ConsoleCodebase, currentDB *db.SQLRepository, profileName string) tea.Cmd {
	repoPath := repo.Path
	repoID := repo.ID

	return func() tea.Msg {
		// Close read-only connection so subprocess can get write lock
		closeReadOnlyConnection(currentDB)

		output, err := executeIngest(repoPath, profileName)

		return operationCompleteMsg{
			opType: "ingest",
			repoID: repoID,
			err:    err,
			output: output,
		}
	}
}

// runWorklogCmd creates a command that closes the db, runs worklog, and returns the result.
func runWorklogCmd(repo ConsoleCodebase, currentDB *db.SQLRepository, profileName string) tea.Cmd {
	repoPath := repo.Path
	repoID := repo.ID

	return func() tea.Msg {
		// Close read-only connection so subprocess can get write lock
		closeReadOnlyConnection(currentDB)

		output, err := executeWorklog(repoPath)

		return operationCompleteMsg{
			opType: "worklog",
			repoID: repoID,
			err:    err,
			output: output,
		}
	}
}

// runIngestAndWorklogCmd creates a command that runs ingest then worklog sequentially.
func runIngestAndWorklogCmd(repo ConsoleCodebase, currentDB *db.SQLRepository, profileName string) tea.Cmd {
	repoPath := repo.Path
	repoID := repo.ID

	return func() tea.Msg {
		// Close read-only connection so subprocess can get write lock
		closeReadOnlyConnection(currentDB)

		// Run ingest first
		ingestOutput, err := executeIngest(repoPath, profileName)
		if err != nil {
			return operationCompleteMsg{
				opType: "ingest+worklog",
				repoID: repoID,
				err:    fmt.Errorf("ingest failed: %w", err),
				output: ingestOutput,
			}
		}

		// Run worklog after successful ingest
		worklogOutput, err := executeWorklog(repoPath)

		// Combine outputs
		combinedOutput := fmt.Sprintf("=== Ingest ===\n%s\n\n=== Worklog ===\n%s", ingestOutput, worklogOutput)

		return operationCompleteMsg{
			opType: "ingest+worklog",
			repoID: repoID,
			err:    err,
			output: combinedOutput,
		}
	}
}

// reloadConsoleDataCmd creates a fresh read-only connection and reloads all data.
// The new dbRepo is returned inside the message so Update() can set it on the model.
func reloadConsoleDataCmd(profileName string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		// Get a fresh read-only connection to see updated data
		newRepo, err := db.GetReadOnlyRepositoryForProfile(profileName)
		if err != nil {
			return reloadDataMsg{} // empty; will be handled gracefully
		}

		// Reload codebases
		codebases, err := newRepo.GetAllCodebases(ctx)
		if err != nil {
			return reloadDataMsg{dbRepo: newRepo}
		}

		// Rebuild console data
		var newCodebases []ConsoleCodebase
		for _, cb := range codebases {
			dates, err := newRepo.ListWorklogDates(ctx, cb.ID, profileName)
			if err != nil {
				continue
			}

			tuiDates := make([]ConsoleDate, len(dates))
			for i, d := range dates {
				tuiDates[i] = ConsoleDate{
					EntryDate:   d.EntryDate,
					EntryCount:  d.EntryCount,
					CommitCount: d.CommitCount,
					Additions:   d.Additions,
					Deletions:   d.Deletions,
				}
			}

			// Load weeks
			weeks, err := newRepo.ListWorklogWeeks(ctx, cb.ID, profileName)
			if err != nil {
				weeks = nil
			}

			tuiWeeks := make([]ConsoleWeek, len(weeks))
			for i, w := range weeks {
				// Find dates that belong to this week
				var weekDates []ConsoleDate
				for _, d := range tuiDates {
					if !d.EntryDate.Before(w.WeekStart) && !d.EntryDate.After(w.WeekEnd) {
						weekDates = append(weekDates, d)
					}
				}

				tuiWeeks[i] = ConsoleWeek{
					WeekStart:   w.WeekStart,
					WeekEnd:     w.WeekEnd,
					DateCount:   w.DateCount,
					EntryCount:  w.EntryCount,
					CommitCount: w.CommitCount,
					Additions:   w.Additions,
					Deletions:   w.Deletions,
					Dates:       weekDates,
				}
			}

			// Load months
			months, err := newRepo.ListWorklogMonths(ctx, cb.ID, profileName)
			if err != nil {
				months = nil
			}

			tuiMonths := make([]ConsoleMonth, len(months))
			for i, m := range months {
				// Find weeks that belong to this month
				var monthWeeks []ConsoleWeek
				for _, w := range tuiWeeks {
					if !w.WeekStart.Before(m.MonthStart) && !w.WeekStart.After(m.MonthEnd) {
						monthWeeks = append(monthWeeks, w)
					}
				}

				tuiMonths[i] = ConsoleMonth{
					MonthStart:  m.MonthStart,
					MonthEnd:    m.MonthEnd,
					DateCount:   m.DateCount,
					WeekCount:   m.WeekCount,
					EntryCount:  m.EntryCount,
					CommitCount: m.CommitCount,
					Additions:   m.Additions,
					Deletions:   m.Deletions,
					Weeks:       monthWeeks,
				}
			}

			// Check if repository has been ingested
			commitCount, err := newRepo.GetCommitCount(ctx, cb.ID)
			if err != nil {
				commitCount = 0
			}

			newCodebases = append(newCodebases, ConsoleCodebase{
				ID:          cb.ID,
				Name:        cb.Name,
				Path:        cb.Path,
				DateCount:   len(dates),
				Dates:       tuiDates,
				Weeks:       tuiWeeks,
				Months:      tuiMonths,
				CommitCount: int(commitCount),
				IsIngested:  commitCount > 0,
			})
		}

		// Return message with both new data AND the new db connection
		return reloadDataMsg{codebases: newCodebases, dbRepo: newRepo}
	}
}

type reloadDataMsg struct {
	codebases []ConsoleCodebase
	dbRepo    *db.SQLRepository
}

// ── Helpers ────────────────────────────────────────────────────────────────

func (m *ConsoleModel) currentDates() []ConsoleDate {
	if m.selectedRepo >= 0 && m.selectedRepo < len(m.codebases) {
		return m.codebases[m.selectedRepo].Dates
	}
	return nil
}

// buildDateHierarchy creates a flattened hierarchical view of months, weeks, and days
func (m *ConsoleModel) buildDateHierarchy() []DateItem {
	if m.selectedRepo < 0 || m.selectedRepo >= len(m.codebases) {
		return nil
	}

	cb := m.codebases[m.selectedRepo]
	var items []DateItem

	// If we have months with weeks, show hierarchical view
	if len(cb.Months) > 0 {
		for _, month := range cb.Months {
			monthKey := month.MonthStart.Format("2006-01")
			monthItem := DateItem{
				Type:        "month",
				Date:        month.MonthStart,
				DisplayText: month.MonthStart.Format("January 2006"),
				Stats:       fmt.Sprintf("+%d/-%d", month.Additions, month.Deletions),
				Indent:      0,
				IsExpanded:  m.expandedMonths[monthKey],
			}

			items = append(items, monthItem)

			// If month is expanded, add weeks
			if m.expandedMonths[monthKey] && len(month.Weeks) > 0 {
				for _, week := range month.Weeks {
					// Use consistent week key format across all contexts
					weekKey := week.WeekStart.Format("2006-W01-02") // e.g., "2026-W02-09"
					weekItem := DateItem{
						Type:        "week",
						Date:        week.WeekStart,
						DisplayText: fmt.Sprintf("Week of %s", week.WeekStart.Format("Jan 2")),
						Stats:       fmt.Sprintf("+%d/-%d", week.Additions, week.Deletions),
						Indent:      1,
						IsExpanded:  m.expandedWeeks[weekKey],
					}

					items = append(items, weekItem)

					// If week is expanded, add days
					if m.expandedWeeks[weekKey] && len(week.Dates) > 0 {
						for _, day := range week.Dates {
							dayItem := DateItem{
								Type:        "day",
								Date:        day.EntryDate,
								DisplayText: day.EntryDate.Format("Mon, Jan 2"),
								Stats:       fmt.Sprintf("+%d/-%d", day.Additions, day.Deletions),
								Indent:      2,
							}
							items = append(items, dayItem)
						}
					}
				}
			}
		}
	} else if len(cb.Weeks) > 0 {
		// Show weeks with days if no months
		for _, week := range cb.Weeks {
			// Use consistent week key format
			weekKey := week.WeekStart.Format("2006-W01-02")
			weekItem := DateItem{
				Type:        "week",
				Date:        week.WeekStart,
				DisplayText: fmt.Sprintf("Week of %s", week.WeekStart.Format("Jan 2")),
				Stats:       fmt.Sprintf("+%d/-%d", week.Additions, week.Deletions),
				Indent:      0,
				IsExpanded:  m.expandedWeeks[weekKey],
			}

			items = append(items, weekItem)

			// If week is expanded, add days
			if m.expandedWeeks[weekKey] && len(week.Dates) > 0 {
				for _, day := range week.Dates {
					dayItem := DateItem{
						Type:        "day",
						Date:        day.EntryDate,
						DisplayText: day.EntryDate.Format("Mon, Jan 2"),
						Stats:       fmt.Sprintf("+%d/-%d", day.Additions, day.Deletions),
						Indent:      1,
					}
					items = append(items, dayItem)
				}
			}
		}
	} else {
		// Fall back to flat date list
		for _, day := range cb.Dates {
			items = append(items, DateItem{
				Type:        "day",
				Date:        day.EntryDate,
				DisplayText: day.EntryDate.Format("Mon, Jan 2"),
				Stats:       fmt.Sprintf("+%d/-%d", day.Additions, day.Deletions),
				Indent:      0,
			})
		}
	}

	return items
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
	if m.dbRepo == nil {
		return
	}

	// Rebuild date items if needed
	if len(m.dateItems) == 0 {
		m.dateItems = m.buildDateHierarchy()
	}

	if m.selectedDate < 0 || m.selectedDate >= len(m.dateItems) {
		return
	}

	cb := m.codebases[m.selectedRepo]
	item := m.dateItems[m.selectedDate]

	ctx := context.Background()
	var md string
	var header string

	switch item.Type {
	case "month":
		// Load monthly summary
		summary, err := m.dbRepo.GetMonthlySummary(ctx, cb.ID, m.profileName, item.Date)
		if err != nil || summary == nil {
			m.contentReady = true
			m.contentHeader = item.Date.Format("January 2006") + " - Monthly Summary"
			m.viewport.SetContent(consoleEmptyStyle.Render("No monthly summary available.\nGenerate worklogs to create summaries."))
			return
		}
		md = fmt.Sprintf("# Monthly Summary - %s\n\n%s", item.Date.Format("January 2006"), summary.Content)
		header = fmt.Sprintf("%s  -  %s", item.Date.Format("January 2006"), cb.Name)

	case "week":
		// Load weekly summary
		summary, err := m.dbRepo.GetWeeklySummary(ctx, cb.ID, m.profileName, item.Date)
		if err != nil || summary == nil {
			m.contentReady = true
			m.contentHeader = fmt.Sprintf("Week of %s - Weekly Summary", item.Date.Format("Jan 2"))
			m.viewport.SetContent(consoleEmptyStyle.Render("No weekly summary available.\nGenerate worklogs spanning >7 days to create weekly summaries."))
			return
		}
		weekEnd := item.Date.AddDate(0, 0, 6)
		md = fmt.Sprintf("# Weekly Summary\n\n**%s - %s**\n\n%s", item.Date.Format("Jan 2"), weekEnd.Format("Jan 2, 2006"), summary.Content)
		header = fmt.Sprintf("Week of %s  -  %s", item.Date.Format("Jan 2"), cb.Name)

	case "day":
		// Load day entries
		entries, err := m.dbRepo.ListWorklogEntriesByDate(ctx, cb.ID, m.profileName, item.Date)
		if err != nil || len(entries) == 0 {
			m.contentReady = true
			m.contentHeader = item.Date.Format("Monday, January 2, 2006")
			m.viewport.SetContent(consoleEmptyStyle.Render("No worklog entries for this date.\nRun 'devlog worklog' to generate worklogs."))
			return
		}
		md = renderDayMarkdown(entries, item.Date)
		header = fmt.Sprintf("%s  -  %s", item.Date.Format("Monday, January 2, 2006"), cb.Name)

	default:
		return
	}

	// Render markdown
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
	m.contentHeader = header
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

	borderStyle := activeBorderStyle
	if m.activePane == paneContent {
		borderStyle = inactiveBorderStyle
	}

	return borderStyle.
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
		maxNameLen := width - 18 // Reserve space for badges
		if len(name) > maxNameLen {
			name = name[:maxNameLen-3] + "..."
		}

		// Status indicator
		statusIndicator := ""
		if cb.IsIngested {
			statusIndicator = lipgloss.NewStyle().
				Foreground(lipgloss.Color("40")).
				Render("✓")
		} else {
			statusIndicator = lipgloss.NewStyle().
				Foreground(lipgloss.Color("241")).
				Render("○")
		}

		badge := ""
		if cb.DateCount > 0 {
			badge = consoleStatStyle.Render(fmt.Sprintf(" %dd", cb.DateCount))
		}

		var line string
		if isCursor {
			line = consoleCursorStyle.Render(cursor + statusIndicator + " " + name)
		} else if isSelected {
			line = consoleItemStyle.Bold(true).Render(cursor + statusIndicator + " " + name)
		} else {
			line = consoleItemStyle.Render(cursor + statusIndicator + " " + name)
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

	sectionLabel := "Timeline"
	if m.activePane == paneDates {
		b.WriteString(consoleSectionTitle.Render(sectionLabel))
	} else {
		b.WriteString(consoleDimStyle.Bold(true).Render(" " + sectionLabel))
	}
	b.WriteString("\n")
	b.WriteString(consoleDimStyle.Render(" " + strings.Repeat("─", width-2)))
	b.WriteString("\n")

	// Rebuild date items if needed
	if len(m.dateItems) == 0 {
		m.dateItems = m.buildDateHierarchy()
	}

	items := m.dateItems
	if len(items) == 0 {
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
	if end > len(items) {
		end = len(items)
	}

	if start > 0 {
		b.WriteString(consoleDimStyle.Render(" ↑ more"))
		b.WriteString("\n")
	}

	for i := start; i < end; i++ {
		item := items[i]
		isSelected := i == m.selectedDate
		isCursor := i == m.dateCursor && m.activePane == paneDates

		// Build indentation
		indent := strings.Repeat("  ", item.Indent)

		cursor := "  "
		if isCursor {
			cursor = "> "
		}

		// Add expansion indicator for months and weeks
		expandIcon := ""
		if item.Type == "month" {
			monthKey := item.Date.Format("2006-01")
			if m.expandedMonths[monthKey] {
				expandIcon = "▼ "
			} else {
				expandIcon = "▶ "
			}
		} else if item.Type == "week" {
			// Use consistent week key format
			weekKey := item.Date.Format("2006-W01-02")
			if m.expandedWeeks[weekKey] {
				expandIcon = "▼ "
			} else {
				expandIcon = "▶ "
			}
		}

		displayText := item.DisplayText
		stats := consoleStatStyle.Render(fmt.Sprintf(" %s", item.Stats))

		var line string
		lineContent := cursor + indent + expandIcon + displayText

		if isCursor {
			line = consoleCursorStyle.Render(lineContent)
		} else if isSelected {
			line = consoleItemStyle.Bold(true).Render(lineContent)
		} else {
			// Different colors for different types
			switch item.Type {
			case "month":
				line = lipgloss.NewStyle().Foreground(lipgloss.Color("86")).Bold(true).Render(lineContent)
			case "week":
				line = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(lineContent)
			default:
				line = consoleItemStyle.Render(lineContent)
			}
		}

		b.WriteString(line + stats)
		b.WriteString("\n")
	}

	if end < len(items) {
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

	// Show operation status if running
	if m.operationRunning {
		content = m.renderOperationStatus(rightW, panelHeight)
	} else if m.operationError != "" {
		content = m.renderOperationError(rightW, panelHeight)
	} else if m.operationOutput != "" {
		content = m.renderOperationSuccess(rightW, panelHeight)
	} else if !m.contentReady {
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

	borderStyle := inactiveBorderStyle
	if m.activePane == paneContent {
		borderStyle = activeBorderStyle
	}

	return borderStyle.
		Width(rightW).
		Height(panelHeight).
		Render(content)
}

func (m ConsoleModel) renderOperationSuccess(width, height int) string {
	var b strings.Builder

	// Success icon and title
	successTitle := "✓ Operation Complete"
	successStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("40")).
		Bold(true)

	b.WriteString("\n")
	pad := (width - len(successTitle)) / 2
	if pad < 0 {
		pad = 2
	}
	b.WriteString(strings.Repeat(" ", pad))
	b.WriteString(successStyle.Render(successTitle))
	b.WriteString("\n\n")

	// Show full output
	if m.operationOutput != "" {
		outputLines := wrapOutput(m.operationOutput, width-4)
		maxLines := height - 8 // Reserve space for title and footer

		startLine := 0
		if len(outputLines) > maxLines {
			startLine = len(outputLines) - maxLines
		}

		for i := startLine; i < len(outputLines) && i-startLine < maxLines; i++ {
			line := outputLines[i]
			b.WriteString("  ")
			b.WriteString(consoleItemStyle.Render(line))
			b.WriteString("\n")
		}

		if len(outputLines) > maxLines {
			b.WriteString("\n")
			scrollHint := fmt.Sprintf("(showing last %d of %d lines)", maxLines, len(outputLines))
			scrollPad := (width - len(scrollHint)) / 2
			if scrollPad < 0 {
				scrollPad = 2
			}
			b.WriteString(strings.Repeat(" ", scrollPad))
			b.WriteString(consoleDimStyle.Render(scrollHint))
		}
	}

	b.WriteString("\n")

	// Hint
	hint := "Press any key to continue"
	hintPad := (width - len(hint)) / 2
	if hintPad < 0 {
		hintPad = 2
	}
	b.WriteString(strings.Repeat(" ", hintPad))
	b.WriteString(consoleDimStyle.Italic(true).Render(hint))

	return b.String()
}

func (m ConsoleModel) renderOperationError(width, height int) string {
	var b strings.Builder

	// Error icon and title
	errorTitle := "✗ Operation Failed"
	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("196")).
		Bold(true)

	b.WriteString("\n")
	pad := (width - len(errorTitle)) / 2
	if pad < 0 {
		pad = 2
	}
	b.WriteString(strings.Repeat(" ", pad))
	b.WriteString(errorStyle.Render(errorTitle))
	b.WriteString("\n\n")

	// Show full output if available, wrap long lines
	if m.operationOutput != "" {
		outputLines := wrapOutput(m.operationOutput, width-4)
		maxLines := height - 8 // Reserve space for title and footer

		startLine := 0
		if len(outputLines) > maxLines {
			startLine = len(outputLines) - maxLines
		}

		for i := startLine; i < len(outputLines) && i-startLine < maxLines; i++ {
			line := outputLines[i]
			b.WriteString("  ")
			b.WriteString(consoleItemStyle.Render(line))
			b.WriteString("\n")
		}

		if len(outputLines) > maxLines {
			b.WriteString("\n")
			scrollHint := fmt.Sprintf("(showing last %d of %d lines)", maxLines, len(outputLines))
			scrollPad := (width - len(scrollHint)) / 2
			if scrollPad < 0 {
				scrollPad = 2
			}
			b.WriteString(strings.Repeat(" ", scrollPad))
			b.WriteString(consoleDimStyle.Render(scrollHint))
		}
	} else {
		// Just show error message
		errLines := wrapOutput(m.operationError, width-4)
		for _, line := range errLines {
			errPad := 2
			b.WriteString(strings.Repeat(" ", errPad))
			b.WriteString(consoleDimStyle.Render(line))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")

	// Hint
	hint := "Press any key to continue"
	hintPad := (width - len(hint)) / 2
	if hintPad < 0 {
		hintPad = 2
	}
	b.WriteString(strings.Repeat(" ", hintPad))
	b.WriteString(consoleDimStyle.Italic(true).Render(hint))

	return b.String()
}

// wrapOutput wraps text to fit within the specified width
func wrapOutput(text string, maxWidth int) []string {
	if maxWidth < 10 {
		maxWidth = 10
	}

	lines := strings.Split(text, "\n")
	var wrapped []string

	for _, line := range lines {
		if len(line) <= maxWidth {
			wrapped = append(wrapped, line)
			continue
		}

		// Wrap long lines
		for len(line) > maxWidth {
			// Try to break at a space
			breakPoint := maxWidth
			for i := maxWidth - 1; i > maxWidth/2; i-- {
				if line[i] == ' ' || line[i] == ',' || line[i] == ':' {
					breakPoint = i + 1
					break
				}
			}

			wrapped = append(wrapped, strings.TrimRight(line[:breakPoint], " "))
			line = strings.TrimLeft(line[breakPoint:], " ")
		}

		if len(line) > 0 {
			wrapped = append(wrapped, line)
		}
	}

	return wrapped
}

func (m ConsoleModel) renderOperationStatus(width, height int) string {
	var b strings.Builder

	// Center vertically
	topPad := (height - 10) / 2
	if topPad < 2 {
		topPad = 2
	}

	for i := 0; i < topPad; i++ {
		b.WriteString("\n")
	}

	// Operation type
	opType := m.operationType
	if opType == "ingest+worklog" {
		opType = "Ingest + Worklog"
	} else {
		opType = strings.Title(opType)
	}
	opTitle := "Running " + opType
	pad := (width - len(opTitle)) / 2
	if pad < 0 {
		pad = 0
	}
	b.WriteString(strings.Repeat(" ", pad))
	b.WriteString(logoMainStyle.Render(opTitle))
	b.WriteString("\n\n")

	// Spinner/progress indicator
	spinner := "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"
	spinChar := string(spinner[int(time.Now().Unix())%len(spinner)])
	spinnerLine := fmt.Sprintf("%s  Processing...", spinChar)
	spinPad := (width - len(spinnerLine)) / 2
	if spinPad < 0 {
		spinPad = 0
	}
	b.WriteString(strings.Repeat(" ", spinPad))
	b.WriteString(logoSubStyle.Render(spinnerLine))
	b.WriteString("\n\n")

	// Repo info
	if m.operationRepo != "" {
		for _, cb := range m.codebases {
			if cb.ID == m.operationRepo {
				repoLine := fmt.Sprintf("Repository: %s", cb.Name)
				repoPad := (width - len(repoLine)) / 2
				if repoPad < 0 {
					repoPad = 0
				}
				b.WriteString(strings.Repeat(" ", repoPad))
				b.WriteString(consoleDimStyle.Render(repoLine))
				b.WriteString("\n")
				break
			}
		}
	}

	b.WriteString("\n\n")
	hint := "Please wait, this may take a few moments..."
	hintPad := (width - len(hint)) / 2
	if hintPad < 0 {
		hintPad = 0
	}
	b.WriteString(strings.Repeat(" ", hintPad))
	b.WriteString(consoleDimStyle.Italic(true).Render(hint))

	return b.String()
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
	logoHeight := len(logo) + 14 // logo lines + spacing + subtitle + hints + shortcuts
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
	b.WriteString("\n\n\n")

	// Quick Actions section
	actionsTitle := "Quick Actions"
	actionsPad := (width - len(actionsTitle)) / 2
	if actionsPad < 0 {
		actionsPad = 2
	}
	b.WriteString(strings.Repeat(" ", actionsPad))
	b.WriteString(logoSubStyle.Bold(true).Render(actionsTitle))
	b.WriteString("\n\n")

	// Keyboard shortcuts - styled nicely
	keyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("86")).
		Bold(true)

	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245"))

	shortcuts := []struct {
		key  string
		desc string
	}{
		{"Shift+G", "Run ingest + worklog on selected repo"},
		{"Shift+I", "Run ingest on selected repo"},
		{"Shift+W", "Generate worklog for selected repo"},
	}

	for _, sc := range shortcuts {
		line := fmt.Sprintf("%s  %s", keyStyle.Render(sc.key), descStyle.Render(sc.desc))
		linePad := (width - lipgloss.Width(line)) / 2
		if linePad < 0 {
			linePad = 2
		}
		b.WriteString(strings.Repeat(" ", linePad))
		b.WriteString(line)
		b.WriteString("\n")
	}

	b.WriteString("\n")

	// Hint
	hint := "Select a repo, then navigate the timeline to view worklogs"
	hintPad := (width - len(hint)) / 2
	if hintPad < 0 {
		hintPad = 0
	}
	b.WriteString(strings.Repeat(" ", hintPad))
	b.WriteString(consoleDimStyle.Italic(true).Render(hint))

	return b.String()
}

// helpItem renders a single key+description pair for the help bar.
func helpItem(key, desc string) string {
	return helpKeyStyle.Render(key) + helpDescStyle.Render(desc)
}

func (m ConsoleModel) renderHelpBar() string {
	sep := helpSepStyle.Render(" ")

	var items []string
	switch m.activePane {
	case paneRepos:
		if m.operationRunning {
			items = []string{
				helpItem("...", "operation in progress"),
				helpItem("q", "quit"),
			}
		} else {
			items = []string{
				helpItem("↑↓", "navigate"),
				helpItem("enter", "select"),
				helpItem("Shift+G", "ingest+log"),
				helpItem("Shift+I", "ingest"),
				helpItem("Shift+W", "worklog"),
				helpItem("tab", "dates"),
				helpItem("→", "content"),
				helpItem("q", "quit"),
			}
		}
	case paneDates:
		items = []string{
			helpItem("↑↓", "navigate"),
			helpItem("enter", "view/expand"),
			helpItem("esc", "up"),
			helpItem("tab", "repos"),
			helpItem("→", "content"),
			helpItem("pgup/dn", "scroll"),
			helpItem("q", "quit"),
		}
	case paneContent:
		items = []string{
			helpItem("↑↓", "scroll"),
			helpItem("pgup/dn", "page"),
			helpItem("←", "back"),
			helpItem("tab", "panels"),
			helpItem("q", "quit"),
		}
	}

	bar := strings.Join(items, sep)
	barWidth := lipgloss.Width(bar)

	spacerLen := m.width - barWidth
	if spacerLen < 0 {
		spacerLen = 0
	}
	spacer := helpBarBg.Render(strings.Repeat(" ", spacerLen))

	return bar + spacer
}

// ── Runner ─────────────────────────────────────────────────────────────────

// RunConsole launches the full-screen console TUI.
func RunConsole(codebases []ConsoleCodebase, profileName string, dbRepo *db.SQLRepository) error {
	model := NewConsoleModel(codebases, profileName, dbRepo)
	p := tea.NewProgram(model, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// ── External command executors ─────────────────────────────────────────────

// closeReadOnlyConnection safely closes a read-only database connection
func closeReadOnlyConnection(repo *db.SQLRepository) {
	if repo != nil {
		repo.Close()
	}
}

// executeIngest runs the ingest command on a repository
func executeIngest(repoPath, profileName string) (string, error) {
	// Get the path to the current devlog executable
	devlogPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}

	// Run devlog ingest with --all-branches and --skip-worklog flags to avoid interactive prompts
	cmd := exec.Command(devlogPath, "ingest", repoPath, "--all-branches", "--skip-worklog")
	cmd.Dir = repoPath

	// Capture output
	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	if err != nil {
		return outputStr, fmt.Errorf("ingest failed: %w", err)
	}

	return outputStr, nil
}

// executeWorklog runs the worklog command on a repository
func executeWorklog(repoPath string) (string, error) {
	// Get the path to the current devlog executable
	devlogPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}

	// Run devlog worklog
	cmd := exec.Command(devlogPath, "worklog")
	cmd.Dir = repoPath

	// Capture output
	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	if err != nil {
		return outputStr, fmt.Errorf("worklog failed: %w", err)
	}

	return outputStr, nil
}
