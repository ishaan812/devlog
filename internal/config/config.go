package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const DefaultConfigFileName = "config.json"

type RepoBranchSelection struct {
	MainBranch       string   `json:"main_branch"`
	SelectedBranches []string `json:"selected_branches"`
}

// IndexFoldersConfig stores which folders to index for a repo (for repos with many files).
type IndexFoldersConfig struct {
	Folders []string `json:"folders"`
}

type ObsidianVaultConfig struct {
	VaultPath  string `json:"vault_path"`
	RootFolder string `json:"root_folder,omitempty"`
}

type Profile struct {
	Name             string                          `json:"name"`
	Description      string                          `json:"description,omitempty"`
	CreatedAt        string                          `json:"created_at"`
	Timezone         string                          `json:"timezone,omitempty"`
	WorklogStyle     string                          `json:"worklog_style,omitempty"`
	Repos            []string                        `json:"repos"`
	BranchSelections map[string]*RepoBranchSelection `json:"branch_selections"`
	IndexFolders     map[string]*IndexFoldersConfig  `json:"index_folders,omitempty"`
	ObsidianVaults   map[string]*ObsidianVaultConfig `json:"obsidian_vaults,omitempty"`

	DefaultProvider string `json:"default_provider,omitempty"`
	DefaultModel    string `json:"default_model,omitempty"`

	AnthropicAPIKey     string `json:"anthropic_api_key,omitempty"`
	OpenAIAPIKey        string `json:"openai_api_key,omitempty"`
	ChatGPTAccessToken  string `json:"chatgpt_access_token,omitempty"`
	ChatGPTRefreshToken string `json:"chatgpt_refresh_token,omitempty"`
	OpenRouterAPIKey    string `json:"openrouter_api_key,omitempty"`
	GeminiAPIKey        string `json:"gemini_api_key,omitempty"`

	AWSRegion          string `json:"aws_region,omitempty"`
	AWSAccessKeyID     string `json:"aws_access_key_id,omitempty"`
	AWSSecretAccessKey string `json:"aws_secret_access_key,omitempty"`

	OllamaBaseURL string `json:"ollama_base_url,omitempty"`
	OllamaModel   string `json:"ollama_model,omitempty"`

	UserName       string `json:"user_name,omitempty"`
	UserEmail      string `json:"user_email,omitempty"`
	GitHubUsername string `json:"github_username,omitempty"`
}

type Config struct {
	OnboardingComplete bool                `json:"onboarding_complete"`
	Profiles           map[string]*Profile `json:"profiles,omitempty"`
	ActiveProfile      string              `json:"active_profile,omitempty"`

	path string

	DefaultProvider     string `json:"-"`
	DefaultModel        string `json:"-"`
	AnthropicAPIKey     string `json:"-"`
	OpenAIAPIKey        string `json:"-"`
	ChatGPTAccessToken  string `json:"-"`
	ChatGPTRefreshToken string `json:"-"`
	OpenRouterAPIKey    string `json:"-"`
	GeminiAPIKey        string `json:"-"`
	AWSRegion           string `json:"-"`
	AWSAccessKeyID      string `json:"-"`
	AWSSecretAccessKey  string `json:"-"`
	OllamaBaseURL       string `json:"-"`
	OllamaModel         string `json:"-"`
	UserName            string `json:"-"`
	UserEmail           string `json:"-"`
	GitHubUsername      string `json:"-"`
}

type Option func(*Config)

func WithDefaultProvider(provider string) Option {
	return func(c *Config) {
		c.DefaultProvider = provider
	}
}

func WithOllamaConfig(baseURL, model string) Option {
	return func(c *Config) {
		c.OllamaBaseURL = baseURL
		c.OllamaModel = model
	}
}

func WithAWSConfig(region, accessKeyID, secretAccessKey string) Option {
	return func(c *Config) {
		c.AWSRegion = region
		c.AWSAccessKeyID = accessKeyID
		c.AWSSecretAccessKey = secretAccessKey
	}
}

func defaultConfig() *Config {
	return &Config{}
}

func GetConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".devlog", DefaultConfigFileName)
	}
	return filepath.Join(homeDir, ".devlog", DefaultConfigFileName)
}

func GetDevlogDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ".devlog"
	}
	return filepath.Join(homeDir, ".devlog")
}

