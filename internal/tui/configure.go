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

// Configuration steps
type configStep int

const (
	configStepMenu configStep = iota
	configStepLLMProvider
	configStepLLMConfig
	configStepLLMModelSelection
	configStepTimezone
	configStepUserInfo
	configStepAPIKeys
	configStepReview
	configStepSaved
)

// ConfigModel for the configuration TUI
type ConfigModel struct {
	step           configStep
	config         *config.Config
	originalConfig *config.Config // Keep original for comparison
	selectedIdx    int
	textInput      textinput.Model
	spinner        spinner.Model
	testing        bool
	testResult     string
	testSuccess    bool
	err            error
	width          int
	height         int
	animationTick  int

	// Menu state
	menuOptions []menuOption
}

type menuOption struct {
	key         string
	title       string
	description string
}

// NewConfigModel creates a new configuration model
func NewConfigModel(cfg *config.Config) ConfigModel {
	ti := textinput.New()
	ti.Focus()
	ti.CharLimit = 128
	ti.Width = 50

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("86"))

	// Create a copy of the config to compare changes
	originalCfg := *cfg

	menuOpts := []menuOption{
		{"1", "LLM Provider", "Change language model provider"},
		{"2", "LLM Model", "Change language model"},
		{"3", "Timezone", "Change timezone for time tracking"},
		{"4", "User Information", "Update name, email, GitHub username"},
		{"5", "API Keys", "Update API keys"},
		{"6", "Review Settings", "View current configuration"},
		{"7", "Save & Exit", "Save changes and exit"},
		{"0", "Cancel", "Exit without saving"},
	}

	return ConfigModel{
		step:           configStepMenu,
		config:         cfg,
		originalConfig: &originalCfg,
		textInput:      ti,
		spinner:        s,
		menuOptions:    menuOpts,
	}
}

func (m ConfigModel) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		tickCmd(),
	)
}

func (m ConfigModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			if m.step == configStepMenu || m.step == configStepSaved {
				return m, tea.Quit
			}
		case "enter":
			return m.handleEnter()
		case "up", "k":
			if m.step == configStepMenu && m.selectedIdx > 0 {
				m.selectedIdx--
			} else if m.step == configStepLLMProvider && m.selectedIdx > 0 {
				m.selectedIdx--
			} else if m.step == configStepLLMModelSelection && m.selectedIdx > 0 {
				m.selectedIdx--
			} else if m.step == configStepTimezone && m.selectedIdx > 0 {
				m.selectedIdx--
			} else if m.step == configStepAPIKeys && m.selectedIdx > 0 {
				m.selectedIdx--
			}
		case "down", "j":
			if m.step == configStepMenu && m.selectedIdx < len(m.menuOptions)-1 {
				m.selectedIdx++
			} else if m.step == configStepLLMProvider && m.selectedIdx < len(getLLMProviders())-1 {
				m.selectedIdx++
			} else if m.step == configStepLLMModelSelection && m.selectedIdx < len(getModelOptions(constants.Provider(m.config.DefaultProvider)))-1 {
				m.selectedIdx++
			} else if m.step == configStepTimezone && m.selectedIdx < len(getTimezoneOptions())-1 {
				m.selectedIdx++
			} else if m.step == configStepAPIKeys && m.selectedIdx < 6 {
				m.selectedIdx++
			}
		case "esc":
			// Go back to menu from any step
			if m.step != configStepMenu && m.step != configStepSaved {
				m.step = configStepMenu
				m.selectedIdx = 0
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
			// After successful LLM config test, show model selection if available
			if m.step == configStepLLMConfig {
				if constants.ProviderHasModelSelection(constants.Provider(m.config.DefaultProvider)) {
					m.step = configStepLLMModelSelection
					m.selectedIdx = 0
					return m, nil
				}
			}
			m.step = configStepMenu
			m.selectedIdx = 0
			m.prepareStep()
		}

	case spinner.TickMsg:
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	// Update text input for certain steps
	if m.step == configStepLLMConfig || m.step == configStepLLMModelSelection ||
		m.step == configStepUserInfo || m.step == configStepAPIKeys {
		m.textInput, cmd = m.textInput.Update(msg)
	}

	return m, cmd
}

