package forge

import (
	"context"
	"fmt"

	"github.com/google/go-github/v93/github"
)

// GitHub implements the Forge interface for GitHub.
type GitHub struct {
	client *github.Client
}

// NewGitHub creates a new GitHub forge client.
func NewGitHub(token string) (*GitHub, error) {
	var opts []github.ClientOptionsFunc
	if token != "" {
		opts = append(opts, github.WithAuthToken(token))
	}

	client, err := github.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create GitHub client: %w", err)
	}

	return &GitHub{client: client}, nil
}

// Name returns the name of the forge.
func (g *GitHub) Name() string {
	return "github"
}

// GetPR retrieves information about a pull request by number.
func (g *GitHub) GetPR(ctx context.Context, owner, repo string, number int) (*PRInfo, error) {
	pr, _, err := g.client.PullRequests.Get(ctx, owner, repo, number)
	if err != nil {
		return nil, fmt.Errorf("failed to get PR #%d: %w", number, err)
	}

	if !pr.GetMerged() {
		return nil, fmt.Errorf("PR #%d is not merged", number)
	}

	// Check if it was squash merged by looking at the merge commit.
	mergeCommit, _, err := g.client.Repositories.GetCommit(ctx, owner, repo, pr.GetMergeCommitSHA(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get merge commit: %w", err)
	}

	squashed := len(mergeCommit.Parents) == 1

	// Extract labels.
	labels := make([]string, len(pr.Labels))
	for i, label := range pr.Labels {
		labels[i] = label.GetName()
	}

	info := &PRInfo{
		Number:      pr.GetNumber(),
		Title:       pr.GetTitle(),
		Body:        pr.GetBody(),
		State:       pr.GetState(),
		MergeCommit: pr.GetMergeCommitSHA(),
		HeadSHA:     pr.GetHead().GetSHA(),
		BaseBranch:  pr.GetBase().GetRef(),
		HeadBranch:  pr.GetHead().GetRef(),
		Merged:      pr.GetMerged(),
		Squashed:    squashed,
		Author:      pr.GetUser().GetLogin(),
		MergedAt:    pr.GetMergedAt().Time,
		Labels:      labels,
	}

	return info, nil
}

// GetCommit retrieves information about a commit by SHA.
func (g *GitHub) GetCommit(ctx context.Context, owner, repo, sha string) (*CommitInfo, error) {
	commit, _, err := g.client.Repositories.GetCommit(ctx, owner, repo, sha, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get commit %s: %w", sha, err)
	}

	parents := make([]string, len(commit.Parents))
	for i, parent := range commit.Parents {
		parents[i] = parent.GetSHA()
	}

	info := &CommitInfo{
		SHA:       commit.GetSHA(),
		Message:   commit.GetCommit().GetMessage(),
		Author:    commit.GetCommit().GetAuthor().GetName(),
		Email:     commit.GetCommit().GetAuthor().GetEmail(),
		Timestamp: commit.GetCommit().GetAuthor().GetDate().Time,
		Parents:   parents,
	}

	return info, nil
}

// ListRecentPRs lists recently merged PRs.
func (g *GitHub) ListRecentPRs(ctx context.Context, owner, repo string, limit int) ([]*PRInfo, error) {
	opts := &github.PullRequestListOptions{
		State:     "closed",
		Sort:      "updated",
		Direction: "desc",
		ListOptions: github.ListOptions{
			PerPage: limit,
		},
	}

	prs, _, err := g.client.PullRequests.List(ctx, owner, repo, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list PRs: %w", err)
	}

	var result []*PRInfo
	for _, pr := range prs {
		if !pr.GetMerged() {
			continue
		}

		info := &PRInfo{
			Number:      pr.GetNumber(),
			Title:       pr.GetTitle(),
			State:       pr.GetState(),
			MergeCommit: pr.GetMergeCommitSHA(),
			HeadSHA:     pr.GetHead().GetSHA(),
			BaseBranch:  pr.GetBase().GetRef(),
			HeadBranch:  pr.GetHead().GetRef(),
			Merged:      pr.GetMerged(),
			Author:      pr.GetUser().GetLogin(),
			MergedAt:    pr.GetMergedAt().Time,
		}
		result = append(result, info)

		if len(result) >= limit {
			break
		}
	}

	return result, nil
}

// CreatePR creates a new pull request and returns its number.
func (g *GitHub) CreatePR(ctx context.Context, owner, repo string, opts CreatePROptions) (int, error) {
	newPR := github.CreatePullRequest{
		Title: new(opts.Title),
		Body:  new(opts.Body),
		Head:  opts.Head,
		Base:  opts.Base,
	}

	pr, _, err := g.client.PullRequests.Create(ctx, owner, repo, newPR)
	if err != nil {
		return 0, fmt.Errorf("failed to create PR: %w", err)
	}

	return pr.GetNumber(), nil
}

// ListOpenPRs lists open PRs, optionally filtered by head branch.
func (g *GitHub) ListOpenPRs(ctx context.Context, owner, repo string, opts ListPROptions) ([]*PRInfo, error) {
	const maxPRsPerPage = 100
	listOpts := &github.PullRequestListOptions{
		State: "open",
		ListOptions: github.ListOptions{
			PerPage: maxPRsPerPage,
		},
	}

	if opts.Head != "" {
		// GitHub requires head to be in format "owner:branch" or just "branch".
		listOpts.Head = opts.Head
	}

	prs, _, err := g.client.PullRequests.List(ctx, owner, repo, listOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to list open PRs: %w", err)
	}

	var result []*PRInfo
	for _, pr := range prs {
		// Extract labels.
		labels := make([]string, len(pr.Labels))
		for i, label := range pr.Labels {
			labels[i] = label.GetName()
		}

		info := &PRInfo{
			Number:     pr.GetNumber(),
			Title:      pr.GetTitle(),
			Body:       pr.GetBody(),
			State:      pr.GetState(),
			HeadSHA:    pr.GetHead().GetSHA(),
			BaseBranch: pr.GetBase().GetRef(),
			HeadBranch: pr.GetHead().GetRef(),
			Merged:     pr.GetMerged(),
			Author:     pr.GetUser().GetLogin(),
			Labels:     labels,
		}
		result = append(result, info)
	}

	return result, nil
}

// CommentOnPR posts a comment on a pull request.
func (g *GitHub) CommentOnPR(ctx context.Context, owner, repo string, number int, body string) error {
	comment := github.IssueCommentRequest{Body: body}
	if _, _, err := g.client.Issues.CreateComment(ctx, owner, repo, number, comment); err != nil {
		return fmt.Errorf("failed to comment on PR #%d: %w", number, err)
	}
	return nil
}