func GetProfileDBPath(name string) string {
	return filepath.Join(GetDevlogDir(), "profiles", name, "devlog.db")
}

func Load(opts ...Option) (*Config, error) {
	return LoadFrom(GetConfigPath(), opts...)
}

func LoadFrom(path string, opts ...Option) (*Config, error) {
	cfg := defaultConfig()
	cfg.path = path

	for _, opt := range opts {
		opt(cfg)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("read config file: %w", err)
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}

	cfg.path = path

	return cfg, nil
}

func (c *Config) Save() error {
	path := c.path
	if path == "" {
		path = GetConfigPath()
	}
	return c.SaveTo(path)
}

func (c *Config) SaveTo(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}

	return nil
}

func (c *Config) GetAPIKey(provider string) string {
	switch provider {
	case "anthropic":
		if c.AnthropicAPIKey != "" {
			return c.AnthropicAPIKey
		}
		return os.Getenv("ANTHROPIC_API_KEY")
	case "openai":
		if c.OpenAIAPIKey != "" {
			return c.OpenAIAPIKey
		}
		return os.Getenv("OPENAI_API_KEY")
	case "chatgpt":
		if c.ChatGPTAccessToken != "" {
			return c.ChatGPTAccessToken
		}
		return os.Getenv("OPENAI_API_KEY")
	case "openrouter":
		if c.OpenRouterAPIKey != "" {
			return c.OpenRouterAPIKey
		}
		return os.Getenv("OPENROUTER_API_KEY")
	case "gemini":
		if c.GeminiAPIKey != "" {
			return c.GeminiAPIKey
		}
		return os.Getenv("GEMINI_API_KEY")
	case "bedrock":
		return c.AWSAccessKeyID
	default:
		return ""
	}
}

func (c *Config) HasProvider(provider string) bool {
	switch provider {
	case "ollama":
		return true
	case "anthropic":
		return c.GetAPIKey("anthropic") != ""
	case "openai":
		return c.GetAPIKey("openai") != ""
	case "chatgpt":
		return c.GetAPIKey("chatgpt") != ""
	case "openrouter":
		return c.GetAPIKey("openrouter") != ""
	case "gemini":
		return c.GetAPIKey("gemini") != ""
	case "bedrock":
		return c.AWSAccessKeyID != "" && c.AWSSecretAccessKey != ""
	default:
		return false
	}
}

func (c *Config) GetActiveProfile() *Profile {
	if c.ActiveProfile == "" || c.Profiles == nil {
		return nil
	}
	return c.Profiles[c.ActiveProfile]
}

func (c *Config) GetActiveProfileName() string {
	if c.ActiveProfile == "" {
		return "default"
	}
	return c.ActiveProfile
}

func (c *Config) CreateProfile(name, description string) error {
	if c.Profiles == nil {
		c.Profiles = make(map[string]*Profile)
	}

	if _, exists := c.Profiles[name]; exists {
		return fmt.Errorf("profile '%s' already exists", name)
	}

	for existingName := range c.Profiles {
		if strings.EqualFold(existingName, name) {
			return fmt.Errorf("profile '%s' conflicts with existing profile '%s' (names are case-insensitive on some filesystems)", name, existingName)
		}
	}

	profileDir := filepath.Join(GetDevlogDir(), "profiles", name)
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		return fmt.Errorf("create profile directory: %w", err)
	}

	c.Profiles[name] = &Profile{
		Name:        name,
		Description: description,
		CreatedAt:   time.Now().Format(time.RFC3339),
		Repos:       []string{},
	}

	return nil
}

func (c *Config) DeleteProfile(name string, deleteData bool) error {
	if c.Profiles == nil {
		return fmt.Errorf("profile '%s' not found", name)
	}

	if _, exists := c.Profiles[name]; !exists {
		return fmt.Errorf("profile '%s' not found", name)
	}

	if c.ActiveProfile == name {
		return fmt.Errorf("cannot delete active profile '%s'; switch to another profile first", name)
	}

	if deleteData {
		profileDir := filepath.Join(GetDevlogDir(), "profiles", name)
		if err := os.RemoveAll(profileDir); err != nil {
			return fmt.Errorf("delete profile data: %w", err)
		}
	}

	delete(c.Profiles, name)
	return nil
}

