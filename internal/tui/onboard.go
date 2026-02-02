package tui

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ishaan812/devlog/internal/config"
)

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("86")).
			MarginBottom(1)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			MarginBottom(1)

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("86")).
			Bold(true)

	normalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("82")).
			Bold(true)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	inputStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("86")).
			Padding(0, 1)
)

// Steps in the onboarding flow
type step int

const (
	stepWelcome step = iota
	stepProfileName
	stepProfileDesc
	stepProvider
	stepProviderConfig
	stepGitHubUsername
	stepUserEmail
	stepSuccess
)

// Provider options
type providerOption struct {
	name        string
	description string
}

var providers = []providerOption{
	{"ollama", "Local, free, private - requires Ollama installed"},
	{"anthropic", "Claude AI - high quality, requires API key"},
	{"openai", "GPT models - widely used, requires API key"},
	{"bedrock", "AWS Bedrock - enterprise, requires AWS credentials"},
}

// Model for the onboarding TUI
type Model struct {
	step          step
	config        *config.Config
	profileName   string
	profileDesc   string
	selectedIdx   int
	textInput     textinput.Model
	spinner       spinner.Model
	testing       bool
	testResult    string
	testSuccess   bool
	err           error
	width         int
	height        int
	animationTick int
}

// Messages
type tickMsg time.Time
type testResultMsg struct {
	success bool
	message string
}

