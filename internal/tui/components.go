package tui

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ishaan812/devlog/internal/constants"
)

// ── Shared types ───────────────────────────────────────────────────────────

// SelectItem represents a selectable item in a list.
type SelectItem struct {
	Label       string
	Description string
}

// TestState holds state for connection-testing feedback in text input views.
type TestState struct {
	Testing     bool
	Spinner     spinner.Model
	TestResult  string
	TestSuccess bool
}

// ── Messages ───────────────────────────────────────────────────────────────

type tickMsg time.Time

type testResultMsg struct {
	success bool
	message string
}

// ── Commands ───────────────────────────────────────────────────────────────

func tickCmd() tea.Cmd {
	return tea.Tick(time.Millisecond*100, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// testProvider tests a provider connection / API key format.
func testProvider(provider, apiKey, baseURL string) tea.Cmd {
	return func() tea.Msg {
		p := constants.Provider(provider)
		setupInfo := constants.GetProviderSetupInfo(p)

		// Ollama: connection test
		if p == constants.ProviderOllama {
			url := baseURL
			if url == "" {
				url = constants.GetDefaultBaseURL(constants.ProviderOllama)
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
		}

		// API-key providers
		if !setupInfo.NeedsAPIKey {
			return testResultMsg{true, "Configuration saved!"}
		}
		if apiKey == "" {
			return testResultMsg{false, "API key is required"}
		}
		if setupInfo.APIKeyPrefix != "" && !strings.HasPrefix(apiKey, setupInfo.APIKeyPrefix) {
			return testResultMsg{false, fmt.Sprintf("Invalid API key format (should start with %s)", setupInfo.APIKeyPrefix)}
		}
		return testResultMsg{true, "API key format valid!"}
	}
}

// ── Helpers ────────────────────────────────────────────────────────────────

// maskKey masks an API key for display (first 4 + last 4 chars).
func maskKey(key string) string {
	if len(key) <= 8 {
		return "***"
	}
	return key[:4] + "..." + key[len(key)-4:]
}

// titleCase capitalises the first letter of s.
func titleCase(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// ── Data providers ─────────────────────────────────────────────────────────

// getLLMProviders returns all providers that support LLM.
func getLLMProviders() []constants.ProviderInfo {
	var llmProviders []constants.ProviderInfo
	for _, p := range constants.AllProviders {
		if p.SupportsLLM {
			llmProviders = append(llmProviders, p)
		}
	}
	return llmProviders
}

// getEmbeddingProviders returns all embedding provider options.
func getEmbeddingProviders() []constants.EmbeddingProviderInfo {
	return constants.AllEmbeddingProviders
}

// getModelOptions returns available LLM models for a provider.
func getModelOptions(provider constants.Provider) []constants.ModelOption {
	return constants.GetLLMModels(provider)
}

// ── Item converters (providers/models → SelectItems) ───────────────────────

// LLMProviderItems converts LLM provider info to SelectItems.
func LLMProviderItems() []SelectItem {
	providers := getLLMProviders()
	items := make([]SelectItem, len(providers))
	for i, p := range providers {
		items[i] = SelectItem{Label: p.Name, Description: p.Description}
	}
	return items
}

// ModelItems converts model options for a provider to SelectItems.
func ModelItems(provider constants.Provider) []SelectItem {
	models := getModelOptions(provider)
	items := make([]SelectItem, len(models))
	for i, m := range models {
		items[i] = SelectItem{Label: m.Model, Description: m.Description}
	}
	return items
}

// EmbeddingProviderItems converts embedding provider info to SelectItems.
func EmbeddingProviderItems() []SelectItem {
	providers := getEmbeddingProviders()
	items := make([]SelectItem, len(providers))
	for i, p := range providers {
		items[i] = SelectItem{Label: p.Name, Description: p.Description}
	}
	return items
}

// ── View renderers ─────────────────────────────────────────────────────────

// RenderSelectList renders a navigable selection list.
//
//   - title:      rendered with titleStyle
//   - header:     pre-rendered text between title and list (pass "" to omit)
//   - items:      the list of selectable items
//   - selectedIdx: cursor position
//   - inline:     if true, label + description on same line; if false, description below
//   - labelWidth: left-pad width for inline layout (ignored when inline=false)
//   - helpText:   footer hint
func RenderSelectList(title, header string, items []SelectItem, selectedIdx int, inline bool, labelWidth int, helpText string) string {
	var s strings.Builder

	s.WriteString("\n")
	s.WriteString(titleStyle.Render(title))
	s.WriteString("\n\n")

	if header != "" {
		s.WriteString(header)
		s.WriteString("\n\n")
	}

	for i, item := range items {
		cursor := "  "
		style := normalStyle
		if i == selectedIdx {
			cursor = "> "
			style = selectedStyle
		}

		if inline {
			s.WriteString(style.Render(fmt.Sprintf("%s%-*s", cursor, labelWidth, item.Label)))
			s.WriteString(dimStyle.Render(fmt.Sprintf(" %s", item.Description)))
			s.WriteString("\n")
		} else {
			s.WriteString(style.Render(fmt.Sprintf("%s%s", cursor, item.Label)))
			s.WriteString("\n")
			s.WriteString(dimStyle.Render(fmt.Sprintf("    %s", item.Description)))
			s.WriteString("\n")
		}
	}

	s.WriteString("\n")
	s.WriteString(dimStyle.Render(helpText))
	s.WriteString("\n")
	return s.String()
}

// RenderTextInput renders a text input step with optional test feedback.
//
//   - title:    rendered with titleStyle
//   - body:     pre-rendered content shown between title and input (pass "" to omit)
//   - ti:       the text input model
//   - test:     optional test state (pass nil to omit test feedback)
//   - helpText: footer hint
func RenderTextInput(title, body string, ti textinput.Model, test *TestState, helpText string) string {
	var s strings.Builder

	s.WriteString("\n")
	s.WriteString(titleStyle.Render(title))
	s.WriteString("\n\n")

	if body != "" {
		s.WriteString(body)
	}

	s.WriteString("\n")
	s.WriteString(inputStyle.Render(ti.View()))
	s.WriteString("\n")

	if test != nil {
		if test.Testing {
			s.WriteString("\n")
			s.WriteString(test.Spinner.View())
			s.WriteString(" Testing connection...")
			s.WriteString("\n")
		} else if test.TestResult != "" {
			s.WriteString("\n")
			if test.TestSuccess {
				s.WriteString(successStyle.Render("  " + test.TestResult))
			} else {
				s.WriteString(errorStyle.Render("  " + test.TestResult))
			}
			s.WriteString("\n")
		}
	}

	s.WriteString("\n")
	s.WriteString(dimStyle.Render(helpText))
	s.WriteString("\n")
	return s.String()
}