func (c *Config) SetActiveProfile(name string) error {
	if c.Profiles == nil || c.Profiles[name] == nil {
		return fmt.Errorf("profile '%s' not found", name)
	}
	c.ActiveProfile = name
	return nil
}

func (c *Config) AddRepoToProfile(profileName, repoPath string) error {
	if c.Profiles == nil {
		return fmt.Errorf("profile '%s' not found", profileName)
	}

	profile, exists := c.Profiles[profileName]
	if !exists {
		return fmt.Errorf("profile '%s' not found", profileName)
	}

	// Normalize the path
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		absPath = repoPath
	}

	for _, r := range profile.Repos {
		if r == absPath {
			return nil
		}
	}

	profile.Repos = append(profile.Repos, absPath)
	return nil
}

func (c *Config) RemoveRepoFromProfile(profileName, repoPath string) error {
	if c.Profiles == nil {
		return fmt.Errorf("profile '%s' not found", profileName)
	}

	profile, exists := c.Profiles[profileName]
	if !exists {
		return fmt.Errorf("profile '%s' not found", profileName)
	}

	// Normalize the path
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		absPath = repoPath
	}

	for i, r := range profile.Repos {
		if r == absPath {
			profile.Repos = append(profile.Repos[:i], profile.Repos[i+1:]...)
			return nil
		}
	}

	return nil // Not found, but that's okay
}

// GetBranchSelection returns the saved branch selection for a repo, or nil if not found.
func (c *Config) GetBranchSelection(profileName, repoPath string) *RepoBranchSelection {
	if c.Profiles == nil {
		return nil
	}

	profile, exists := c.Profiles[profileName]
	if !exists || profile.BranchSelections == nil {
		return nil
	}

	// Normalize the path
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		absPath = repoPath
	}

	return profile.BranchSelections[absPath]
}

// SaveBranchSelection saves the branch selection for a repo in a profile.
func (c *Config) SaveBranchSelection(profileName, repoPath, mainBranch string, selectedBranches []string) error {
	if c.Profiles == nil {
		return fmt.Errorf("profile '%s' not found", profileName)
	}

	profile, exists := c.Profiles[profileName]
	if !exists {
		return fmt.Errorf("profile '%s' not found", profileName)
	}

	// Initialize map if needed
	if profile.BranchSelections == nil {
		profile.BranchSelections = make(map[string]*RepoBranchSelection)
	}

	// Normalize the path
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		absPath = repoPath
	}

	profile.BranchSelections[absPath] = &RepoBranchSelection{
		MainBranch:       mainBranch,
		SelectedBranches: selectedBranches,
	}

	return nil
}

// ClearBranchSelection removes the saved branch selection for a repo.
func (c *Config) ClearBranchSelection(profileName, repoPath string) error {
	if c.Profiles == nil {
		return nil
	}

	profile, exists := c.Profiles[profileName]
	if !exists || profile.BranchSelections == nil {
		return nil
	}

	// Normalize the path
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		absPath = repoPath
	}

	delete(profile.BranchSelections, absPath)
	return nil
}

// GetIndexFolders returns the saved index folder selection for a repo, or nil if not found.
func (c *Config) GetIndexFolders(profileName, repoPath string) []string {
	if c.Profiles == nil {
		return nil
	}
	profile, exists := c.Profiles[profileName]
	if !exists || profile.IndexFolders == nil {
		return nil
	}
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		absPath = repoPath
	}
	cfg := profile.IndexFolders[absPath]
	if cfg == nil || len(cfg.Folders) == 0 {
		return nil
	}
	return cfg.Folders
}

// SaveIndexFolders saves the index folder selection for a repo.
func (c *Config) SaveIndexFolders(profileName, repoPath string, folders []string) error {
	if c.Profiles == nil {
		return fmt.Errorf("no profiles found")
	}
	profile, exists := c.Profiles[profileName]
	if !exists {
		return fmt.Errorf("profile '%s' not found", profileName)
	}
	if profile.IndexFolders == nil {
		profile.IndexFolders = make(map[string]*IndexFoldersConfig)
	}
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		absPath = repoPath
	}
	profile.IndexFolders[absPath] = &IndexFoldersConfig{Folders: folders}
	return nil
}

