package backport

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"charm.land/huh/v2"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"

	"codefloe.com/pat-s/backporter/cli/internal"
	cliconfig "codefloe.com/pat-s/backporter/cli/internal/config"
	"codefloe.com/pat-s/backporter/pkg/backport"
	"codefloe.com/pat-s/backporter/pkg/config"
	"codefloe.com/pat-s/backporter/pkg/forge"
	"codefloe.com/pat-s/backporter/pkg/git"
)

// Interactive runs the interactive backport wizard.
func Interactive(ctx context.Context, c *cli.Command) error {
	log.Info().Msg("starting interactive backport wizard")

	// Check if we're in a git repository.
	repo, err := git.OpenCurrent()
	if err != nil {
		return fmt.Errorf("failed to open git repository: %w (make sure you are in a git repository)", err)
	}

	// Verify we have commits (HEAD exists).
	if _, err := repo.CurrentBranch(); err != nil {
		return fmt.Errorf("git repository has no commits - please create at least one commit first")
	}

	// Check if running in CI.
	if c.Args().Len() > 0 {
		// Not interactive mode, check if first arg looks like a SHA or number.
		firstArg := c.Args().First()
		if looksLikeSHA(firstArg) {
			// Direct commit SHA provided.
			if c.Args().Len() < 2 { //nolint:mnd
				return fmt.Errorf("usage: backporter <commit-sha> <target-branch>")
			}
			return backportCommit(ctx, c)
		}

		// Check if it's a PR number.
		if _, err := strconv.Atoi(firstArg); err == nil {
			if c.Args().Len() < 2 { //nolint:mnd
				return fmt.Errorf("usage: backporter <pr-number> <target-branch>")
			}
			return backportPR(ctx, c)
		}

		return fmt.Errorf("unrecognized argument: %s", firstArg)
	}

	// Get branches for selection.
	branches, err := repo.ListBranches()
	if err != nil {
		return fmt.Errorf("failed to list branches: %w", err)
	}

	// Load config to check for target branches.
	cfg, err := cliconfig.GetConfig(c)
	if err != nil {
		return err
	}

	// Check if configured target branches exist, offer to create if not.
	if len(cfg.TargetBranches) > 0 {
		branches, err = checkAndCreateTargetBranches(branches, cfg.TargetBranches)
		if err != nil {
			return err
		}
	}

	// Create options for branch selection, prioritizing configured target branches.
	branchOptions := createBranchOptions(branches, cfg.TargetBranches)

	// Ask what to backport.
	var backportType string
	err = huh.NewSelect[string]().
		Title("What do you want to backport?").
		Options(
			huh.NewOption("Pull Request", "pr"),
			huh.NewOption("Commit", "commit"),
		).
		Value(&backportType).
		Run()
	if err != nil {
		return err
	}

	var targetBranch string

	if backportType == "pr" {
		return interactivePR(ctx, c, branchOptions, &targetBranch)
	}

	return interactiveCommit(ctx, c, branchOptions, &targetBranch)
}

