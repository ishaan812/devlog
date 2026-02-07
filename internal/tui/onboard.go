package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ishaan812/devlog/internal/config"
	"github.com/ishaan812/devlog/internal/constants"
)

// Steps in the onboarding flow
type step int

const (
	stepWelcome          step = iota
	stepExistingProfiles      // show existing profiles
	stepProfileName
	stepProfileDesc
	stepProvider
	stepProviderConfig
	stepModelSelection    // model selection
	stepEmbeddingProvider // embedding provider selection
	stepEmbeddingConfig   // embedding model configuration
	stepGitHubUsername
	stepUserEmail
	stepSuccess
)

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

	// Existing profiles
	existingProfiles []string
	useExisting      bool
}

// NewModel creates a new onboarding model
func NewModel() Model {
	ti := textinput.New()
	ti.Focus()
	ti.CharLimit = 700
	ti.Width = 40

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("86"))

	// Load existing config to check for profiles
	existingCfg, loadErr := config.Load()
	var existingProfiles []string
	var cfg *config.Config

	if loadErr != nil {
		// Config doesn't exist yet or is unreadable — start fresh
		cfg = &config.Config{}
	} else {
		cfg = existingCfg
		if existingCfg.Profiles != nil {
			for name := range existingCfg.Profiles {
				existingProfiles = append(existingProfiles, name)
			}
		}
	}

	return Model{
		step:             stepWelcome,
		config:           cfg,
		textInput:        ti,
		spinner:          s,
		existingProfiles: existingProfiles,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		tickCmd(),
	)
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
			} else if m.step == stepModelSelection && m.selectedIdx > 0 {
				m.selectedIdx--
			} else if m.step == stepEmbeddingProvider && m.selectedIdx > 0 {
				m.selectedIdx--
			} else if m.step == stepExistingProfiles && m.selectedIdx > 0 {
				m.selectedIdx--
			}
		case "down", "j":
			if m.step == stepProvider && m.selectedIdx < len(getLLMProviders())-1 {
				m.selectedIdx++
			} else if m.step == stepModelSelection && m.selectedIdx < len(getModelOptions(constants.Provider(m.config.DefaultProvider)))-1 {
				m.selectedIdx++
			} else if m.step == stepEmbeddingProvider && m.selectedIdx < len(getEmbeddingProviders())-1 {
				m.selectedIdx++
			} else if m.step == stepExistingProfiles && m.selectedIdx < len(m.existingProfiles) {
				// +1 for "create new" option
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
		m.step == stepProviderConfig || m.step == stepModelSelection ||
		m.step == stepEmbeddingConfig || m.step == stepGitHubUsername ||
		m.step == stepUserEmail {
		m.textInput, cmd = m.textInput.Update(msg)
	}

	return m, cmd
}

