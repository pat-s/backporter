package backport

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"

	"codefloe.com/pat-s/backporter/cli/internal"
	"codefloe.com/pat-s/backporter/pkg/forge"
	"codefloe.com/pat-s/backporter/pkg/git"
	"codefloe.com/pat-s/backporter/shared/logger"
)

// CIResult represents the result of a CI backport operation for a single branch.
type CIResult struct {
	TargetBranch string
	Success      bool
	PRNumber     int  // The created backport PR number
	Skipped      bool // True if backport PR already exists
	Error        error
	Message      string
}

// convCommitPattern matches conventional commit prefixes.
var convCommitPattern = regexp.MustCompile(`^(feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert)(\([^)]+\))?:\s`)

// prNumberPatterns match PR numbers in commit messages.
var prNumberPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\(#(\d+)\)`),                // Squash merge: "feat: something (#123)"
	regexp.MustCompile(`Merge pull request #(\d+)`), // GitHub merge commit
	regexp.MustCompile(`Merge branch.*#(\d+)`),      // Alternative merge format
	regexp.MustCompile(`See merge request.*!(\d+)`), // GitLab style
	regexp.MustCompile(`Reviewed-on:.*pull/(\d+)`),  // Forgejo/Gitea style
}

func backportCI(ctx context.Context, c *cli.Command) error {
	// 1. Verify CI environment.
	if !logger.IsCI() {
		return fmt.Errorf("CI mode requires CI environment variable to be set")
	}

	dryRun := c.Bool("dry-run")

	log.Info().Msg("running in CI mode")

	// 2. Create service to get config and forge client.
	_, cfg, forgeClient, owner, repoName, err := internal.CreateServiceWithDetails(ctx, c)
	if err != nil {
		return err
	}

	// 3. Configure git user if not already set.
	configured, err := git.ConfigureUserForCI(cfg.ForgeType)
	if err != nil {
		return fmt.Errorf("failed to configure git user: %w", err)
	}
	if configured {
		log.Debug().Str("forge", cfg.ForgeType).Msg("configured git user for CI")
	}

	// 4. Fetch from remote to ensure we have the latest commits.
	log.Debug().Str("remote", cfg.Remote).Msg("fetching from remote")
	if err := git.Fetch(cfg.Remote); err != nil {
		return fmt.Errorf("failed to fetch from remote: %w", err)
	}

	// 5. Get the most recent commit on the default branch from remote.
	defaultBranch := cfg.DefaultBranch
	if defaultBranch == "" {
		defaultBranch = "main"
	}
	remoteRef := fmt.Sprintf("%s/%s", cfg.Remote, defaultBranch)

	commitMsg, err := git.GetCommitMessage(remoteRef)
	if err != nil {
		return fmt.Errorf("failed to get commit message from %s: %w", remoteRef, err)
	}

	log.Debug().Str("ref", remoteRef).Str("message", commitMsg).Msg("default branch commit message")

	// 6. Parse PR number from commit message.
	prNumber := parsePRNumber(commitMsg)
	if prNumber == 0 {
		log.Info().Msg("no PR number found in commit message, skipping backport")
		return nil
	}

	log.Info().Int("pr", prNumber).Msg("found PR number in commit")

	// 7. Fetch PR info including labels.
	prInfo, err := forgeClient.GetPR(ctx, owner, repoName, prNumber)
	if err != nil {
		return fmt.Errorf("failed to get PR #%d: %w", prNumber, err)
	}

	log.Debug().Strs("labels", prInfo.Labels).Msg("PR labels")

	// 8. Check for backport label.
	if !prInfo.HasBackportLabel() {
		log.Info().Msg("PR does not have a backport label, skipping")
		return nil
	}

	log.Info().Msg("PR has backport label, proceeding with backport")

	// 9. Get target branches from config.
	targetBranches := cfg.TargetBranches
	if len(targetBranches) == 0 {
		return fmt.Errorf("no target branches configured in config file")
	}

	log.Info().Strs("branches", targetBranches).Msg("target branches")

	// 10. Extract conventional commit prefix from PR title.
	prefix := extractConvCommitPrefix(prInfo.Title)
	if prefix == "" {
		prefix = cfg.CI.DefaultPrefix
		log.Debug().Str("prefix", prefix).Msg("using default prefix")
	} else {
		log.Debug().Str("prefix", prefix).Msg("extracted prefix from PR title")
	}

	// 11. Process each target branch.
	var results []CIResult
	for _, targetBranch := range targetBranches {
		result := processCIBackport(ctx, forgeClient, owner, repoName, prInfo, targetBranch, prefix, cfg.Remote, dryRun)
		results = append(results, result)
	}

	// 12. Output summary.
	outputCISummary(results, prNumber)

	// Check if any failed.
	for _, r := range results {
		if r.Error != nil && !r.Skipped {
			return fmt.Errorf("some backports failed")
		}
	}

	return nil
}