// GetObsidianVault returns the saved Obsidian vault config for a repo, or nil.
func (c *Config) GetObsidianVault(profileName, repoPath string) *ObsidianVaultConfig {
	if c.Profiles == nil {
		return nil
	}
	profile, exists := c.Profiles[profileName]
	if !exists || profile.ObsidianVaults == nil {
		return nil
	}
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		absPath = repoPath
	}
	return profile.ObsidianVaults[absPath]
}

// SaveObsidianVault saves the Obsidian vault config for a repo.
func (c *Config) SaveObsidianVault(profileName, repoPath, vaultPath, rootFolder string) error {
	if c.Profiles == nil {
		return fmt.Errorf("no profiles found")
	}
	profile, exists := c.Profiles[profileName]
	if !exists {
		return fmt.Errorf("profile '%s' not found", profileName)
	}
	if profile.ObsidianVaults == nil {
		profile.ObsidianVaults = make(map[string]*ObsidianVaultConfig)
	}

	absRepoPath, err := filepath.Abs(repoPath)
	if err != nil {
		absRepoPath = repoPath
	}
	absVaultPath, err := filepath.Abs(vaultPath)
	if err != nil {
		absVaultPath = vaultPath
	}

	profile.ObsidianVaults[absRepoPath] = &ObsidianVaultConfig{
		VaultPath:  absVaultPath,
		RootFolder: strings.TrimSpace(rootFolder),
	}
	return nil
}

// MigrateOldDB migrates an old ~/.devlog/devlog.db to profiles/default/devlog.db.
func MigrateOldDB() error {
	devlogDir := GetDevlogDir()
	oldDBPath := filepath.Join(devlogDir, "devlog.db")
	newDBDir := filepath.Join(devlogDir, "profiles", "default")
	newDBPath := filepath.Join(newDBDir, "devlog.db")

	// Check if old DB exists and new one doesn't
	if _, err := os.Stat(oldDBPath); os.IsNotExist(err) {
		return nil // Nothing to migrate
	}

	if _, err := os.Stat(newDBPath); err == nil {
		return nil // Already migrated
	}

	// Create the new directory
	if err := os.MkdirAll(newDBDir, 0755); err != nil {
		return fmt.Errorf("create profile directory: %w", err)
	}

	// Move the database
	if err := os.Rename(oldDBPath, newDBPath); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}

	return nil
}

// EnsureDefaultProfile ensures a default profile exists.
func (c *Config) EnsureDefaultProfile() error {
	if c.Profiles == nil {
		c.Profiles = make(map[string]*Profile)
	}

	if _, exists := c.Profiles["default"]; !exists {
		if err := c.CreateProfile("default", "Default profile"); err != nil {
			return fmt.Errorf("create default profile: %w", err)
		}
	}

	if c.ActiveProfile == "" {
		c.ActiveProfile = "default"
	}

	return nil
}

// ListProfiles returns all profile names.
func (c *Config) ListProfiles() []string {
	if c.Profiles == nil {
		return []string{}
	}
	names := make([]string, 0, len(c.Profiles))
	for name := range c.Profiles {
		names = append(names, name)
	}
	return names
}

// GetTimezone returns the timezone for the active profile, defaulting to UTC
func (c *Config) GetTimezone() string {
	if c.Profiles != nil && c.ActiveProfile != "" {
		if profile := c.Profiles[c.ActiveProfile]; profile != nil && profile.Timezone != "" {
			return profile.Timezone
		}
	}
	return "UTC"
}

// GetWorklogStyle returns the worklog style for the active profile, defaulting to "non-technical"
func (c *Config) GetWorklogStyle() string {
	if c.Profiles != nil && c.ActiveProfile != "" {
		if profile := c.Profiles[c.ActiveProfile]; profile != nil && profile.WorklogStyle != "" {
			return profile.WorklogStyle
		}
	}
	return "non-technical"
}

// SetWorklogStyle sets the worklog style for a profile
func (c *Config) SetWorklogStyle(profileName, style string) error {
	if c.Profiles == nil {
		return fmt.Errorf("profile '%s' not found", profileName)
	}

	profile, exists := c.Profiles[profileName]
	if !exists {
		return fmt.Errorf("profile '%s' not found", profileName)
	}

	if style != "technical" && style != "non-technical" {
		return fmt.Errorf("invalid worklog style: %s (must be 'technical' or 'non-technical')", style)
	}

	profile.WorklogStyle = style
	return nil
}