func interactivePR(ctx context.Context, c *cli.Command, branchOptions []huh.Option[string], targetBranch *string) error {
	cfg, err := cliconfig.GetConfig(c)
	if err != nil {
		return err
	}

	if cfg.ForgeType == "" {
		return fmt.Errorf("forge_type not configured, cannot fetch PRs")
	}

	service, err := internal.CreateService(ctx, c)
	if err != nil {
		return err
	}

	// Get recent PRs.
	log.Info().Msg("fetching recent PRs...")

	repo, err := git.OpenCurrent()
	if err != nil {
		return err
	}

	remote := c.String("remote")
	if remote == "" {
		remote = cfg.Remote
	}

	remoteURL, err := repo.RemoteURL(remote)
	if err != nil {
		return err
	}

	owner, repoName, err := git.ParseRemoteURL(remoteURL)
	if err != nil {
		return err
	}

	token := ""
	switch cfg.ForgeType {
	case "github":
		token = getEnvToken("GITHUB_TOKEN")
	case "forgejo":
		token = getEnvToken("FORGEJO_TOKEN")
	}

	forgeOpts := forge.NewOptions{
		ForgejoURL: cfg.ForgejoURL,
	}
	forgeClient, err := forge.NewWithOptions(cfg.ForgeType, token, forgeOpts)
	if err != nil {
		return err
	}

	prLimit := cfg.RecentPRCount
	if prLimit <= 0 {
		prLimit = config.DefaultRecentPRCount
	}

	prs, err := forgeClient.ListRecentPRs(ctx, owner, repoName, prLimit)
	if err != nil {
		log.Warn().Err(err).Msg("failed to fetch recent PRs")
		// Fall back to manual input.
		return interactivePRManualInput(ctx, service, branchOptions, targetBranch)
	}

	// Loop to allow loading more PRs.
	for {
		selectedPR, loadMore, err := selectPRFromList(prs)
		if err != nil {
			return err
		}

		if loadMore {
			// Fetch more PRs.
			prLimit += cfg.RecentPRCount
			if prLimit <= 0 {
				prLimit = config.DefaultRecentPRCount * prLoadMoreMultiple
			}
			log.Info().Int("limit", prLimit).Msg("fetching more PRs...")
			prs, err = forgeClient.ListRecentPRs(ctx, owner, repoName, prLimit)
			if err != nil {
				return fmt.Errorf("failed to fetch more PRs: %w", err)
			}
			continue
		}

		if selectedPR == -1 {
			// Manual input selected.
			return interactivePRManualInput(ctx, service, branchOptions, targetBranch)
		}

		// Get target branch.
		err = huh.NewSelect[string]().
			Title("Select target branch to backport to:").
			Description("⭐ indicates configured target branches").
			Options(branchOptions...).
			Value(targetBranch).
			Run()
		if err != nil {
			return err
		}

		opts := backport.BackportOptions{
			TargetBranch: *targetBranch,
		}

		result, err := service.BackportPR(ctx, selectedPR, opts)
		if err != nil {
			return err
		}

		return handleBackportResult(result)
	}
}

const (
	prLoadMoreValue    = -2
	prManualInputValue = -1
	prLoadMoreMultiple = 2
)

func selectPRFromList(prs []*forge.PRInfo) (int, bool, error) {
	// Create PR options with special actions.
	prOptions := make([]huh.Option[int], 0, len(prs)+2)

	for _, pr := range prs {
		label := fmt.Sprintf("#%d - %s (%s)", pr.Number, pr.Title, pr.Author)
		if len(label) > 80 { //nolint:mnd
			label = label[:77] + "..."
		}
		prOptions = append(prOptions, huh.NewOption(label, pr.Number))
	}

	// Add special options at the end.
	prOptions = append(prOptions, huh.NewOption("▼ Load more PRs...", prLoadMoreValue))
	prOptions = append(prOptions, huh.NewOption("✎ Enter PR number manually", prManualInputValue))

	var selectedPR int
	err := huh.NewSelect[int]().
		Title(fmt.Sprintf("Select PR to backport (showing %d):", len(prs))).
		Options(prOptions...).
		Value(&selectedPR).
		Run()
	if err != nil {
		return 0, false, err
	}

	if selectedPR == prLoadMoreValue {
		return 0, true, nil
	}

	return selectedPR, false, nil
}

func interactivePRManualInput(ctx context.Context, service *backport.Service, branchOptions []huh.Option[string], targetBranch *string) error {
	var prNumberStr string
	err := huh.NewInput().
		Title("Enter PR number:").
		Validate(func(s string) error {
			_, e := strconv.Atoi(s)
			return e
		}).
		Value(&prNumberStr).
		Run()
	if err != nil {
		return err
	}

	prNumber, _ := strconv.Atoi(prNumberStr)

	// Get target branch.
	err = huh.NewSelect[string]().
		Title("Select target branch to backport to:").
		Description("⭐ indicates configured target branches").
		Options(branchOptions...).
		Value(targetBranch).
		Run()
	if err != nil {
		return err
	}

	opts := backport.BackportOptions{
		TargetBranch: *targetBranch,
	}

	result, err := service.BackportPR(ctx, prNumber, opts)
	if err != nil {
		return err
	}

	return handleBackportResult(result)
}

