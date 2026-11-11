// Package config provides configuration management for backporter.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
)

// DefaultRecentPRCount is the default number of recent PRs to show in interactive mode.
const DefaultRecentPRCount = 10

// Config represents the backporter configuration.
type Config struct {
	// Forge type: "github" or "forgejo".
	ForgeType string `yaml:"forge_type"`

	// Forgejo/Gitea instance URL (only for forgejo forge type).
	ForgejoURL string `yaml:"forgejo_url,omitempty"`

	// Default target branches for backporting. In CI mode, "latest:<glob>"
	// resolves to the newest matching branch of the remote.
	TargetBranches []string `yaml:"target_branches"`

	// Default commit message template.
	CommitMessage string `yaml:"commit_message"`

	// Default author name for commits.
	AuthorName string `yaml:"author_name"`

	// Default author email for commits.
	AuthorEmail string `yaml:"author_email"`

	// Default branch to work from.
	DefaultBranch string `yaml:"default_branch"`

	// Remote name.
	Remote string `yaml:"remote"`

	// Number of recent PRs to show in interactive mode.
	RecentPRCount int `yaml:"recent_pr_count"`

	// Cache settings.
	Cache CacheConfig `yaml:"cache"`

	// CI settings for automated backporting.
	CI CIConfig `yaml:"ci"`
}

// CacheConfig holds cache-related settings.
type CacheConfig struct {
	// Enable caching of backported commits/PRs.
	Enabled bool `yaml:"enabled"`

	// Path to cache file.
	Path string `yaml:"path"`
}

// CIConfig holds CI-specific settings for automated backporting.
type CIConfig struct {
	// Default conventional commit prefix when original PR title doesn't have one.
	// Default: "fix"
	DefaultPrefix string `yaml:"default_prefix"`
}

// DefaultConfig returns a new Config with default values.
func DefaultConfig() *Config {
	return &Config{
		ForgeType:      "",
		TargetBranches: []string{},
		CommitMessage:  "",
		AuthorName:     "",
		AuthorEmail:    "",
		DefaultBranch:  "main",
		Remote:         "origin",
		RecentPRCount:  DefaultRecentPRCount,
		Cache: CacheConfig{
			Enabled: true,
			Path:    "",
		},
		CI: CIConfig{
			DefaultPrefix: "fix",
		},
	}
}

// LoadFromFile loads configuration from a YAML file.
func LoadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return cfg, nil
}

// Merge merges another config into this one. Values from other take precedence if non-empty.
func (c *Config) Merge(other *Config) {
	if other == nil {
		return
	}

	if other.ForgeType != "" {
		c.ForgeType = other.ForgeType
	}
	if other.ForgejoURL != "" {
		c.ForgejoURL = other.ForgejoURL
	}
	if len(other.TargetBranches) > 0 {
		c.TargetBranches = other.TargetBranches
	}
	if other.CommitMessage != "" {
		c.CommitMessage = other.CommitMessage
	}
	if other.AuthorName != "" {
		c.AuthorName = other.AuthorName
	}
	if other.AuthorEmail != "" {
		c.AuthorEmail = other.AuthorEmail
	}
	if other.DefaultBranch != "" {
		c.DefaultBranch = other.DefaultBranch
	}
	if other.Remote != "" {
		c.Remote = other.Remote
	}
	if other.RecentPRCount > 0 {
		c.RecentPRCount = other.RecentPRCount
	}
	if other.Cache.Path != "" {
		c.Cache.Path = other.Cache.Path
	}
	// Always take explicit boolean settings.
	c.Cache.Enabled = other.Cache.Enabled

	// CI settings.
	if other.CI.DefaultPrefix != "" {
		c.CI.DefaultPrefix = other.CI.DefaultPrefix
	}
}

// GlobalConfigPath returns the path to the global config file.
func GlobalConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "backporter", "config.yaml")
}

// RepoConfigPath returns the path to the repo-local config file.
func RepoConfigPath() string {
	return ".backporter.yaml"
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	if c.ForgeType != "" && c.ForgeType != "github" && c.ForgeType != "forgejo" {
		return fmt.Errorf("invalid forge_type: %s (must be 'github' or 'forgejo')", c.ForgeType)
	}
	return nil
}

// SaveToFile saves the configuration to a YAML file.
func (c *Config) SaveToFile(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
