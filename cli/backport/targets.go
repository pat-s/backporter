package backport

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"

	"codefloe.com/pat-s/backporter/pkg/git"
)

// latestPrefix marks a target that resolves to the newest branch matching a glob,
// such as "latest:release/v*".
const latestPrefix = "latest:"

// resolveConfiguredTargets resolves the configured targets against the
// branches the remote had at the last fetch, warning about targets that match
// no branch.
func resolveConfiguredTargets(targets []string, remote string) ([]string, error) {
	var branches []string
	for _, target := range targets {
		if strings.HasPrefix(target, latestPrefix) {
			var err error
			if branches, err = git.ListRemoteBranches(remote); err != nil {
				return nil, err
			}
			break
		}
	}

	resolved, skipped, err := resolveTargetBranches(targets, branches)
	if err != nil {
		return nil, err
	}
	if len(skipped) > 0 {
		log.Warn().Msgf("no branch of %s matches %s, skipping", remote, formatSkippedTargets(skipped))
	}
	return resolved, nil
}

// resolveTargetBranches turns the configured targets into branch names.
// Plain entries are used as they are. An entry "latest:<glob>" resolves to the
// newest of the given branches that match the glob, comparing the numbers in
// the names numerically, so "release/v0.10" is newer than "release/v0.9".
// An entry that matches no branch is left out and reported in skipped.
func resolveTargetBranches(targets, branches []string) (resolved, skipped []string, err error) {
	seen := make(map[string]bool)
	add := func(branch string) {
		if !seen[branch] {
			seen[branch] = true
			resolved = append(resolved, branch)
		}
	}

	for _, target := range targets {
		pattern, ok := strings.CutPrefix(target, latestPrefix)
		if !ok {
			add(target)
			continue
		}

		if _, err := path.Match(pattern, ""); err != nil {
			return nil, nil, fmt.Errorf("invalid pattern in target %q: %w", target, err)
		}

		newest := ""
		for _, branch := range branches {
			if match, _ := path.Match(pattern, branch); !match {
				continue
			}
			if newest == "" || compareVersionNames(branch, newest) > 0 {
				newest = branch
			}
		}

		if newest == "" {
			skipped = append(skipped, target)
			continue
		}
		add(newest)
	}

	return resolved, skipped, nil
}

// compareVersionNames compares two branch names, treating runs of digits as
// numbers. It returns a negative number if a sorts before b, zero if they are
// equal, and a positive number if a sorts after b.
func compareVersionNames(a, b string) int {
	for a != "" && b != "" {
		aNum, aRest := splitDigits(a)
		bNum, bRest := splitDigits(b)

		if aNum != "" && bNum != "" {
			if c := compareDigits(aNum, bNum); c != 0 {
				return c
			}
			a, b = aRest, bRest
			continue
		}

		if a[0] != b[0] {
			return int(a[0]) - int(b[0])
		}
		a, b = a[1:], b[1:]
	}

	return len(a) - len(b)
}

// splitDigits splits off the leading run of digits of s.
func splitDigits(s string) (digits, rest string) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return s[:i], s[i:]
}

// compareDigits compares two non-empty runs of digits by numeric value.
func compareDigits(a, b string) int {
	a = strings.TrimLeft(a, "0")
	b = strings.TrimLeft(b, "0")
	if len(a) != len(b) {
		return len(a) - len(b)
	}
	return strings.Compare(a, b)
}

// formatSkippedTargets describes targets that matched no branch.
func formatSkippedTargets(skipped []string) string {
	quoted := make([]string, len(skipped))
	for i, s := range skipped {
		quoted[i] = strconv.Quote(s)
	}
	return strings.Join(quoted, ", ")
}