func (m ConfigModel) handleEnter() (tea.Model, tea.Cmd) {
	switch m.step {
	case configStepMenu:
		// Handle menu selection
		if m.selectedIdx >= len(m.menuOptions) {
			return m, nil
		}

		option := m.menuOptions[m.selectedIdx]
		switch option.key {
		case "1":
			m.step = configStepLLMProvider
			m.selectedIdx = 0
		case "2":
			// Standalone model selection — only if provider has models
			if constants.ProviderHasModelSelection(constants.Provider(m.config.DefaultProvider)) {
				m.step = configStepLLMModelSelection
				m.selectedIdx = 0
			}
		case "3":
			m.step = configStepTimezone
			m.selectedIdx = 0
		case "4":
			m.step = configStepUserInfo
			m.prepareStep()
		case "5":
			m.step = configStepAPIKeys
			m.selectedIdx = 0
			m.prepareStep()
		case "6":
			m.step = configStepReview
		case "7":
			// Save and exit
			return m.finishConfiguration()
		case "0":
			// Cancel
			return m, tea.Quit
		}
		return m, nil

	case configStepLLMProvider:
		providers := getLLMProviders()
		m.config.DefaultProvider = strings.ToLower(providers[m.selectedIdx].Name)
		m.step = configStepLLMConfig
		m.prepareStep()
		return m, nil

	case configStepLLMConfig:
		value := strings.TrimSpace(m.textInput.Value())
		switch constants.Provider(m.config.DefaultProvider) {
		case constants.ProviderOllama:
			if value != "" {
				m.config.OllamaBaseURL = value
			}
		case constants.ProviderAnthropic:
			if value != "" {
				m.config.AnthropicAPIKey = value
			}
		case constants.ProviderOpenAI:
			if value != "" {
				m.config.OpenAIAPIKey = value
			}
		case constants.ProviderOpenRouter:
			if value != "" {
				m.config.OpenRouterAPIKey = value
			}
		case constants.ProviderGemini:
			if value != "" {
				m.config.GeminiAPIKey = value
			}
		case constants.ProviderBedrock:
			if value != "" {
				m.config.AWSAccessKeyID = value
			}
		}

		// Test the configuration if API key was provided
		if value != "" && constants.Provider(m.config.DefaultProvider) != constants.ProviderOllama {
			m.testing = true
			m.testResult = ""
			return m, tea.Batch(
				m.spinner.Tick,
				testProvider(m.config.DefaultProvider, value, m.config.OllamaBaseURL),
			)
		}

		// Show model selection if provider has multiple options
		if constants.ProviderHasModelSelection(constants.Provider(m.config.DefaultProvider)) {
			m.step = configStepLLMModelSelection
			m.selectedIdx = 0
			return m, nil
		}

		m.step = configStepMenu
		m.selectedIdx = 0
		return m, nil

	case configStepLLMModelSelection:
		models := getModelOptions(constants.Provider(m.config.DefaultProvider))
		if m.selectedIdx < len(models) {
			m.config.DefaultModel = models[m.selectedIdx].Model
		}
		m.step = configStepMenu
		m.selectedIdx = 0
		return m, nil

	case configStepTimezone:
		timezones := getTimezoneOptions()
		if m.selectedIdx < len(timezones) {
			selectedTZ := timezones[m.selectedIdx].IANAName
			if selectedTZ != "" {
				// Update timezone for active profile
				if m.config.Profiles != nil && m.config.ActiveProfile != "" {
					if profile := m.config.Profiles[m.config.ActiveProfile]; profile != nil {
						profile.Timezone = selectedTZ
					}
				}
			}
		}
		m.step = configStepMenu
		m.selectedIdx = 0
		return m, nil

	case configStepUserInfo:
		// This step uses multiple text inputs, handled differently
		m.step = configStepMenu
		m.selectedIdx = 0
		return m, nil

	case configStepAPIKeys:
		value := strings.TrimSpace(m.textInput.Value())
		if value != "" {
			switch m.selectedIdx {
			case 0:
				m.config.AnthropicAPIKey = value
			case 1:
				m.config.OpenAIAPIKey = value
			case 2:
				m.config.OpenRouterAPIKey = value
			case 3:
				m.config.GeminiAPIKey = value
			case 4:
				m.config.AWSAccessKeyID = value
			case 5:
				m.config.AWSSecretAccessKey = value
			}
		}
		m.step = configStepMenu
		m.selectedIdx = 0
		return m, nil

	case configStepReview:
		m.step = configStepMenu
		m.selectedIdx = 0
		return m, nil

	case configStepSaved:
		return m, tea.Quit
	}

	return m, nil
}

