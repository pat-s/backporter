package backport

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codefloe.com/pat-s/backporter/pkg/forge"
)

func TestParsePRNumber(t *testing.T) {
	tests := []struct {
		name     string
		message  string
		expected int
	}{
		{
			name:     "squash merge format",
			message:  "feat: add new feature (#123)",
			expected: 123,
		},
		{
			name:     "squash merge with scope",
			message:  "fix(api): resolve bug (#456)",
			expected: 456,
		},
		{
			name:     "GitHub merge commit",
			message:  "Merge pull request #789 from user/branch",
			expected: 789,
		},
		{
			name:     "GitHub merge commit multiline",
			message:  "Merge pull request #42 from user/feature\n\nSome description here",
			expected: 42,
		},
		{
			name:     "GitLab style",
			message:  "Merge branch 'feature' into main\n\nSee merge request owner/repo!100",
			expected: 100,
		},
		{
			name:     "Forgejo/Gitea style",
			message:  "Some commit message\n\nReviewed-on: https://codeberg.org/owner/repo/pull/55",
			expected: 55,
		},
		{
			name:     "alternative merge format",
			message:  "Merge branch 'feature' #200",
			expected: 200,
		},
		{
			name:     "no PR number",
			message:  "Just a regular commit message",
			expected: 0,
		},
		{
			name:     "empty message",
			message:  "",
			expected: 0,
		},
		{
			name:     "PR number at end without parens",
			message:  "fix: something #999",
			expected: 0, // Not matched by our patterns
		},
		{
			name:     "multiple PR references takes first",
			message:  "feat: feature (#111)\n\nRelated to (#222)",
			expected: 111,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parsePRNumber(tt.message)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractConvCommitPrefix(t *testing.T) {
	tests := []struct {
		name     string
		title    string
		expected string
	}{
		{
			name:     "feat prefix",
			title:    "feat: add new feature",
			expected: "feat",
		},
		{
			name:     "fix prefix",
			title:    "fix: resolve bug",
			expected: "fix",
		},
		{
			name:     "feat with scope",
			title:    "feat(api): add endpoint",
			expected: "feat(api)",
		},
		{
			name:     "fix with scope",
			title:    "fix(auth): fix login issue",
			expected: "fix(auth)",
		},
		{
			name:     "docs prefix",
			title:    "docs: update README",
			expected: "docs",
		},
		{
			name:     "chore with scope",
			title:    "chore(deps): update dependencies",
			expected: "chore(deps)",
		},
		{
			name:     "refactor prefix",
			title:    "refactor: simplify code",
			expected: "refactor",
		},
		{
			name:     "test prefix",
			title:    "test: add unit tests",
			expected: "test",
		},
		{
			name:     "ci prefix",
			title:    "ci: update workflow",
			expected: "ci",
		},
		{
			name:     "build prefix",
			title:    "build: update Dockerfile",
			expected: "build",
		},
		{
			name:     "perf prefix",
			title:    "perf: optimize query",
			expected: "perf",
		},
		{
			name:     "style prefix",
			title:    "style: format code",
			expected: "style",
		},
		{
			name:     "revert prefix",
			title:    "revert: undo change",
			expected: "revert",
		},
		{
			name:     "no conventional commit",
			title:    "Add new feature",
			expected: "",
		},
		{
			name:     "empty title",
			title:    "",
			expected: "",
		},
		{
			name:     "wrong format - no colon",
			title:    "feat add new feature",
			expected: "",
		},
		{
			name:     "wrong format - no space after colon",
			title:    "feat:add new feature",
			expected: "",
		},
		{
			name:     "complex scope with dashes",
			title:    "feat(my-scope): add feature",
			expected: "feat(my-scope)",
		},
		{
			name:     "complex scope with underscores",
			title:    "fix(my_module): fix bug",
			expected: "fix(my_module)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractConvCommitPrefix(tt.title)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatBackportPRBody(t *testing.T) {
	mergedAt := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	tests := []struct {
		name         string
		pr           *forge.PRInfo
		targetBranch string
		contains     []string
		notContains  []string
	}{
		{
			name: "basic PR info",
			pr: &forge.PRInfo{
				Number:   123,
				Title:    "feat: add feature",
				Body:     "This is the PR description.",
				Author:   "testuser",
				MergedAt: mergedAt,
			},
			targetBranch: "release-1.x",
			contains: []string{
				"Backport of #123 to `release-1.x`",
				"**Title**: feat: add feature",
				"**Author**: @testuser",
				"**Merged**: 2024-01-15 10:30:00 UTC",
				"## Original Description",
				"This is the PR description.",
				"automatically created by [backporter]",
			},
		},
		{
			name: "PR without body",
			pr: &forge.PRInfo{
				Number:   456,
				Title:    "fix: bug fix",
				Body:     "",
				Author:   "anotheruser",
				MergedAt: mergedAt,
			},
			targetBranch: "stable",
			contains: []string{
				"Backport of #456 to `stable`",
				"**Title**: fix: bug fix",
			},
			notContains: []string{
				"## Original Description",
			},
		},
		{
			name: "PR with long body gets truncated",
			pr: &forge.PRInfo{
				Number:   789,
				Title:    "docs: update",
				Body:     string(make([]byte, 2500)), // 2500 bytes, over 2000 limit
				Author:   "user",
				MergedAt: mergedAt,
			},
			targetBranch: "main",
			contains: []string{
				"... (truncated)",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatBackportPRBody(tt.pr, tt.targetBranch)

			for _, s := range tt.contains {
				assert.Contains(t, result, s)
			}

			for _, s := range tt.notContains {
				assert.NotContains(t, result, s)
			}
		})
	}
}

func TestHasBackportLabel(t *testing.T) {
	tests := []struct {
		name     string
		labels   []string
		expected bool
	}{
		{
			name:     "exact backport label",
			labels:   []string{"backport"},
			expected: true,
		},
		{
			name:     "backport with prefix",
			labels:   []string{"needs-backport"},
			expected: true,
		},
		{
			name:     "backport with suffix",
			labels:   []string{"backport-needed"},
			expected: true,
		},
		{
			name:     "backport uppercase",
			labels:   []string{"BACKPORT"},
			expected: true,
		},
		{
			name:     "backport mixed case",
			labels:   []string{"BackPort"},
			expected: true,
		},
		{
			name:     "backport among other labels",
			labels:   []string{"bug", "backport", "priority:high"},
			expected: true,
		},
		{
			name:     "no backport label",
			labels:   []string{"bug", "enhancement", "documentation"},
			expected: false,
		},
		{
			name:     "empty labels",
			labels:   []string{},
			expected: false,
		},
		{
			name:     "nil labels",
			labels:   nil,
			expected: false,
		},
		{
			name:     "similar but not backport",
			labels:   []string{"back", "port", "back-port-ish"},
			expected: false, // "back-port-ish" does NOT contain "backport" as substring
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := &forge.PRInfo{Labels: tt.labels}
			result := pr.HasBackportLabel()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCIResultStates(t *testing.T) {
	tests := []struct {
		name    string
		result  CIResult
		success bool
		skipped bool
		hasErr  bool
	}{
		{
			name: "successful backport",
			result: CIResult{
				TargetBranch: "release-1.x",
				Success:      true,
				PRNumber:     100,
				Message:      "created backport PR #100",
			},
			success: true,
			skipped: false,
			hasErr:  false,
		},
		{
			name: "skipped - already exists",
			result: CIResult{
				TargetBranch: "release-1.x",
				Success:      true,
				Skipped:      true,
				PRNumber:     50,
				Message:      "backport PR #50 already exists",
			},
			success: true,
			skipped: true,
			hasErr:  false,
		},
		{
			name: "failed backport",
			result: CIResult{
				TargetBranch: "release-1.x",
				Success:      false,
				Error:        assert.AnError,
				Message:      "cherry-pick failed",
			},
			success: false,
			skipped: false,
			hasErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.success, tt.result.Success)
			assert.Equal(t, tt.skipped, tt.result.Skipped)
			if tt.hasErr {
				assert.NotNil(t, tt.result.Error)
			} else {
				assert.Nil(t, tt.result.Error)
			}
		})
	}
}

func TestDetectPRNumber(t *testing.T) {
	env := func(vars map[string]string) func(string) string {
		return func(name string) string { return vars[name] }
	}

	tests := []struct {
		name   string
		flag   int
		vars   map[string]string
		number int
		source string
	}{
		{
			name:   "flag wins",
			flag:   7,
			vars:   map[string]string{"CI_COMMIT_REF": "refs/pull/12/head"},
			number: 7,
			source: "--pr flag",
		},
		{
			name:   "Crow pull request event",
			vars:   map[string]string{"CI_COMMIT_REF": "refs/pull/12/head", "CI_COMMIT_SHA": "abc"},
			number: 12,
			source: "CI_COMMIT_REF",
		},
		{
			name:   "GitHub pull request event",
			vars:   map[string]string{"GITHUB_REF": "refs/pull/34/merge", "GITHUB_SHA": "abc"},
			number: 34,
			source: "GITHUB_REF",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			number, source, err := detectPRNumber(tt.flag, env(tt.vars), "origin/main")
			require.NoError(t, err)
			assert.Equal(t, tt.number, number)
			assert.Equal(t, tt.source, source)
		})
	}
}

func TestDetectPRNumberFromPushedCommit(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	for _, args := range [][]string{
		{"init", "-q"},
		{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", "fix: first (#5)"},
		{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", "feat: second (#6)"},
	} {
		require.NoError(t, exec.Command("git", args...).Run())
	}
	out, err := exec.Command("git", "rev-parse", "HEAD~1").Output()
	require.NoError(t, err)
	first := strings.TrimSpace(string(out))

	// A push event names the pushed commit, which need not be the newest one.
	vars := map[string]string{"CI_COMMIT_REF": "refs/heads/main", "CI_COMMIT_SHA": first}
	number, source, err := detectPRNumber(0, func(n string) string { return vars[n] }, "HEAD")
	require.NoError(t, err)
	assert.Equal(t, 5, number)
	assert.Equal(t, "commit CI_COMMIT_SHA", source)

	// Without CI variables, the newest commit on the fallback ref is used.
	number, _, err = detectPRNumber(0, func(string) string { return "" }, "HEAD")
	require.NoError(t, err)
	assert.Equal(t, 6, number)
}

func TestFormatFailureComment(t *testing.T) {
	assert.Empty(t, formatFailureComment([]CIResult{
		{TargetBranch: "release/v0.2", Success: true, PRNumber: 9},
		{TargetBranch: "release/v0.1", Skipped: true, Success: true, Error: errors.New("ignored")},
	}, 4))

	body := formatFailureComment([]CIResult{
		{TargetBranch: "release/v0.2", Success: true, PRNumber: 9},
		{
			TargetBranch: "release/v0.3",
			Error:        errors.New("cherry-pick has conflicts"),
			Message:      "cherry-pick has conflicts - manual backport required",
		},
	}, 4)
	assert.Contains(t, body, "- `release/v0.3`: cherry-pick has conflicts - manual backport required")
	assert.Contains(t, body, "backporter backport pr 4 release/v0.3")
	assert.NotContains(t, body, "release/v0.2")
}
