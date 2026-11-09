// Package setup provides interactive configuration setup.
package setup

import (
	"fmt"
	"os"

	"charm.land/huh/v2"
	"github.com/rs/zerolog/log"

	"codefloe.com/pat-s/backporter/pkg/config"
)

// PromptForConfigCreation prompts user to create a config file.
func PromptForConfigCreation() error {
	fmt.Println("No configuration file found and forge type is not configured.")
	fmt.Println("Without configuration, PR features will not be available.")

	var createConfig bool
	err := huh.NewConfirm().
		Title("Would you like to create a configuration file now?").
		Affirmative("Yes").
		Negative("No").
		Value(&createConfig).
		Run()
	if err != nil {
		return err
	}

	if !createConfig {
		log.Warn().Msg("continuing without configuration - PR features will be unavailable")
		return nil
	}

	return CreateConfigInteractive()
}

// CreateConfigInteractive creates a config file interactively.
func CreateConfigInteractive() error {
	cfg := config.DefaultConfig()

	// Select forge type.
	var forgeType string
	err := huh.NewSelect[string]().
		Title("Select your forge type:").
		Options(
			huh.NewOption("GitHub", "github"),
			huh.NewOption("Forgejo/Gitea", "forgejo"),
			huh.NewOption("None (skip)", ""),
		).
		Value(&forgeType).
		Run()
	if err != nil {
		return err
	}

	cfg.ForgeType = forgeType

	switch forgeType {
	case "forgejo":
		// Query for Forgejo URL.
		var forgejoURL string
		err = huh.NewInput().
			Title("Forgejo instance URL (e.g., https://codeberg.org):").
			Value(&forgejoURL).
			Validate(func(s string) error {
				if s == "" {
					return fmt.Errorf("URL is required for Forgejo")
				}
				return nil
			}).
			Run()
		if err != nil {
			return err
		}

		cfg.ForgejoURL = forgejoURL

		fmt.Println("\nNote: Set FORGEJO_TOKEN environment variable:")
		fmt.Println("  export FORGEJO_TOKEN=<your-token>")
		fmt.Println("\nRequired token scopes for Forgejo/Gitea:")
		fmt.Println("  - repository:read (to fetch PR information)")
	case "github":
		fmt.Println("\nNote: Set GITHUB_TOKEN environment variable:")
		fmt.Println("  export GITHUB_TOKEN=<your-token>")
		fmt.Println("\nRequired token scopes for GitHub:")
		fmt.Println("  - repo (for private repositories)")
		fmt.Println("  - public_repo (for public repositories only)")
	}

	// Select default branch.
	var defaultBranch string
	err = huh.NewInput().
		Title("Default branch name:").
		Value(&defaultBranch).
		Placeholder("main").
		Run()
	if err != nil {
		return err
	}

	if defaultBranch != "" {
		cfg.DefaultBranch = defaultBranch
	}

	// Select remote.
	var remote string
	err = huh.NewInput().
		Title("Git remote name:").
		Value(&remote).
		Placeholder("origin").
		Run()
	if err != nil {
		return err
	}

	if remote != "" {
		cfg.Remote = remote
	}

	// Enable cache?
	var enableCache bool
	err = huh.NewConfirm().
		Title("Enable caching of backported commits/PRs?").
		Affirmative("Yes").
		Negative("No").
		Value(&enableCache).
		Run()
	if err != nil {
		return err
	}

	cfg.Cache.Enabled = enableCache

	// Select config file location.
	var configLocation string
	err = huh.NewSelect[string]().
		Title("Where should the config file be saved?").
		Options(
			huh.NewOption("Repository (.backporter.yaml)", "repo"),
			huh.NewOption("Global (~/.config/backporter/config.yaml)", "global"),
		).
		Value(&configLocation).
		Run()
	if err != nil {
		return err
	}

	var configPath string
	if configLocation == "global" {
		configPath = config.GlobalConfigPath()
	} else {
		configPath = config.RepoConfigPath()
	}

	if err := cfg.SaveToFile(configPath); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("\nConfiguration saved to: %s\n", configPath)

	return nil
}

// ShouldPromptForConfig checks if we should prompt user to create config.
func ShouldPromptForConfig() bool {
	// Check if any config file exists.
	globalPath := config.GlobalConfigPath()
	repoPath := config.RepoConfigPath()

	_, errGlobal := os.Stat(globalPath)
	_, errRepo := os.Stat(repoPath)

	// If no config files exist at all, prompt.
	return os.IsNotExist(errGlobal) && os.IsNotExist(errRepo)
}