func interactiveCommit(ctx context.Context, c *cli.Command, branchOptions []huh.Option[string], targetBranch *string) error {
	service, err := internal.CreateService(ctx, c)
	if err != nil {
		return err
	}

	var sha string
	err = huh.NewInput().
		Title("Enter commit SHA:").
		Value(&sha).
		Validate(func(s string) error {
			if len(s) < 7 { //nolint:mnd
				return fmt.Errorf("SHA too short")
			}
			return nil
		}).
		Run()
	if err != nil {
		return err
	}

	// Get target branch.
	err = huh.NewSelect[string]().
		Title("Select target branch to backport to:").
		Description("⭐ indicates configured target branches").
		Options(branchOptions...).
		Value(targetBranch).
		Run()
	if err != nil {
		return err
	}

	opts := backport.BackportOptions{
		TargetBranch: *targetBranch,
	}

	result, err := service.BackportCommit(ctx, sha, opts)
	if err != nil {
		return err
	}

	return handleBackportResult(result)
}

func looksLikeSHA(s string) bool {
	if len(s) < 7 { //nolint:mnd
		return false
	}

	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}

	return true
}

func getEnvToken(key string) string {
	return os.Getenv(key)
}

func checkAndCreateTargetBranches(existingBranches, targetBranches []string) ([]string, error) {
	// Build a set of existing branches for quick lookup.
	existingSet := make(map[string]bool)
	for _, b := range existingBranches {
		existingSet[b] = true
	}

	// Find missing branches (skip regex patterns).
	var missingBranches []string
	for _, target := range targetBranches {
		// Skip if it looks like a regex pattern.
		if containsRegexChars(target) {
			continue
		}
		if !existingSet[target] {
			missingBranches = append(missingBranches, target)
		}
	}

	if len(missingBranches) == 0 {
		return existingBranches, nil
	}

	// Prompt to create missing branches.
	fmt.Printf("\nThe following configured target branches do not exist:\n")
	for _, b := range missingBranches {
		fmt.Printf("  - %s\n", b)
	}

	var createBranches bool
	err := huh.NewConfirm().
		Title("Would you like to create these branches?").
		Affirmative("Yes").
		Negative("No").
		Value(&createBranches).
		Run()
	if err != nil {
		return nil, err
	}

	if !createBranches {
		return existingBranches, nil
	}

	// Get the base branch for new branches.
	var baseBranch string
	baseOptions := make([]huh.Option[string], len(existingBranches))
	for i, b := range existingBranches {
		baseOptions[i] = huh.NewOption(b, b)
	}

	err = huh.NewSelect[string]().
		Title("Select base branch for new branches:").
		Options(baseOptions...).
		Value(&baseBranch).
		Run()
	if err != nil {
		return nil, err
	}

	// Create the missing branches.
	for _, branchName := range missingBranches {
		log.Info().Str("branch", branchName).Str("base", baseBranch).Msg("creating branch")
		if err := git.CreateBranchFrom(branchName, baseBranch); err != nil {
			return nil, fmt.Errorf("failed to create branch %s: %w", branchName, err)
		}
		existingBranches = append(existingBranches, branchName)
	}

	fmt.Printf("\nCreated %d branch(es)\n", len(missingBranches))
	return existingBranches, nil
}

func containsRegexChars(s string) bool {
	regexChars := []rune{'*', '+', '?', '.', '[', ']', '(', ')', '{', '}', '|', '^', '$', '\\'}
	for _, c := range s {
		for _, rc := range regexChars {
			if c == rc {
				return true
			}
		}
	}
	return false
}

// createBranchOptions creates branch selection options, prioritizing configured target branches.
func createBranchOptions(branches, targetBranches []string) []huh.Option[string] {
	if len(targetBranches) == 0 {
		// No target branches configured, return all branches as-is.
		options := make([]huh.Option[string], len(branches))
		for i, branch := range branches {
			options[i] = huh.NewOption(branch, branch)
		}
		return options
	}

	// Build a set of target branches for exact matching.
	// We prioritize branches that match exactly, regardless of whether
	// the pattern contains regex characters (e.g., "v4.4.x" is a literal branch name).
	targetSet := make(map[string]bool)
	for _, target := range targetBranches {
		targetSet[target] = true
	}

	// Separate branches into targets and others.
	var targetOpts, otherOpts []huh.Option[string]
	for _, branch := range branches {
		if targetSet[branch] {
			// Mark target branches with a visual indicator.
			targetOpts = append(targetOpts, huh.NewOption("⭐ "+branch, branch))
		} else {
			otherOpts = append(otherOpts, huh.NewOption(branch, branch))
		}
	}

	// Combine: targets first, then others.
	return append(targetOpts, otherOpts...)
}