// NewModel creates a new onboarding model
func NewModel() Model {
	ti := textinput.New()
	ti.Focus()
	ti.CharLimit = 64
	ti.Width = 40

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("86"))

	return Model{
		step:      stepWelcome,
		config:    &config.Config{},
		textInput: ti,
		spinner:   s,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		tickCmd(),
	)
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Millisecond*100, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func testProvider(provider, apiKey, baseURL string) tea.Cmd {
	return func() tea.Msg {
		switch provider {
		case "ollama":
			url := baseURL
			if url == "" {
				url = "http://localhost:11434"
			}
			resp, err := http.Get(url + "/api/tags")
			if err != nil {
				return testResultMsg{false, "Cannot connect to Ollama. Is it running?"}
			}
			defer resp.Body.Close()
			if resp.StatusCode == 200 {
				return testResultMsg{true, "Connected to Ollama!"}
			}
			return testResultMsg{false, fmt.Sprintf("Ollama returned status %d", resp.StatusCode)}

		case "anthropic":
			if apiKey == "" {
				return testResultMsg{false, "API key is required"}
			}
			// Simple validation - just check it looks like an API key
			if !strings.HasPrefix(apiKey, "sk-ant-") {
				return testResultMsg{false, "Invalid API key format (should start with sk-ant-)"}
			}
			return testResultMsg{true, "API key format valid!"}

		case "openai":
			if apiKey == "" {
				return testResultMsg{false, "API key is required"}
			}
			if !strings.HasPrefix(apiKey, "sk-") {
				return testResultMsg{false, "Invalid API key format (should start with sk-)"}
			}
			return testResultMsg{true, "API key format valid!"}

		case "bedrock":
			if apiKey == "" {
				return testResultMsg{false, "AWS Access Key ID is required"}
			}
			return testResultMsg{true, "AWS credentials configured!"}
		}
		return testResultMsg{true, "Configuration saved!"}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			if m.step == stepWelcome || m.step == stepSuccess {
				return m, tea.Quit
			}
		case "enter":
			return m.handleEnter()
		case "up", "k":
			if m.step == stepProvider && m.selectedIdx > 0 {
				m.selectedIdx--
			}
		case "down", "j":
			if m.step == stepProvider && m.selectedIdx < len(providers)-1 {
				m.selectedIdx++
			}
		case "esc":
			if m.step > stepWelcome && m.step < stepSuccess {
				m.step--
				m.prepareStep()
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tickMsg:
		m.animationTick++
		if m.testing {
			m.spinner, cmd = m.spinner.Update(msg)
			return m, tea.Batch(cmd, tickCmd())
		}
		return m, tickCmd()

	case testResultMsg:
		m.testing = false
		m.testSuccess = msg.success
		m.testResult = msg.message
		if msg.success {
			// Auto-advance after successful test
			return m.advanceStep()
		}

	case spinner.TickMsg:
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	// Update text input
	if m.step == stepProfileName || m.step == stepProfileDesc ||
		m.step == stepProviderConfig || m.step == stepGitHubUsername || m.step == stepUserEmail {
		m.textInput, cmd = m.textInput.Update(msg)
	}

	return m, cmd
}

func (m Model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.step {
	case stepWelcome:
		m.step = stepProfileName
		m.prepareStep()
		return m, nil

	case stepProfileName:
		name := strings.TrimSpace(m.textInput.Value())
		if name == "" {
			name = "default"
		}
		m.profileName = name
		m.step = stepProfileDesc
		m.prepareStep()
		return m, nil

	case stepProfileDesc:
		m.profileDesc = strings.TrimSpace(m.textInput.Value())
		m.step = stepProvider
		return m, nil

	case stepProvider:
		m.config.DefaultProvider = providers[m.selectedIdx].name
		m.step = stepProviderConfig
		m.prepareStep()
		return m, nil

	case stepProviderConfig:
		value := strings.TrimSpace(m.textInput.Value())
		switch m.config.DefaultProvider {
		case "ollama":
			if value != "" {
				m.config.OllamaBaseURL = value
			} else {
				m.config.OllamaBaseURL = "http://localhost:11434"
			}
		case "anthropic":
			m.config.AnthropicAPIKey = value
		case "openai":
			m.config.OpenAIAPIKey = value
		case "bedrock":
			m.config.AWSAccessKeyID = value
		}
		// Test the configuration
		m.testing = true
		m.testResult = ""
		return m, tea.Batch(
			m.spinner.Tick,
			testProvider(m.config.DefaultProvider, value, m.config.OllamaBaseURL),
		)

	case stepGitHubUsername:
		m.config.GitHubUsername = strings.TrimSpace(m.textInput.Value())
		m.step = stepUserEmail
		m.prepareStep()
		return m, nil

	case stepUserEmail:
		m.config.UserEmail = strings.TrimSpace(m.textInput.Value())
		return m.finishOnboarding()

	case stepSuccess:
		return m, tea.Quit
	}

	return m, nil
}

func (m Model) advanceStep() (tea.Model, tea.Cmd) {
	switch m.step {
	case stepProviderConfig:
		m.step = stepGitHubUsername
		m.prepareStep()
	}
	return m, nil
}

func (m *Model) prepareStep() {
	m.textInput.Reset()
	m.testResult = ""
	m.testSuccess = false

	switch m.step {
	case stepProfileName:
		m.textInput.Placeholder = "default"
		m.textInput.SetValue("")
	case stepProfileDesc:
		m.textInput.Placeholder = "My development profile"
		m.textInput.SetValue("")
	case stepProviderConfig:
		switch m.config.DefaultProvider {
		case "ollama":
			m.textInput.Placeholder = "http://localhost:11434"
		case "anthropic":
			m.textInput.Placeholder = "sk-ant-..."
			m.textInput.EchoMode = textinput.EchoPassword
		case "openai":
			m.textInput.Placeholder = "sk-..."
			m.textInput.EchoMode = textinput.EchoPassword
		case "bedrock":
			m.textInput.Placeholder = "AWS Access Key ID"
		}
	case stepGitHubUsername:
		m.textInput.Placeholder = "your-github-username"
		m.textInput.EchoMode = textinput.EchoNormal
	case stepUserEmail:
		m.textInput.Placeholder = "you@example.com (optional)"
	}
}

func (m Model) finishOnboarding() (tea.Model, tea.Cmd) {
	// Create the profile
	if err := m.config.CreateProfile(m.profileName, m.profileDesc); err != nil {
		m.err = err
		return m, nil
	}

	// Set as active profile
	m.config.ActiveProfile = m.profileName
	m.config.OnboardingComplete = true

	// Save config
	if err := m.config.Save(); err != nil {
		m.err = err
		return m, nil
	}

	m.step = stepSuccess
	return m, nil
}

func (m Model) View() string {
	var s strings.Builder

	switch m.step {
	case stepWelcome:
		s.WriteString(m.viewWelcome())
	case stepProfileName:
		s.WriteString(m.viewProfileName())
	case stepProfileDesc:
		s.WriteString(m.viewProfileDesc())
	case stepProvider:
		s.WriteString(m.viewProvider())
	case stepProviderConfig:
		s.WriteString(m.viewProviderConfig())
	case stepGitHubUsername:
		s.WriteString(m.viewGitHubUsername())
	case stepUserEmail:
		s.WriteString(m.viewUserEmail())
	case stepSuccess:
		s.WriteString(m.viewSuccess())
	}

	return s.String()
}

func (m Model) viewWelcome() string {
	// Animated banner
	banner := `
    ____             __
   / __ \___ _   __ / /   ____   ____ _
  / / / / _ \ | / // /   / __ \ / __ ` + "`" + `/
 / /_/ /  __/ |/ // /___/ /_/ // /_/ /
/_____/\___/|___//_____/\____/ \__, /
                              /____/
`
	// Fade in effect based on animation tick
	opacity := min(m.animationTick*20, 255)
	bannerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", opacity/3, opacity, opacity/2)))

	var s strings.Builder
	s.WriteString("\n")
	s.WriteString(bannerStyle.Render(banner))
	s.WriteString("\n")
	s.WriteString(titleStyle.Render("Welcome to DevLog!"))
	s.WriteString("\n\n")
	s.WriteString(normalStyle.Render("Track your development activity with natural language queries."))
	s.WriteString("\n\n")
	s.WriteString(dimStyle.Render("Let's get you set up. This will only take a moment."))
	s.WriteString("\n\n")
	s.WriteString(selectedStyle.Render("Press Enter to continue"))
	s.WriteString(dimStyle.Render(" or q to quit"))
	s.WriteString("\n")

	return s.String()
}

func (m Model) viewProfileName() string {
	var s strings.Builder
	s.WriteString("\n")
	s.WriteString(titleStyle.Render("Step 1: Create a Profile"))
	s.WriteString("\n\n")
	s.WriteString(normalStyle.Render("Profiles keep your data organized. You might have"))
	s.WriteString("\n")
	s.WriteString(normalStyle.Render("separate profiles for personal and work projects."))
	s.WriteString("\n\n")
	s.WriteString(dimStyle.Render("Profile name:"))
	s.WriteString("\n")
	s.WriteString(inputStyle.Render(m.textInput.View()))
	s.WriteString("\n\n")
	s.WriteString(dimStyle.Render("Press Enter to continue, Esc to go back"))
	s.WriteString("\n")
	return s.String()
}

func (m Model) viewProfileDesc() string {
	var s strings.Builder
	s.WriteString("\n")
	s.WriteString(titleStyle.Render("Step 1: Create a Profile"))
	s.WriteString("\n\n")
	s.WriteString(dimStyle.Render(fmt.Sprintf("Profile: %s", m.profileName)))
	s.WriteString("\n\n")
	s.WriteString(dimStyle.Render("Description (optional):"))
	s.WriteString("\n")
	s.WriteString(inputStyle.Render(m.textInput.View()))
	s.WriteString("\n\n")
	s.WriteString(dimStyle.Render("Press Enter to continue, Esc to go back"))
	s.WriteString("\n")
	return s.String()
}

func (m Model) viewProvider() string {
	var s strings.Builder
	s.WriteString("\n")
	s.WriteString(titleStyle.Render("Step 2: Choose LLM Provider"))
	s.WriteString("\n\n")
	s.WriteString(normalStyle.Render("DevLog uses an LLM to generate summaries and answer questions."))
	s.WriteString("\n\n")

	for i, p := range providers {
		cursor := "  "
		style := normalStyle
		if i == m.selectedIdx {
			cursor = "> "
			style = selectedStyle
		}
		s.WriteString(style.Render(fmt.Sprintf("%s%s", cursor, p.name)))
		s.WriteString("\n")
		s.WriteString(dimStyle.Render(fmt.Sprintf("    %s", p.description)))
		s.WriteString("\n")
	}

	s.WriteString("\n")
	s.WriteString(dimStyle.Render("Use arrow keys to select, Enter to confirm"))
	s.WriteString("\n")
	return s.String()
}

func (m Model) viewProviderConfig() string {
	var s strings.Builder
	s.WriteString("\n")
	s.WriteString(titleStyle.Render(fmt.Sprintf("Step 3: Configure %s", strings.Title(m.config.DefaultProvider))))
	s.WriteString("\n\n")

	switch m.config.DefaultProvider {
	case "ollama":
		s.WriteString(normalStyle.Render("Enter Ollama base URL (leave empty for default):"))
	case "anthropic":
		s.WriteString(normalStyle.Render("Enter your Anthropic API key:"))
		s.WriteString("\n")
		s.WriteString(dimStyle.Render("Get one at: https://console.anthropic.com/"))
	case "openai":
		s.WriteString(normalStyle.Render("Enter your OpenAI API key:"))
		s.WriteString("\n")
		s.WriteString(dimStyle.Render("Get one at: https://platform.openai.com/api-keys"))
	case "bedrock":
		s.WriteString(normalStyle.Render("Enter your AWS Access Key ID:"))
	}

	s.WriteString("\n\n")
	s.WriteString(inputStyle.Render(m.textInput.View()))
	s.WriteString("\n")

	if m.testing {
		s.WriteString("\n")
		s.WriteString(m.spinner.View())
		s.WriteString(" Testing connection...")
		s.WriteString("\n")
	} else if m.testResult != "" {
		s.WriteString("\n")
		if m.testSuccess {
			s.WriteString(successStyle.Render("  " + m.testResult))
		} else {
			s.WriteString(errorStyle.Render("  " + m.testResult))
		}
		s.WriteString("\n")
	}

	s.WriteString("\n")
	s.WriteString(dimStyle.Render("Press Enter to test and continue"))
	s.WriteString("\n")
	return s.String()
}

func (m Model) viewGitHubUsername() string {
	var s strings.Builder
	s.WriteString("\n")
	s.WriteString(titleStyle.Render("Step 4: GitHub Username"))
	s.WriteString("\n\n")
	s.WriteString(normalStyle.Render("This is used to identify your commits in git history."))
	s.WriteString("\n")
	s.WriteString(dimStyle.Render("(Matches commits with emails like username@users.noreply.github.com)"))
	s.WriteString("\n\n")
	s.WriteString(dimStyle.Render("GitHub username:"))
	s.WriteString("\n")
	s.WriteString(inputStyle.Render(m.textInput.View()))
	s.WriteString("\n\n")
	s.WriteString(dimStyle.Render("Press Enter to continue"))
	s.WriteString("\n")
	return s.String()
}

func (m Model) viewUserEmail() string {
	var s strings.Builder
	s.WriteString("\n")
	s.WriteString(titleStyle.Render("Step 4: Your Info"))
	s.WriteString("\n\n")
	if m.config.GitHubUsername != "" {
		s.WriteString(dimStyle.Render(fmt.Sprintf("GitHub: %s", m.config.GitHubUsername)))
		s.WriteString("\n\n")
	}
	s.WriteString(dimStyle.Render("Your email (optional, for additional git matching):"))
	s.WriteString("\n")
	s.WriteString(inputStyle.Render(m.textInput.View()))
	s.WriteString("\n\n")
	s.WriteString(dimStyle.Render("Press Enter to finish"))
	s.WriteString("\n")
	return s.String()
}

func (m Model) viewSuccess() string {
	var s strings.Builder
	s.WriteString("\n")
	s.WriteString(successStyle.Render("  Setup Complete!"))
	s.WriteString("\n\n")
	s.WriteString(normalStyle.Render("You're all set! Here's what to do next:"))
	s.WriteString("\n\n")

	s.WriteString(selectedStyle.Render("1. Ingest a repository:"))
	s.WriteString("\n")
	s.WriteString(dimStyle.Render("   cd ~/your-project && devlog ingest"))
	s.WriteString("\n\n")

	s.WriteString(selectedStyle.Render("2. Ask questions:"))
	s.WriteString("\n")
	s.WriteString(dimStyle.Render("   devlog ask \"What did I work on this week?\""))
	s.WriteString("\n\n")

	s.WriteString(selectedStyle.Render("3. Search code:"))
	s.WriteString("\n")
	s.WriteString(dimStyle.Render("   devlog search \"authentication\""))
	s.WriteString("\n\n")

	s.WriteString(selectedStyle.Render("4. Generate reports:"))
	s.WriteString("\n")
	s.WriteString(dimStyle.Render("   devlog worklog --days 7"))
	s.WriteString("\n\n")

	s.WriteString(dimStyle.Render("Press Enter or q to exit"))
	s.WriteString("\n")

	if m.err != nil {
		s.WriteString("\n")
		s.WriteString(errorStyle.Render(fmt.Sprintf("Error: %v", m.err)))
		s.WriteString("\n")
	}

	return s.String()
}

// RunOnboard runs the onboarding TUI and returns the resulting config
func RunOnboard() (*config.Config, error) {
	p := tea.NewProgram(NewModel(), tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}

	m := finalModel.(Model)
	if m.step != stepSuccess {
		return nil, fmt.Errorf("onboarding cancelled")
	}

	return m.config, nil
}