func (m *ConfigModel) prepareStep() {
	m.textInput.Reset()
	m.testResult = ""
	m.testSuccess = false

	switch m.step {
	case configStepLLMConfig:
		provider := constants.Provider(m.config.DefaultProvider)
		setupInfo := constants.GetProviderSetupInfo(provider)
		m.textInput.Placeholder = setupInfo.Placeholder
		if setupInfo.NeedsAPIKey {
			m.textInput.EchoMode = textinput.EchoPassword
			// Show masked existing key if available
			existingKey := m.getExistingAPIKey(provider)
			if existingKey != "" {
				m.textInput.Placeholder = maskKey(existingKey)
			}
		}
	case configStepUserInfo:
		m.textInput.EchoMode = textinput.EchoNormal
		m.textInput.Placeholder = ""
	case configStepAPIKeys:
		m.textInput.EchoMode = textinput.EchoPassword
		m.textInput.Placeholder = "Enter new value or press Enter to keep current"
	}
}

// getExistingAPIKey returns the current API key for a provider from config
func (m ConfigModel) getExistingAPIKey(provider constants.Provider) string {
	switch provider {
	case constants.ProviderAnthropic:
		return m.config.AnthropicAPIKey
	case constants.ProviderOpenAI:
		return m.config.OpenAIAPIKey
	case constants.ProviderOpenRouter:
		return m.config.OpenRouterAPIKey
	case constants.ProviderGemini:
		return m.config.GeminiAPIKey
	case constants.ProviderBedrock:
		return m.config.AWSAccessKeyID
	default:
		return ""
	}
}

func (m ConfigModel) finishConfiguration() (tea.Model, tea.Cmd) {
	// Save config
	if err := m.config.Save(); err != nil {
		m.err = err
		return m, nil
	}

	m.step = configStepSaved
	return m, nil
}

func (m ConfigModel) View() string {
	var s strings.Builder

	switch m.step {
	case configStepMenu:
		s.WriteString(m.viewMenu())
	case configStepLLMProvider:
		s.WriteString(m.viewLLMProvider())
	case configStepLLMConfig:
		s.WriteString(m.viewLLMConfig())
	case configStepLLMModelSelection:
		s.WriteString(m.viewLLMModelSelection())
	case configStepTimezone:
		s.WriteString(m.viewTimezone())
	case configStepUserInfo:
		s.WriteString(m.viewUserInfo())
	case configStepAPIKeys:
		s.WriteString(m.viewAPIKeys())
	case configStepReview:
		s.WriteString(m.viewReview())
	case configStepSaved:
		s.WriteString(m.viewSaved())
	}

	return s.String()
}

// ── Configure-specific views ───────────────────────────────────────────────

func (m ConfigModel) viewMenu() string {
	var s strings.Builder
	s.WriteString("\n")
	s.WriteString(titleStyle.Render("DevLog Configuration"))
	s.WriteString("\n\n")
	s.WriteString(normalStyle.Render("Configure your DevLog settings"))
	s.WriteString("\n\n")

	// Show current settings summary
	s.WriteString(dimStyle.Render("Current Settings:"))
	s.WriteString("\n")
	s.WriteString(normalStyle.Render(fmt.Sprintf("  LLM: %s", m.config.DefaultProvider)))
	s.WriteString("\n")
	if m.config.DefaultModel != "" {
		s.WriteString(dimStyle.Render(fmt.Sprintf("  Model: %s", m.config.DefaultModel)))
		s.WriteString("\n")
	}
	s.WriteString("\n")

	// Menu options
	for i, opt := range m.menuOptions {
		cursor := "  "
		style := normalStyle
		if i == m.selectedIdx {
			cursor = "> "
			style = selectedStyle
		}
		s.WriteString(style.Render(fmt.Sprintf("%s[%s] %s", cursor, opt.key, opt.title)))
		s.WriteString("\n")
		s.WriteString(dimStyle.Render(fmt.Sprintf("      %s", opt.description)))
		s.WriteString("\n")
	}

	s.WriteString("\n")
	s.WriteString(dimStyle.Render("↑/↓: navigate • enter: select • esc: back • q: quit"))
	s.WriteString("\n")
	return s.String()
}

func (m ConfigModel) viewLLMProvider() string {
	header := dimStyle.Render(fmt.Sprintf("Current: %s", m.config.DefaultProvider))
	return RenderSelectList(
		"Configure LLM Provider",
		header,
		LLMProviderItems(),
		m.selectedIdx,
		false, 0,
		"↑/↓: navigate • enter: select • esc: back",
	)
}