// parsePRNumber extracts PR number from a commit message.
func parsePRNumber(message string) int {
	for _, pattern := range prNumberPatterns {
		matches := pattern.FindStringSubmatch(message)
		if len(matches) >= 2 { //nolint:mnd
			var num int
			if _, err := fmt.Sscanf(matches[1], "%d", &num); err == nil && num > 0 {
				return num
			}
		}
	}
	return 0
}

// extractConvCommitPrefix extracts conventional commit prefix from a PR title.
// Returns the full prefix including scope if present (e.g., "feat(api)" from "feat(api): something").
func extractConvCommitPrefix(title string) string {
	matches := convCommitPattern.FindStringSubmatch(title)
	if len(matches) >= 2 { //nolint:mnd
		// matches[1] is the type (feat, fix, etc.)
		// matches[2] is the scope with parens (api) or empty
		if len(matches) >= 3 && matches[2] != "" {
			return matches[1] + matches[2]
		}
		return matches[1]
	}
	return ""
}

// processCIBackport handles backporting to a single target branch.
func processCIBackport(
	ctx context.Context,
	forgeClient forge.Forge,
	owner, repoName string,
	prInfo *forge.PRInfo,
	targetBranch string,
	prefix string,
	remote string,
	dryRun bool,
) CIResult {
	result := CIResult{
		TargetBranch: targetBranch,
	}

	branchName := fmt.Sprintf("backport-%d-to-%s", prInfo.Number, targetBranch)

	log.Info().
		Str("target", targetBranch).
		Str("branch", branchName).
		Msg("processing backport")

	// Check if backport PR already exists.
	existingPRs, err := forgeClient.ListOpenPRs(ctx, owner, repoName, forge.ListPROptions{
		Head: branchName,
	})
	if err != nil {
		log.Warn().Err(err).Msg("failed to check for existing backport PR")
		// Continue anyway - we'll fail later if there's a real problem.
	} else if len(existingPRs) > 0 {
		result.Skipped = true
		result.Success = true
		result.PRNumber = existingPRs[0].Number
		result.Message = fmt.Sprintf("backport PR #%d already exists", existingPRs[0].Number)
		log.Info().Int("pr", existingPRs[0].Number).Msg("backport PR already exists, skipping")
		return result
	}

	if dryRun {
		result.Success = true
		result.Message = "would create backport PR"
		log.Info().Msg("dry-run: would create backport branch and PR")
		return result
	}

	// Create backport branch from target branch.
	log.Debug().Str("branch", branchName).Str("from", targetBranch).Msg("creating backport branch")
	if err := git.CreateBranchFrom(branchName, remote+"/"+targetBranch); err != nil {
		result.Error = fmt.Errorf("failed to create branch: %w", err)
		result.Message = result.Error.Error()
		return result
	}

	// Checkout the new branch.
	if err := git.CheckoutBranch(branchName); err != nil {
		// Clean up the branch we created.
		_ = git.DeleteBranch(branchName)
		result.Error = fmt.Errorf("failed to checkout branch: %w", err)
		result.Message = result.Error.Error()
		return result
	}

	// Cherry-pick the merge commit directly since we're on a new branch.
	cpResult, err := git.CherryPick(prInfo.MergeCommit)
	if err != nil {
		_ = git.AbortCherryPick()
		_ = git.CheckoutBranch(targetBranch)
		_ = git.DeleteBranch(branchName)
		result.Error = fmt.Errorf("cherry-pick failed: %w", err)
		result.Message = result.Error.Error()
		return result
	}

	if cpResult.HasConflict {
		_ = git.AbortCherryPick()
		_ = git.CheckoutBranch(targetBranch)
		_ = git.DeleteBranch(branchName)
		result.Error = fmt.Errorf("cherry-pick has conflicts")
		result.Message = "cherry-pick has conflicts - manual backport required"
		return result
	}

	// Push the branch.
	log.Debug().Str("branch", branchName).Msg("pushing backport branch")
	if err := git.Push(remote, branchName); err != nil {
		_ = git.CheckoutBranch(targetBranch)
		_ = git.DeleteBranch(branchName)
		result.Error = fmt.Errorf("failed to push: %w", err)
		result.Message = result.Error.Error()
		return result
	}

	// Create the PR.
	prTitle := fmt.Sprintf("%s: backport #%d to %s", prefix, prInfo.Number, targetBranch)
	prBody := formatBackportPRBody(prInfo, targetBranch)

	log.Debug().Str("title", prTitle).Msg("creating backport PR")
	newPRNumber, err := forgeClient.CreatePR(ctx, owner, repoName, forge.CreatePROptions{
		Title: prTitle,
		Body:  prBody,
		Head:  branchName,
		Base:  targetBranch,
	})
	if err != nil {
		result.Error = fmt.Errorf("failed to create PR: %w", err)
		result.Message = result.Error.Error()
		return result
	}

	// Return to the target branch (optional cleanup).
	_ = git.CheckoutBranch(targetBranch)

	result.Success = true
	result.PRNumber = newPRNumber
	result.Message = fmt.Sprintf("created backport PR #%d", newPRNumber)

	log.Info().
		Int("pr", newPRNumber).
		Str("target", targetBranch).
		Msg("backport PR created successfully")

	return result
}

