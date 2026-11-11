package backport

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveTargetBranches(t *testing.T) {
	branches := []string{
		"main",
		"release/v0.9",
		"release/v0.10",
		"release/v0.2",
		"stable",
		"backport-12-to-release/v0.9",
	}

	tests := []struct {
		name     string
		targets  []string
		resolved []string
		skipped  []string
	}{
		{
			name:     "plain targets pass through",
			targets:  []string{"stable", "release-1.x"},
			resolved: []string{"stable", "release-1.x"},
		},
		{
			name:     "latest picks the numerically newest match",
			targets:  []string{"latest:release/v*"},
			resolved: []string{"release/v0.10"},
		},
		{
			name:     "latest does not match across path segments",
			targets:  []string{"latest:backport-*"},
			skipped:  []string{"latest:backport-*"},
			resolved: nil,
		},
		{
			name:     "latest without a match is skipped",
			targets:  []string{"stable", "latest:release-*"},
			resolved: []string{"stable"},
			skipped:  []string{"latest:release-*"},
		},
		{
			name:     "duplicates are removed",
			targets:  []string{"release/v0.10", "latest:release/v*"},
			resolved: []string{"release/v0.10"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved, skipped, err := resolveTargetBranches(tt.targets, branches)
			require.NoError(t, err)
			assert.Equal(t, tt.resolved, resolved)
			assert.Equal(t, tt.skipped, skipped)
		})
	}
}

func TestResolveTargetBranchesInvalidPattern(t *testing.T) {
	_, _, err := resolveTargetBranches([]string{"latest:release/["}, []string{"main"})
	assert.Error(t, err)
}

func TestCompareVersionNames(t *testing.T) {
	tests := []struct {
		a, b string
		sign int
	}{
		{"release/v0.10", "release/v0.9", 1},
		{"release/v0.9", "release/v0.10", -1},
		{"release/v1.0", "release/v0.99", 1},
		{"release/v1.0", "release/v1.0", 0},
		{"release/v01", "release/v1", 0},
		{"release/v1.0.1", "release/v1.0", 1},
		{"v17.0/forgejo", "v9.0/forgejo", 1},
		{"stable-b", "stable-a", 1},
	}

	for _, tt := range tests {
		t.Run(tt.a+" vs "+tt.b, func(t *testing.T) {
			got := compareVersionNames(tt.a, tt.b)
			switch {
			case tt.sign > 0:
				assert.Positive(t, got)
			case tt.sign < 0:
				assert.Negative(t, got)
			default:
				assert.Zero(t, got)
			}
		})
	}
}