func (m ConfigModel) viewLLMConfig() string {
	providerName := titleCase(m.config.DefaultProvider)
	setupInfo := constants.GetProviderSetupInfo(constants.Provider(m.config.DefaultProvider))

	var body string
	if setupInfo.NeedsAPIKey {
		body = normalStyle.Render(fmt.Sprintf("%s API key (leave empty to keep current):", providerName))
		if setupInfo.APIKeyURL != "" {
			body += "\n" + dimStyle.Render(fmt.Sprintf("Get one at: %s", setupInfo.APIKeyURL))
		}
	} else {
		body = normalStyle.Render(fmt.Sprintf("%s base URL (leave empty to keep current):", providerName))
	}
	body += "\n" // extra spacing before input

	test := &TestState{
		Testing:     m.testing,
		Spinner:     m.spinner,
		TestResult:  m.testResult,
		TestSuccess: m.testSuccess,
	}

	return RenderTextInput(
		fmt.Sprintf("Configure %s", providerName),
		body, m.textInput, test,
		"Press Enter to save • Esc to cancel",
	)
}

func (m ConfigModel) viewLLMModelSelection() string {
	header := normalStyle.Render("Choose a model for your LLM:")
	if m.config.DefaultModel != "" {
		header = dimStyle.Render(fmt.Sprintf("Current: %s", m.config.DefaultModel)) + "\n\n" +
			normalStyle.Render("Choose a model for your LLM:")
	}

	return RenderSelectList(
		fmt.Sprintf("Select %s Model", titleCase(m.config.DefaultProvider)),
		header,
		ModelItems(constants.Provider(m.config.DefaultProvider)),
		m.selectedIdx,
		true, 45,
		"↑/↓ or k/j to navigate, Enter to select",
	)
}

func (m ConfigModel) viewTimezone() string {
	var currentTZ string
	if m.config.Profiles != nil && m.config.ActiveProfile != "" {
		if profile := m.config.Profiles[m.config.ActiveProfile]; profile != nil {
			currentTZ = profile.Timezone
		}
	}
	if currentTZ == "" {
		currentTZ = "UTC"
	}

	header := dimStyle.Render(fmt.Sprintf("Current: %s", currentTZ)) + "\n\n" +
		normalStyle.Render("Select a new timezone:")

	return RenderSelectList(
		"Configure Timezone",
		header,
		TimezoneItems(),
		m.selectedIdx,
		false, 0,
		"↑/↓: navigate • enter: select • esc: back",
	)
}

func (m ConfigModel) viewUserInfo() string {
	var s strings.Builder
	s.WriteString("\n")
	s.WriteString(titleStyle.Render("User Information"))
	s.WriteString("\n\n")
	s.WriteString(normalStyle.Render("This feature requires text input fields."))
	s.WriteString("\n")
	s.WriteString(normalStyle.Render("Use the CLI with --legacy flag for user info editing."))
	s.WriteString("\n\n")

	// Show current values
	if m.config.GitHubUsername != "" {
		s.WriteString(normalStyle.Render(fmt.Sprintf("  GitHub: %s", m.config.GitHubUsername)))
		s.WriteString("\n")
	}
	if m.config.UserEmail != "" {
		s.WriteString(normalStyle.Render(fmt.Sprintf("  Email: %s", m.config.UserEmail)))
		s.WriteString("\n")
	}
	if m.config.UserName != "" {
		s.WriteString(normalStyle.Render(fmt.Sprintf("  Name: %s", m.config.UserName)))
		s.WriteString("\n")
	}

	s.WriteString("\n")
	s.WriteString(dimStyle.Render("Press Enter to return • Esc to go back"))
	s.WriteString("\n")
	return s.String()
}

func (m ConfigModel) viewAPIKeys() string {
	var s strings.Builder
	s.WriteString("\n")
	s.WriteString(titleStyle.Render("Configure API Keys"))
	s.WriteString("\n\n")
	s.WriteString(normalStyle.Render("Select a provider to update its API key:"))
	s.WriteString("\n\n")

	apiKeyOptions := []struct {
		name    string
		current string
	}{
		{"Anthropic", m.config.AnthropicAPIKey},
		{"OpenAI", m.config.OpenAIAPIKey},
		{"OpenRouter", m.config.OpenRouterAPIKey},
		{"Gemini", m.config.GeminiAPIKey},
		{"AWS Access Key", m.config.AWSAccessKeyID},
		{"AWS Secret Key", m.config.AWSSecretAccessKey},
	}

	for i, opt := range apiKeyOptions {
		cursor := "  "
		style := normalStyle
		if i == m.selectedIdx {
			cursor = "> "
			style = selectedStyle
		}

		status := dimStyle.Render("(not set)")
		if opt.current != "" {
			status = dimStyle.Render(fmt.Sprintf("(%s)", maskKey(opt.current)))
		}

		s.WriteString(style.Render(fmt.Sprintf("%s%s %s", cursor, opt.name, status)))
		s.WriteString("\n")
	}

	s.WriteString("\n")
	s.WriteString(dimStyle.Render("↑/↓: navigate • enter: edit • esc: back"))
	s.WriteString("\n")
	return s.String()
}

