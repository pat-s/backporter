# backporter <img src="assets/logo.png" alt="logo" align="right" height="138" />

A CLI tool for backporting git commits and pull requests to target branches.

<p align="left">
  <a href="https://liberapay.com/pat-s"><img alt="Liberapay receiving" src="https://img.shields.io/liberapay/receives/pat-s.svg?logo=liberapay"></a>&nbsp;&nbsp;
  <a href="https://liberapay.com/pat-s/donate"><img alt="Donate using Liberapay" src="https://img.shields.io/badge/Donate-F6C915?logo=liberapay&logoColor=black"></a>
</p>

## Features

- Backport commits by SHA or pull requests by number
- Interactive mode with branch and PR selection
- CI mode for automatic backporting on PR merge
- Support for GitHub and Forgejo/Gitea forges
- Configurable target branches, including the newest branch matching a pattern
- Cache of backported commits/PRs for tracking
- Colored terminal output

## Installation

### Binary releases

Download the latest release from the [releases page](https://codefloe.com/pat-s/backporter/releases).

### Container image

```bash
docker pull codefloe.com/pat-s/backporter:latest
```

### From source

```bash
go install codefloe.com/pat-s/backporter/cmd/backporter@latest
```

### Via homebrew

```bash
brew tap pat-s/homebrew-tap https://codefloe.com/pat-s/homebrew-tap
brew install pat-s/tap/backporter
```

## Usage

### Interactive mode

Run without arguments to start the interactive wizard:

```bash
backporter
```

### Backport a commit

```bash
backporter backport commit <sha> <target-branch>

# Or directly with SHA:
backporter <sha> <target-branch>
```

### Backport a pull request

```bash
backporter backport pr <pr-number> <target-branch>

# Or directly with PR number:
backporter <pr-number> <target-branch>
```

### List backported items

```bash
backporter list
backporter list --clear  # Clear cache
```

### CI mode

Automatically backport merged PRs that have a label containing "backport":

```bash
backporter backport --ci
```

This command:

1. Finds the merged PR: from `--pr` (or `BACKPORTER_PR`), from the pull request ref of a pull request event (`CI_COMMIT_REF` in Crow and Woodpecker, `GITHUB_REF` in GitHub and Forgejo Actions), or from the message of the commit a push event ran for (`CI_COMMIT_SHA`, `GITHUB_SHA`), falling back to the newest commit on the default branch
2. Checks if the PR has any label containing "backport"
3. If so, creates backport branches and PRs for all configured target branches
4. If a backport fails, for example on a conflict, comments on the original PR with the branches that need a manual backport

The backport PR title uses the conventional commit prefix from the original PR (e.g., `feat(api):` becomes `feat(api): backport #123 to release-1.x`). If no prefix is found, it defaults to `fix:`.

#### GitHub Actions

```yaml
name: Backport

on:
  pull_request:
    types: [closed]

jobs:
  backport:
    if: github.event.pull_request.merged == true
    runs-on: ubuntu-latest
    permissions:
      contents: write
      pull-requests: write
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: pat-s/backporter@v1
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
```

#### Forgejo Actions

```yaml
name: Backport

on:
  pull_request:
    types: [closed]

jobs:
  backport:
    if: github.event.pull_request.merged == true
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
          token: ${{ secrets.FORGEJO_TOKEN }}

      - uses: codefloe.com/pat-s/backporter@v1
        with:
          token: ${{ secrets.FORGEJO_TOKEN }}
          forge-type: forgejo
          forgejo-url: https://codefloe.com
```

#### Crow CI

```yaml
# .crow/backport.yaml
when:
  - event: pull_request_merged

steps:
  backport:
    image: codefloe.com/pat-s/backporter:latest
    environment:
      FORGEJO_TOKEN:
        from_secret: forgejo_token
    commands:
      - git config --global --add safe.directory "$CI_WORKSPACE"
      - backporter backport --ci
```

#### Target only the newest release branch

Projects that only patch their newest release can let the target follow it, instead of editing the config whenever a release branch is cut:

```yaml
target_branches:
  - latest:release/v*
```

`latest:` takes a glob in which `*` does not match `/`, and picks the matching branch of the remote with the highest version, comparing runs of digits as numbers.
`backport pr` and `backport commit` without a target branch resolve `latest:` the same way, against the remote branches as of the last fetch. Interactive mode ignores `latest:` targets.

#### Action Inputs

| Input            | Description                                    | Required | Default  |
| ---------------- | ---------------------------------------------- | -------- | -------- |
| `token`          | GitHub/Forgejo token with repo permissions     | Yes      | -        |
| `dry-run`        | Show what would be done without making changes | No       | `false`  |
| `forge-type`     | Forge type: `github` or `forgejo`              | No       | `github` |
| `forgejo-url`    | Forgejo instance URL (required for forgejo)    | No       | -        |
| `default-prefix` | Default conventional commit prefix             | No       | `fix`    |

Git user configuration (`user.name` and `user.email`) is auto-detected from the forge type if not already set.

## Configuration

Configuration can be set globally (`~/.config/backporter/config.yaml`) or per-repository (`.backporter.yaml`).

```yaml
# Forge type: "github" or "forgejo"
forge_type: forgejo

# Forgejo instance URL (only for forgejo)
# forgejo_url: https://codefloe.com

# Default target branches
target_branches:
  - release-1.x
  - release-2.x
  - stable
  # The newest branch matching a glob, comparing numbers numerically
  # (release/v0.10 is newer than release/v0.9). Skipped if nothing matches.
  - latest:release/v*

# Default branch to work from
default_branch: main

# Git remote name
remote: origin

# Number of recent PRs in interactive mode
recent_pr_count: 10

# Cache settings
cache:
  enabled: true
  path: '' # Defaults to ~/.cache/backporter/history.json

# CI mode settings
ci:
  default_prefix: fix # Conventional commit prefix when not detected from PR title
```

## Authentication

Set the appropriate environment variable for your forge:

```bash
# GitHub
export GITHUB_TOKEN=<your-token>

# Forgejo/Gitea
export FORGEJO_TOKEN=<your-token>
```

## Global options

| Option         | Description                       |
| -------------- | --------------------------------- |
| `--config, -c` | Path to config file               |
| `--remote`     | Git remote name (default: origin) |
| `--log-level`  | Logging level (default: info)     |
| `--pretty`     | Pretty-printed debug output       |
| `--nocolor`    | Disable colored output            |

## License

MIT
