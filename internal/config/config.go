package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Profile represents a work context with isolated data
type Profile struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	CreatedAt   string   `json:"created_at"`
	Repos       []string `json:"repos"` // Repo paths in this profile
}

type Config struct {
	DefaultProvider string `json:"default_provider"`
	DefaultModel    string `json:"default_model"`

	// API Keys
	AnthropicAPIKey string `json:"anthropic_api_key,omitempty"`
	OpenAIAPIKey    string `json:"openai_api_key,omitempty"`

	// Bedrock config
	AWSRegion          string `json:"aws_region,omitempty"`
	AWSAccessKeyID     string `json:"aws_access_key_id,omitempty"`
	AWSSecretAccessKey string `json:"aws_secret_access_key,omitempty"`

	// Ollama config
	OllamaBaseURL string `json:"ollama_base_url,omitempty"`
	OllamaModel   string `json:"ollama_model,omitempty"`

	// User info
	UserName  string `json:"user_name,omitempty"`
	UserEmail string `json:"user_email,omitempty"`

	// Onboarding
	OnboardingComplete bool `json:"onboarding_complete"`

	// Profiles
	Profiles      map[string]*Profile `json:"profiles,omitempty"`
	ActiveProfile string              `json:"active_profile,omitempty"`
}

var configPath string

func init() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		configPath = ".devlog/config.json"
		return
	}
	configPath = filepath.Join(homeDir, ".devlog", "config.json")
}

func GetConfigPath() string {
	return configPath
}

func Load() (*Config, error) {
	cfg := &Config{
		DefaultProvider: "ollama",
		OllamaBaseURL:   "http://localhost:11434",
		OllamaModel:     "llama3.2",
		AWSRegion:       "us-east-1",
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return cfg, nil
}

func (c *Config) Save() error {
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
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
	case "bedrock":
		// Bedrock uses AWS credentials
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
	case "bedrock":
		return c.AWSAccessKeyID != "" && c.AWSSecretAccessKey != ""
	default:
		return false
	}
}

// GetDevlogDir returns the base devlog directory path
func GetDevlogDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ".devlog"
	}
	return filepath.Join(homeDir, ".devlog")
}

// GetActiveProfile returns the active profile, or nil if none
func (c *Config) GetActiveProfile() *Profile {
	if c.ActiveProfile == "" || c.Profiles == nil {
		return nil
	}
	return c.Profiles[c.ActiveProfile]
}

// GetActiveProfileName returns the active profile name, defaulting to "default"
func (c *Config) GetActiveProfileName() string {
	if c.ActiveProfile == "" {
		return "default"
	}
	return c.ActiveProfile
}

// GetProfileDBPath returns the database path for a given profile
func GetProfileDBPath(name string) string {
	return filepath.Join(GetDevlogDir(), "profiles", name, "devlog.db")
}

// CreateProfile creates a new profile
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
		return fmt.Errorf("failed to create profile directory: %w", err)
	}

	c.Profiles[name] = &Profile{
		Name:        name,
		Description: description,
		CreatedAt:   time.Now().Format(time.RFC3339),
		Repos:       []string{},
	}

	return nil
}

// DeleteProfile removes a profile and optionally its data
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
			return fmt.Errorf("failed to delete profile data: %w", err)
		}
	}

	delete(c.Profiles, name)
	return nil
}

// SetActiveProfile switches to a different profile
func (c *Config) SetActiveProfile(name string) error {
	if c.Profiles == nil || c.Profiles[name] == nil {
		return fmt.Errorf("profile '%s' not found", name)
	}
	c.ActiveProfile = name
	return nil
}

// AddRepoToProfile adds a repository path to a profile's repo list
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

// RemoveRepoFromProfile removes a repository path from a profile's repo list
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

// MigrateOldDB migrates an old ~/.devlog/devlog.db to profiles/default/devlog.db
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
		return fmt.Errorf("failed to create profile directory: %w", err)
	}

	// Move the database
	if err := os.Rename(oldDBPath, newDBPath); err != nil {
		return fmt.Errorf("failed to migrate database: %w", err)
	}

	return nil
}

// EnsureDefaultProfile ensures a default profile exists
func (c *Config) EnsureDefaultProfile() error {
	if c.Profiles == nil {
		c.Profiles = make(map[string]*Profile)
	}

	if _, exists := c.Profiles["default"]; !exists {
		if err := c.CreateProfile("default", "Default profile"); err != nil {
			return err
		}
	}

	if c.ActiveProfile == "" {
		c.ActiveProfile = "default"
	}

	return nil
}

// ListProfiles returns all profile names
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