func (m ConfigModel) viewReview() string {
	var s strings.Builder
	s.WriteString("\n")
	s.WriteString(titleStyle.Render("Configuration Review"))
	s.WriteString("\n\n")

	// LLM Configuration
	s.WriteString(successStyle.Render("LLM Provider:"))
	s.WriteString("\n")
	s.WriteString(normalStyle.Render(fmt.Sprintf("  %s", m.config.DefaultProvider)))
	s.WriteString("\n")
	if m.config.DefaultModel != "" {
		s.WriteString(dimStyle.Render(fmt.Sprintf("  Model: %s", m.config.DefaultModel)))
		s.WriteString("\n")
	}
	s.WriteString("\n")

	// User Information
	if m.config.GitHubUsername != "" || m.config.UserEmail != "" || m.config.UserName != "" {
		s.WriteString(successStyle.Render("User Information:"))
		s.WriteString("\n")
		if m.config.GitHubUsername != "" {
			s.WriteString(normalStyle.Render(fmt.Sprintf("  GitHub: %s", m.config.GitHubUsername)))
			s.WriteString("\n")
		}
		if m.config.UserEmail != "" {
			s.WriteString(normalStyle.Render(fmt.Sprintf("  Email: %s", m.config.UserEmail)))
			s.WriteString("\n")
		}
		if m.config.UserName != "" {
			s.WriteString(normalStyle.Render(fmt.Sprintf("  Name: %s", m.config.UserName)))
			s.WriteString("\n")
		}
		s.WriteString("\n")
	}

	// API Keys (masked)
	hasAPIKeys := m.config.AnthropicAPIKey != "" || m.config.OpenAIAPIKey != "" ||
		m.config.OpenRouterAPIKey != "" || m.config.GeminiAPIKey != "" || m.config.AWSAccessKeyID != ""

	if hasAPIKeys {
		s.WriteString(successStyle.Render("API Keys:"))
		s.WriteString("\n")
		if m.config.AnthropicAPIKey != "" {
			s.WriteString(dimStyle.Render(fmt.Sprintf("  Anthropic: %s", maskKey(m.config.AnthropicAPIKey))))
			s.WriteString("\n")
		}
		if m.config.OpenAIAPIKey != "" {
			s.WriteString(dimStyle.Render(fmt.Sprintf("  OpenAI: %s", maskKey(m.config.OpenAIAPIKey))))
			s.WriteString("\n")
		}
		if m.config.OpenRouterAPIKey != "" {
			s.WriteString(dimStyle.Render(fmt.Sprintf("  OpenRouter: %s", maskKey(m.config.OpenRouterAPIKey))))
			s.WriteString("\n")
		}
		if m.config.GeminiAPIKey != "" {
			s.WriteString(dimStyle.Render(fmt.Sprintf("  Gemini: %s", maskKey(m.config.GeminiAPIKey))))
			s.WriteString("\n")
		}
		if m.config.AWSAccessKeyID != "" {
			s.WriteString(dimStyle.Render(fmt.Sprintf("  AWS: %s", maskKey(m.config.AWSAccessKeyID))))
			s.WriteString("\n")
		}
		s.WriteString("\n")
	}

	s.WriteString(dimStyle.Render("Press Enter to return • Esc to go back"))
	s.WriteString("\n")
	return s.String()
}

func (m ConfigModel) viewSaved() string {
	var s strings.Builder
	s.WriteString("\n")
	s.WriteString(successStyle.Render("  Configuration Saved!"))
	s.WriteString("\n\n")
	s.WriteString(normalStyle.Render("Your settings have been updated successfully."))
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

// RunConfigure runs the configuration TUI and returns the resulting config
func RunConfigure(cfg *config.Config) (*config.Config, error) {
	p := tea.NewProgram(NewConfigModel(cfg))
	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}

	m := finalModel.(ConfigModel)
	if m.step != configStepSaved {
		return nil, fmt.Errorf("configuration canceled")
	}

	return m.config, nil
}
