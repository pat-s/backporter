// Package backport provides CLI commands for backporting.
package backport

import (
	"context"

	"github.com/urfave/cli/v3"
)

// Command is the root backport command.
var Command = &cli.Command{
	Name:  "backport",
	Usage: "backport commits or PRs to target branches",
	Commands: []*cli.Command{
		prCmd,
		commitCmd,
	},
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "ci",
			Usage: "run automatic backporting in CI mode",
		},
		&cli.BoolFlag{
			Name:  "dry-run",
			Usage: "show what would be done without making changes (CI mode only)",
		},
		&cli.IntFlag{
			Name:    "pr",
			Usage:   "number of the merged PR to backport (CI mode only; detected from the CI environment if not set)",
			Sources: cli.EnvVars("BACKPORTER_PR"),
		},
	},
	Action: func(ctx context.Context, c *cli.Command) error {
		if c.Bool("ci") {
			return backportCI(ctx, c)
		}
		// No --ci flag and no subcommand: show help
		return cli.ShowSubcommandHelp(c)
	},
}

var prCmd = &cli.Command{
	Name:      "pr",
	Usage:     "backport a pull request",
	ArgsUsage: "<pr-number> <target-branch>",
	Action:    backportPR,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "dry-run",
			Usage: "show what would be done without making changes",
		},
	},
}

var commitCmd = &cli.Command{
	Name:      "commit",
	Usage:     "backport a commit",
	ArgsUsage: "<commit-sha> <target-branch>",
	Action:    backportCommit,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "dry-run",
			Usage: "show what would be done without making changes",
		},
	},
}