// ── Per-profile LLM config helpers ─────────────────────────────────────────
// LLM configuration lives exclusively on Profile. These helpers read from
// the active profile, with environment-variable fallback for API keys.

// GetEffectiveProvider returns the LLM provider for the active profile.
func (c *Config) GetEffectiveProvider() string {
	if p := c.GetActiveProfile(); p != nil {
		return p.DefaultProvider
	}
	return ""
}

// GetEffectiveModel returns the LLM model for the active profile.
func (c *Config) GetEffectiveModel() string {
	if p := c.GetActiveProfile(); p != nil {
		return p.DefaultModel
	}
	return ""
}

// GetEffectiveAPIKey returns the API key for a provider from the active profile,
// falling back to environment variables.
func (c *Config) GetEffectiveAPIKey(provider string) string {
	if p := c.GetActiveProfile(); p != nil {
		switch provider {
		case "anthropic":
			if p.AnthropicAPIKey != "" {
				return p.AnthropicAPIKey
			}
		case "openai":
			if p.OpenAIAPIKey != "" {
				return p.OpenAIAPIKey
			}
		case "chatgpt":
			if p.ChatGPTAccessToken != "" {
				return p.ChatGPTAccessToken
			}
		case "openrouter":
			if p.OpenRouterAPIKey != "" {
				return p.OpenRouterAPIKey
			}
		case "gemini":
			if p.GeminiAPIKey != "" {
				return p.GeminiAPIKey
			}
		case "bedrock":
			if p.AWSAccessKeyID != "" {
				return p.AWSAccessKeyID
			}
		}
	}
	// Fall back to environment variables
	switch provider {
	case "anthropic":
		return os.Getenv("ANTHROPIC_API_KEY")
	case "openai":
		return os.Getenv("OPENAI_API_KEY")
	case "chatgpt":
		return os.Getenv("OPENAI_API_KEY")
	case "openrouter":
		return os.Getenv("OPENROUTER_API_KEY")
	case "gemini":
		return os.Getenv("GEMINI_API_KEY")
	default:
		return ""
	}
}

// GetEffectiveChatGPTRefreshToken returns the ChatGPT refresh token for the active profile.
func (c *Config) GetEffectiveChatGPTRefreshToken() string {
	if p := c.GetActiveProfile(); p != nil && p.ChatGPTRefreshToken != "" {
		return p.ChatGPTRefreshToken
	}
	return ""
}

// GetEffectiveOllamaBaseURL returns the Ollama base URL for the active profile.
func (c *Config) GetEffectiveOllamaBaseURL() string {
	if p := c.GetActiveProfile(); p != nil {
		return p.OllamaBaseURL
	}
	return ""
}

// GetEffectiveOllamaModel returns the Ollama model for the active profile.
func (c *Config) GetEffectiveOllamaModel() string {
	if p := c.GetActiveProfile(); p != nil {
		return p.OllamaModel
	}
	return ""
}

// GetEffectiveAWSRegion returns the AWS region for the active profile.
func (c *Config) GetEffectiveAWSRegion() string {
	if p := c.GetActiveProfile(); p != nil {
		return p.AWSRegion
	}
	return ""
}

// GetEffectiveAWSAccessKeyID returns the AWS access key ID for the active profile.
func (c *Config) GetEffectiveAWSAccessKeyID() string {
	if p := c.GetActiveProfile(); p != nil {
		return p.AWSAccessKeyID
	}
	return ""
}

// GetEffectiveAWSSecretAccessKey returns the AWS secret access key for the active profile.
func (c *Config) GetEffectiveAWSSecretAccessKey() string {
	if p := c.GetActiveProfile(); p != nil {
		return p.AWSSecretAccessKey
	}
	return ""
}

// GetEffectiveUserName returns the user name for the active profile.
func (c *Config) GetEffectiveUserName() string {
	if p := c.GetActiveProfile(); p != nil {
		return p.UserName
	}
	return ""
}

// GetEffectiveUserEmail returns the user email for the active profile.
func (c *Config) GetEffectiveUserEmail() string {
	if p := c.GetActiveProfile(); p != nil {
		return p.UserEmail
	}
	return ""
}

