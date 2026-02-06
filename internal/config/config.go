package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// DefaultConfigFileName is the name of the configuration file.
const DefaultConfigFileName = "config.json"

// RepoBranchSelection stores the saved branch selection for a repo.
type RepoBranchSelection struct {
	MainBranch       string   `json:"main_branch"`
	SelectedBranches []string `json:"selected_branches"`
}

// Profile represents a work context with isolated data.
type Profile struct {
	Name             string                          `json:"name"`
	Description      string                          `json:"description,omitempty"`
	CreatedAt        string                          `json:"created_at"`
	Repos            []string                        `json:"repos"`             // Repo paths in this profile
	BranchSelections map[string]*RepoBranchSelection `json:"branch_selections"` // Saved branch selections per repo path
}

// Config holds all configuration for devlog.
type Config struct {
	// Provider configuration
	DefaultProvider   string `json:"default_provider"`
	DefaultModel      string `json:"default_model"`
	DefaultEmbedModel string `json:"default_embed_model,omitempty"`
	EmbeddingProvider string `json:"embedding_provider,omitempty"` // Can be different from DefaultProvider

	// API Keys
	AnthropicAPIKey  string `json:"anthropic_api_key,omitempty"`
	OpenAIAPIKey     string `json:"openai_api_key,omitempty"`
	OpenRouterAPIKey string `json:"openrouter_api_key,omitempty"`
	VoyageAIAPIKey   string `json:"voyageai_api_key,omitempty"`

	// Bedrock config
	AWSRegion          string `json:"aws_region,omitempty"`
	AWSAccessKeyID     string `json:"aws_access_key_id,omitempty"`
	AWSSecretAccessKey string `json:"aws_secret_access_key,omitempty"`

	// Ollama config
	OllamaBaseURL string `json:"ollama_base_url,omitempty"`
	OllamaModel   string `json:"ollama_model,omitempty"`

	// User info
	UserName       string `json:"user_name,omitempty"`
	UserEmail      string `json:"user_email,omitempty"`
	GitHubUsername string `json:"github_username,omitempty"`

	// Onboarding
	OnboardingComplete bool `json:"onboarding_complete"`

	// Profiles
	Profiles      map[string]*Profile `json:"profiles,omitempty"`
	ActiveProfile string              `json:"active_profile,omitempty"`

	// Internal: path where config was loaded from
	path string
}

// Option is a functional option for configuring Config defaults.
type Option func(*Config)

// WithDefaultProvider sets the default provider.
func WithDefaultProvider(provider string) Option {
	return func(c *Config) {
		c.DefaultProvider = provider
	}
}

// WithOllamaConfig sets Ollama configuration.
func WithOllamaConfig(baseURL, model string) Option {
	return func(c *Config) {
		c.OllamaBaseURL = baseURL
		c.OllamaModel = model
	}
}

// WithAWSConfig sets AWS configuration.
func WithAWSConfig(region, accessKeyID, secretAccessKey string) Option {
	return func(c *Config) {
		c.AWSRegion = region
		c.AWSAccessKeyID = accessKeyID
		c.AWSSecretAccessKey = secretAccessKey
	}
}

// defaultConfig returns a new Config with empty fields.
// Users must run 'devlog onboard' to populate the configuration.
func defaultConfig() *Config {
	return &Config{}
}

// GetConfigPath returns the default configuration file path.
func GetConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".devlog", DefaultConfigFileName)
	}
	return filepath.Join(homeDir, ".devlog", DefaultConfigFileName)
}

// GetDevlogDir returns the base devlog directory path.
func GetDevlogDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ".devlog"
	}
	return filepath.Join(homeDir, ".devlog")
}

// GetProfileDBPath returns the database path for a given profile.
func GetProfileDBPath(name string) string {
	return filepath.Join(GetDevlogDir(), "profiles", name, "devlog.db")
}

// Load loads configuration from the default path.
func Load(opts ...Option) (*Config, error) {
	return LoadFrom(GetConfigPath(), opts...)
}

// LoadFrom loads configuration from a specific path.
func LoadFrom(path string, opts ...Option) (*Config, error) {
	cfg := defaultConfig()
	cfg.path = path

	// Apply options
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

	// Preserve the path
	cfg.path = path

	return cfg, nil
}

// Save saves the configuration to its original path.
func (c *Config) Save() error {
	path := c.path
	if path == "" {
		path = GetConfigPath()
	}
	return c.SaveTo(path)
}

// SaveTo saves the configuration to a specific path.
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

// GetAPIKey returns the API key for a provider.
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
	case "openrouter":
		if c.OpenRouterAPIKey != "" {
			return c.OpenRouterAPIKey
		}
		return os.Getenv("OPENROUTER_API_KEY")
	case "voyageai":
		if c.VoyageAIAPIKey != "" {
			return c.VoyageAIAPIKey
		}
		return os.Getenv("VOYAGEAI_API_KEY")
	case "bedrock":
		return c.AWSAccessKeyID
	default:
		return ""
	}
}

// HasProvider checks if a provider is configured.
func (c *Config) HasProvider(provider string) bool {
	switch provider {
	case "ollama":
		return true
	case "anthropic":
		return c.GetAPIKey("anthropic") != ""
	case "openai":
		return c.GetAPIKey("openai") != ""
	case "openrouter":
		return c.GetAPIKey("openrouter") != ""
	case "voyageai":
		return c.GetAPIKey("voyageai") != ""
	case "bedrock":
		return c.AWSAccessKeyID != "" && c.AWSSecretAccessKey != ""
	default:
		return false
	}
}

// GetActiveProfile returns the active profile, or nil if none.
func (c *Config) GetActiveProfile() *Profile {
	if c.ActiveProfile == "" || c.Profiles == nil {
		return nil
	}
	return c.Profiles[c.ActiveProfile]
}

// GetActiveProfileName returns the active profile name, defaulting to "default".
func (c *Config) GetActiveProfileName() string {
	if c.ActiveProfile == "" {
		return "default"
	}
	return c.ActiveProfile
}

// CreateProfile creates a new profile.
func (c *Config) CreateProfile(name, description string) error {
	if c.Profiles == nil {
		c.Profiles = make(map[string]*Profile)
	}

	if _, exists := c.Profiles[name]; exists {
		return fmt.Errorf("profile '%s' already exists", name)
	}

	// Create profile directory
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

// DeleteProfile removes a profile and optionally its data.
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

// SetActiveProfile switches to a different profile.
func (c *Config) SetActiveProfile(name string) error {
	if c.Profiles == nil || c.Profiles[name] == nil {
		return fmt.Errorf("profile '%s' not found", name)
	}
	c.ActiveProfile = name
	return nil
}

// AddRepoToProfile adds a repository path to a profile's repo list.
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

	// Check if already added
	for _, r := range profile.Repos {
		if r == absPath {
			return nil // Already exists
		}
	}

	profile.Repos = append(profile.Repos, absPath)
	return nil
}

// RemoveRepoFromProfile removes a repository path from a profile's repo list.
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

	// Find and remove
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