// formatBackportPRBody creates the PR body for a backport PR.
func formatBackportPRBody(originalPR *forge.PRInfo, targetBranch string) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "Backport of #%d to `%s`.\n\n", originalPR.Number, targetBranch)
	sb.WriteString("## Original PR\n\n")
	fmt.Fprintf(&sb, "- **Title**: %s\n", originalPR.Title)
	fmt.Fprintf(&sb, "- **Author**: @%s\n", originalPR.Author)
	fmt.Fprintf(&sb, "- **Merged**: %s\n", originalPR.MergedAt.Format("2006-01-02 15:04:05 UTC"))

	if originalPR.Body != "" {
		sb.WriteString("\n## Original Description\n\n")
		// Truncate very long descriptions.
		const maxBodyLen = 2000
		body := originalPR.Body
		if len(body) > maxBodyLen {
			body = body[:maxBodyLen] + "\n\n... (truncated)"
		}
		sb.WriteString(body)
		sb.WriteString("\n")
	}

	sb.WriteString("\n---\n")
	sb.WriteString("*This PR was automatically created by [backporter](https://github.com/pat-s/backporter) in CI mode.*\n")

	return sb.String()
}

const summaryLineWidth = 40

// outputCISummary outputs a summary of all backport operations.
func outputCISummary(results []CIResult, originalPR int) {
	fmt.Println()
	fmt.Printf("Backport Summary for PR #%d\n", originalPR)
	fmt.Println(strings.Repeat("=", summaryLineWidth))

	var succeeded, failed, skipped int
	for _, r := range results {
		var status string
		switch {
		case r.Skipped:
			status = "⏭️  SKIPPED"
			skipped++
		case r.Success:
			status = "✓  SUCCESS"
			succeeded++
		default:
			status = "✗  FAILED"
			failed++
		}

		fmt.Printf("%s  %s", status, r.TargetBranch)
		if r.PRNumber > 0 {
			fmt.Printf(" → PR #%d", r.PRNumber)
		}
		if r.Error != nil {
			fmt.Printf(" (%s)", r.Error.Error())
		}
		fmt.Println()
	}

	fmt.Println(strings.Repeat("-", summaryLineWidth))
	fmt.Printf("Total: %d succeeded, %d failed, %d skipped\n", succeeded, failed, skipped)
	fmt.Println()
}
