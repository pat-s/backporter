package backport

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"

	"codefloe.com/pat-s/backporter/cli/internal"
	"codefloe.com/pat-s/backporter/cli/internal/config"
	"codefloe.com/pat-s/backporter/pkg/backport"
)

func backportCommit(ctx context.Context, c *cli.Command) error {
	if c.Args().Len() < 1 {
		return fmt.Errorf("usage: backport commit <commit-sha> [target-branch]")
	}

	sha := c.Args().Get(0)
	dryRun := c.Bool("dry-run")

	// Determine target branches.
	var targetBranches []string
	if c.Args().Len() >= 2 { //nolint:mnd
		// Target branch provided as argument.
		targetBranches = []string{c.Args().Get(1)}
	} else {
		// Try to get from config.
		cfg, err := config.GetConfig(c)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
		if len(cfg.TargetBranches) == 0 {
			return fmt.Errorf("usage: backport commit <commit-sha> <target-branch>\n       (or configure target_branches in .backporter.yaml)")
		}
		targetBranches, err = resolveConfiguredTargets(cfg.TargetBranches, cfg.Remote)
		if err != nil {
			return err
		}
		if len(targetBranches) == 0 {
			return fmt.Errorf("no configured target branch exists")
		}
	}

	service, err := internal.CreateService(ctx, c)
	if err != nil {
		return err
	}

	// Backport to each target branch.
	var lastErr error
	for _, targetBranch := range targetBranches {
		log.Info().Str("branch", targetBranch).Str("sha", sha).Msg("backporting commit")

		opts := backport.BackportOptions{
			TargetBranch: targetBranch,
			DryRun:       dryRun,
		}

		result, err := service.BackportCommit(ctx, sha, opts)
		if err != nil {
			log.Error().Err(err).Str("branch", targetBranch).Msg("backport failed")
			lastErr = err
			continue
		}

		if err := handleBackportResult(result); err != nil {
			lastErr = err
		}
	}

	return lastErr
}