// GetEffectiveGitHubUsername returns the GitHub username for the active profile.
func (c *Config) GetEffectiveGitHubUsername() string {
	if p := c.GetActiveProfile(); p != nil {
		return p.GitHubUsername
	}
	return ""
}

// CopyLLMConfigToProfile copies the transient Config fields into the named profile.
// Used by TUI onboarding/configure flows that write to Config fields as scratch
// space during the interactive session, then persist to the profile on save.
func (c *Config) CopyLLMConfigToProfile(profileName string) {
	if c.Profiles == nil {
		return
	}
	profile, exists := c.Profiles[profileName]
	if !exists || profile == nil {
		return
	}
	profile.DefaultProvider = c.DefaultProvider
	profile.DefaultModel = c.DefaultModel
	profile.AnthropicAPIKey = c.AnthropicAPIKey
	profile.OpenAIAPIKey = c.OpenAIAPIKey
	profile.ChatGPTAccessToken = c.ChatGPTAccessToken
	profile.ChatGPTRefreshToken = c.ChatGPTRefreshToken
	profile.OpenRouterAPIKey = c.OpenRouterAPIKey
	profile.GeminiAPIKey = c.GeminiAPIKey
	profile.AWSRegion = c.AWSRegion
	profile.AWSAccessKeyID = c.AWSAccessKeyID
	profile.AWSSecretAccessKey = c.AWSSecretAccessKey
	profile.OllamaBaseURL = c.OllamaBaseURL
	profile.OllamaModel = c.OllamaModel
	profile.UserName = c.UserName
	profile.UserEmail = c.UserEmail
	profile.GitHubUsername = c.GitHubUsername
}

// HydrateGlobalFromActiveProfile copies the active profile's config into the
// transient Config fields. Used by TUI flows so they can read/write the Config
// fields as scratch space during interactive configuration.
func (c *Config) HydrateGlobalFromActiveProfile() {
	p := c.GetActiveProfile()
	if p == nil {
		return
	}
	c.DefaultProvider = p.DefaultProvider
	c.DefaultModel = p.DefaultModel
	c.AnthropicAPIKey = p.AnthropicAPIKey
	c.OpenAIAPIKey = p.OpenAIAPIKey
	c.ChatGPTAccessToken = p.ChatGPTAccessToken
	c.ChatGPTRefreshToken = p.ChatGPTRefreshToken
	c.OpenRouterAPIKey = p.OpenRouterAPIKey
	c.GeminiAPIKey = p.GeminiAPIKey
	c.AWSRegion = p.AWSRegion
	c.AWSAccessKeyID = p.AWSAccessKeyID
	c.AWSSecretAccessKey = p.AWSSecretAccessKey
	c.OllamaBaseURL = p.OllamaBaseURL
	c.OllamaModel = p.OllamaModel
	c.UserName = p.UserName
	c.UserEmail = p.UserEmail
	c.GitHubUsername = p.GitHubUsername
}

// ApplyLLMConfigToAllProfiles copies the named profile's LLM configuration
// (provider, model, API keys) to every other profile. Used by `devlog models set --global`.
func (c *Config) ApplyLLMConfigToAllProfiles(sourceProfileName string) error {
	if c.Profiles == nil {
		return fmt.Errorf("no profiles exist")
	}
	src, exists := c.Profiles[sourceProfileName]
	if !exists || src == nil {
		return fmt.Errorf("profile '%s' not found", sourceProfileName)
	}
	for name, profile := range c.Profiles {
		if name == sourceProfileName {
			continue
		}
		profile.DefaultProvider = src.DefaultProvider
		profile.DefaultModel = src.DefaultModel
		profile.AnthropicAPIKey = src.AnthropicAPIKey
		profile.OpenAIAPIKey = src.OpenAIAPIKey
		profile.ChatGPTAccessToken = src.ChatGPTAccessToken
		profile.ChatGPTRefreshToken = src.ChatGPTRefreshToken
		profile.OpenRouterAPIKey = src.OpenRouterAPIKey
		profile.GeminiAPIKey = src.GeminiAPIKey
		profile.AWSRegion = src.AWSRegion
		profile.AWSAccessKeyID = src.AWSAccessKeyID
		profile.AWSSecretAccessKey = src.AWSSecretAccessKey
		profile.OllamaBaseURL = src.OllamaBaseURL
		profile.OllamaModel = src.OllamaModel
	}
	return nil
}