func (m Model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.step {
	case stepWelcome:
		// Check if there are existing profiles
		if len(m.existingProfiles) > 0 {
			m.step = stepExistingProfiles
			m.selectedIdx = 0
		} else {
			m.step = stepProfileName
			m.prepareStep()
		}
		return m, nil

	case stepExistingProfiles:
		// selectedIdx: 0 = create new, 1+ = existing profiles
		if m.selectedIdx == 0 {
			// Create new profile
			m.useExisting = false
			m.step = stepProfileName
			m.prepareStep()
		} else {
			// Use existing profile
			m.useExisting = true
			m.profileName = m.existingProfiles[m.selectedIdx-1]
			// Load existing config
			existingCfg, loadErr := config.Load()
			if loadErr != nil {
				m.err = fmt.Errorf("failed to load existing config: %w", loadErr)
				return m, nil
			}
			m.config = existingCfg
			m.config.ActiveProfile = m.profileName
			m.step = stepProvider
		}
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
		providers := getLLMProviders()
		m.config.DefaultProvider = strings.ToLower(providers[m.selectedIdx].Name)
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
		case "openrouter":
			m.config.OpenRouterAPIKey = value
		case "gemini":
			m.config.GeminiAPIKey = value
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

	case stepModelSelection:
		models := getModelOptions(constants.Provider(m.config.DefaultProvider))
		if m.selectedIdx < len(models) {
			m.config.DefaultModel = models[m.selectedIdx].Model
		}
		m.step = stepEmbeddingProvider
		m.selectedIdx = 0
		return m, nil

	case stepEmbeddingProvider:
		embProviders := getEmbeddingProviders()
		selectedProvider := embProviders[m.selectedIdx].Provider
		if selectedProvider == "" {
			// "Same as LLM provider"
			m.config.EmbeddingProvider = m.config.DefaultProvider
		} else {
			m.config.EmbeddingProvider = string(selectedProvider)
		}
		m.step = stepEmbeddingConfig
		m.prepareStep()
		return m, nil

	case stepEmbeddingConfig:
		value := strings.TrimSpace(m.textInput.Value())
		if value != "" {
			m.config.DefaultEmbedModel = value
		} else {
			embProvider := constants.Provider(m.config.EmbeddingProvider)
			if !constants.ProviderSupportsEmbeddings(embProvider) {
				m.err = fmt.Errorf("%s doesn't support embeddings; please go back and select a dedicated embedding provider", m.config.EmbeddingProvider)
				return m, nil
			}
			defaultModel := constants.GetDefaultEmbeddingModel(embProvider)
			if defaultModel == "" {
				m.err = fmt.Errorf("no default embedding model for provider %s; please specify a model", m.config.EmbeddingProvider)
				return m, nil
			}
			m.config.DefaultEmbedModel = defaultModel
		}
		m.step = stepGitHubUsername
		m.prepareStep()
		return m, nil

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
	if m.step == stepProviderConfig {
		// Show model selection if provider has multiple model options
		if constants.ProviderHasModelSelection(constants.Provider(m.config.DefaultProvider)) {
			m.step = stepModelSelection
			m.selectedIdx = 0
		} else {
			m.step = stepEmbeddingProvider
			m.selectedIdx = 0
		}
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
		setupInfo := constants.GetProviderSetupInfo(constants.Provider(m.config.DefaultProvider))
		m.textInput.Placeholder = setupInfo.Placeholder
		if setupInfo.NeedsAPIKey {
			m.textInput.EchoMode = textinput.EchoPassword
		}
	case stepEmbeddingConfig:
		m.textInput.EchoMode = textinput.EchoNormal
		m.textInput.Placeholder = constants.GetDefaultEmbeddingModel(constants.Provider(m.config.EmbeddingProvider))
	case stepGitHubUsername:
		m.textInput.Placeholder = "your-github-username"
		m.textInput.EchoMode = textinput.EchoNormal
	case stepUserEmail:
		m.textInput.Placeholder = "you@example.com (optional)"
	}
}

func (m Model) finishOnboarding() (tea.Model, tea.Cmd) {
	// Only create profile if not using existing
	if !m.useExisting {
		if err := m.config.CreateProfile(m.profileName, m.profileDesc); err != nil {
			// Profile might already exist, that's ok
			if !strings.Contains(err.Error(), "already exists") {
				m.err = err
				return m, nil
			}
		}
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
	case stepExistingProfiles:
		s.WriteString(m.viewExistingProfiles())
	case stepProfileName:
		s.WriteString(m.viewProfileName())
	case stepProfileDesc:
		s.WriteString(m.viewProfileDesc())
	case stepProvider:
		s.WriteString(m.viewProvider())
	case stepProviderConfig:
		s.WriteString(m.viewProviderConfig())
	case stepModelSelection:
		s.WriteString(m.viewModelSelection())
	case stepEmbeddingProvider:
		s.WriteString(m.viewEmbeddingProvider())
	case stepEmbeddingConfig:
		s.WriteString(m.viewEmbeddingConfig())
	case stepGitHubUsername:
		s.WriteString(m.viewGitHubUsername())
	case stepUserEmail:
		s.WriteString(m.viewUserEmail())
	case stepSuccess:
		s.WriteString(m.viewSuccess())
	}

	return s.String()
}

// ── Onboard-specific views ─────────────────────────────────────────────────

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

func (m Model) viewExistingProfiles() string {
	var s strings.Builder
	s.WriteString("\n")
	s.WriteString(titleStyle.Render("Existing Profiles Found"))
	s.WriteString("\n\n")
	s.WriteString(normalStyle.Render("You have existing profiles. Select one or create a new one."))
	s.WriteString("\n\n")

	// Create new option
	cursor := "  "
	if m.selectedIdx == 0 {
		cursor = "> "
		s.WriteString(selectedStyle.Render(fmt.Sprintf("%s+ Create new profile", cursor)))
	} else {
		s.WriteString(normalStyle.Render(fmt.Sprintf("%s+ Create new profile", cursor)))
	}
	s.WriteString("\n\n")

	s.WriteString(dimStyle.Render("  Existing profiles:"))
	s.WriteString("\n")

	// List existing profiles
	for i, name := range m.existingProfiles {
		cursor = "  "
		if m.selectedIdx == i+1 {
			cursor = "> "
			s.WriteString(selectedStyle.Render(fmt.Sprintf("%s%s", cursor, name)))
		} else {
			s.WriteString(normalStyle.Render(fmt.Sprintf("%s%s", cursor, name)))
		}
		s.WriteString("\n")
	}

	s.WriteString("\n")
	s.WriteString(dimStyle.Render("↑/↓: navigate • enter: select"))
	s.WriteString("\n")
	return s.String()
}

func (m Model) viewProfileName() string {
	body := normalStyle.Render("Profiles keep your data organized. You might have") + "\n" +
		normalStyle.Render("separate profiles for personal and work projects.") + "\n\n" +
		dimStyle.Render("Profile name:")
	return RenderTextInput("Step 1: Create a Profile", body, m.textInput, nil, "Press Enter to continue, Esc to go back")
}

func (m Model) viewProfileDesc() string {
	body := dimStyle.Render(fmt.Sprintf("Profile: %s", m.profileName)) + "\n\n" +
		dimStyle.Render("Description (optional):")
	return RenderTextInput("Step 1: Create a Profile", body, m.textInput, nil, "Press Enter to continue, Esc to go back")
}

func (m Model) viewProvider() string {
	return RenderSelectList(
		"Step 2: Choose LLM Provider",
		normalStyle.Render("DevLog uses an LLM to generate summaries and answer questions."),
		LLMProviderItems(),
		m.selectedIdx,
		false, 0,
		"Use arrow keys to select, Enter to confirm",
	)
}

func (m Model) viewProviderConfig() string {
	providerName := titleCase(m.config.DefaultProvider)
	setupInfo := constants.GetProviderSetupInfo(constants.Provider(m.config.DefaultProvider))

	var body string
	if setupInfo.NeedsAPIKey {
		body = normalStyle.Render(fmt.Sprintf("Enter your %s API key:", providerName))
		if setupInfo.APIKeyURL != "" {
			body += "\n" + dimStyle.Render(fmt.Sprintf("Get one at: %s", setupInfo.APIKeyURL))
		}
	} else {
		body = normalStyle.Render(fmt.Sprintf("Enter %s base URL (leave empty for default):", providerName)) +
			"\n" + dimStyle.Render(setupInfo.SetupHint)
	}
	body += "\n" // extra spacing before input

	test := &TestState{
		Testing:     m.testing,
		Spinner:     m.spinner,
		TestResult:  m.testResult,
		TestSuccess: m.testSuccess,
	}

	return RenderTextInput(
		fmt.Sprintf("Step 3: Configure %s", providerName),
		body, m.textInput, test,
		"Press Enter to test and continue",
	)
}

func (m Model) viewModelSelection() string {
	return RenderSelectList(
		fmt.Sprintf("Step 3b: Select %s Model", titleCase(m.config.DefaultProvider)),
		normalStyle.Render("Choose a model for your LLM:"),
		ModelItems(constants.Provider(m.config.DefaultProvider)),
		m.selectedIdx,
		true, 45,
		"↑/↓ or k/j to navigate, Enter to select",
	)
}

func (m Model) viewEmbeddingProvider() string {
	return RenderSelectList(
		"Step 4: Choose Embedding Provider",
		normalStyle.Render("Embeddings are used for semantic search and similarity."),
		EmbeddingProviderItems(),
		m.selectedIdx,
		false, 0,
		"Use arrow keys to select, Enter to confirm",
	)
}

func (m Model) viewEmbeddingConfig() string {
	body := dimStyle.Render(fmt.Sprintf("Provider: %s", m.config.EmbeddingProvider)) + "\n\n" +
		normalStyle.Render("Enter embedding model (leave empty for default):")
	return RenderTextInput("Step 4: Configure Embeddings", body, m.textInput, nil, "Press Enter to continue")
}

func (m Model) viewGitHubUsername() string {
	body := normalStyle.Render("This is used to identify your commits in git history.") + "\n" +
		dimStyle.Render("(Matches commits with emails like username@users.noreply.github.com)") + "\n\n" +
		dimStyle.Render("GitHub username:")
	return RenderTextInput("Step 5: GitHub Username", body, m.textInput, nil, "Press Enter to continue")
}

func (m Model) viewUserEmail() string {
	var bodyParts []string
	if m.config.GitHubUsername != "" {
		bodyParts = append(bodyParts, dimStyle.Render(fmt.Sprintf("GitHub: %s", m.config.GitHubUsername)))
	}
	bodyParts = append(bodyParts, dimStyle.Render("Your email (optional, for additional git matching):"))
	body := strings.Join(bodyParts, "\n\n")
	return RenderTextInput("Step 5: Your Info", body, m.textInput, nil, "Press Enter to finish")
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
	// Don't use alternate screen - stay in same terminal
	p := tea.NewProgram(NewModel())
	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}

	m := finalModel.(Model)
	if m.step != stepSuccess {
		return nil, fmt.Errorf("onboarding canceled")
	}

	return m.config, nil
}
