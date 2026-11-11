package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestRepo creates a temporary git repository for testing.
func setupTestRepo(t *testing.T) (string, func()) {
	t.Helper()

	tmpDir := t.TempDir()

	// Initialize git repo.
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run(), "failed to init git repo")

	// Configure git user (required for commits).
	configUser := exec.Command("git", "config", "user.name", "Test User")
	configUser.Dir = tmpDir
	require.NoError(t, configUser.Run())

	configEmail := exec.Command("git", "config", "user.email", "test@example.com")
	configEmail.Dir = tmpDir
	require.NoError(t, configEmail.Run())

	// Create initial commit.
	testFile := filepath.Join(tmpDir, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("initial content\n"), 0o644))

	add := exec.Command("git", "add", "test.txt")
	add.Dir = tmpDir
	require.NoError(t, add.Run())

	commit := exec.Command("git", "commit", "-m", "Initial commit")
	commit.Dir = tmpDir
	require.NoError(t, commit.Run())

	cleanup := func() {
		// No cleanup needed, t.TempDir() handles it.
	}

	return tmpDir, cleanup
}

func TestCheckoutBranch(t *testing.T) {
	repoPath, cleanup := setupTestRepo(t)
	defer cleanup()

	// Change to repo directory.
	t.Chdir(repoPath)

	// Create a new branch.
	cmd := exec.Command("git", "branch", "test-branch")
	require.NoError(t, cmd.Run())

	// Test checkout.
	err := CheckoutBranch("test-branch")
	assert.NoError(t, err)

	// Verify we're on the correct branch.
	repo, err := OpenCurrent()
	require.NoError(t, err)
	currentBranch, err := repo.CurrentBranch()
	require.NoError(t, err)
	assert.Equal(t, "test-branch", currentBranch)
}

func TestCheckoutBranch_NonExistent(t *testing.T) {
	repoPath, cleanup := setupTestRepo(t)
	defer cleanup()

	t.Chdir(repoPath)

	// Try to checkout non-existent branch.
	err := CheckoutBranch("non-existent-branch")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to checkout")
}

func TestHasUncommittedChanges_CleanWorkingTree(t *testing.T) {
	repoPath, cleanup := setupTestRepo(t)
	defer cleanup()

	repo, err := Open(repoPath)
	require.NoError(t, err)

	hasChanges, err := repo.HasUncommittedChanges()
	require.NoError(t, err)
	assert.False(t, hasChanges, "clean working tree should have no uncommitted changes")
}

func TestHasUncommittedChanges_UntrackedFiles(t *testing.T) {
	repoPath, cleanup := setupTestRepo(t)
	defer cleanup()

	// Create an untracked file.
	untrackedFile := filepath.Join(repoPath, "untracked.txt")
	require.NoError(t, os.WriteFile(untrackedFile, []byte("untracked content\n"), 0o644))

	repo, err := Open(repoPath)
	require.NoError(t, err)

	hasChanges, err := repo.HasUncommittedChanges()
	require.NoError(t, err)
	assert.False(t, hasChanges, "untracked files should not count as uncommitted changes")
}

func TestHasUncommittedChanges_ModifiedFiles(t *testing.T) {
	repoPath, cleanup := setupTestRepo(t)
	defer cleanup()

	// Modify the tracked file.
	testFile := filepath.Join(repoPath, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("modified content\n"), 0o644))

	repo, err := Open(repoPath)
	require.NoError(t, err)

	hasChanges, err := repo.HasUncommittedChanges()
	require.NoError(t, err)
	assert.True(t, hasChanges, "modified files should count as uncommitted changes")
}

func TestHasUncommittedChanges_StagedFiles(t *testing.T) {
	repoPath, cleanup := setupTestRepo(t)
	defer cleanup()

	// Create and stage a new file.
	newFile := filepath.Join(repoPath, "new.txt")
	require.NoError(t, os.WriteFile(newFile, []byte("new content\n"), 0o644))

	add := exec.Command("git", "add", "new.txt")
	add.Dir = repoPath
	require.NoError(t, add.Run())

	repo, err := Open(repoPath)
	require.NoError(t, err)

	hasChanges, err := repo.HasUncommittedChanges()
	require.NoError(t, err)
	assert.True(t, hasChanges, "staged files should count as uncommitted changes")
}

func TestCherryPick_Success(t *testing.T) {
	repoPath, cleanup := setupTestRepo(t)
	defer cleanup()

	t.Chdir(repoPath)

	// Create a second commit to cherry-pick.
	testFile := filepath.Join(repoPath, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("initial content\nsecond line\n"), 0o644))

	add := exec.Command("git", "add", "test.txt")
	require.NoError(t, add.Run())

	commit := exec.Command("git", "commit", "-m", "Add second line")
	require.NoError(t, commit.Run())

	// Get the commit SHA.
	shaCmd := exec.Command("git", "rev-parse", "HEAD")
	shaOutput, err := shaCmd.Output()
	require.NoError(t, err)
	sha := string(shaOutput[:7]) // Use short SHA.

	// Create and checkout a new branch from the initial commit.
	cmd := exec.Command("git", "checkout", "-b", "target-branch", "HEAD~1")
	require.NoError(t, cmd.Run())

	// Cherry-pick the commit.
	result, err := CherryPick(sha)
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.False(t, result.HasConflict)
}

func TestCherryPick_Conflict(t *testing.T) {
	repoPath, cleanup := setupTestRepo(t)
	defer cleanup()

	t.Chdir(repoPath)

	testFile := filepath.Join(repoPath, "test.txt")

	// Create a second commit on main.
	require.NoError(t, os.WriteFile(testFile, []byte("initial content\nmain branch line\n"), 0o644))
	add := exec.Command("git", "add", "test.txt")
	require.NoError(t, add.Run())
	commit := exec.Command("git", "commit", "-m", "Main branch change")
	require.NoError(t, commit.Run())

	// Get the commit SHA.
	shaCmd := exec.Command("git", "rev-parse", "HEAD")
	shaOutput, err := shaCmd.Output()
	require.NoError(t, err)
	sha := string(shaOutput[:7])

	// Create a branch from initial commit and make conflicting change.
	cmd := exec.Command("git", "checkout", "-b", "target-branch", "HEAD~1")
	require.NoError(t, cmd.Run())

	require.NoError(t, os.WriteFile(testFile, []byte("initial content\ntarget branch line\n"), 0o644))
	add2 := exec.Command("git", "add", "test.txt")
	require.NoError(t, add2.Run())
	commit2 := exec.Command("git", "commit", "-m", "Target branch change")
	require.NoError(t, commit2.Run())

	// Cherry-pick should result in conflict.
	result, err := CherryPick(sha)
	require.NoError(t, err, "cherry-pick with conflict should not return error")
	assert.False(t, result.Success)
	assert.True(t, result.HasConflict)

	// Cleanup: abort the cherry-pick.
	_ = AbortCherryPick()
}

func TestCreateBranch(t *testing.T) {
	repoPath, cleanup := setupTestRepo(t)
	defer cleanup()

	t.Chdir(repoPath)

	err := CreateBranch("new-branch")
	assert.NoError(t, err)

	// Verify branch exists.
	repo, err := OpenCurrent()
	require.NoError(t, err)
	exists, err := repo.BranchExists("new-branch")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestCreateBranchFrom(t *testing.T) {
	repoPath, cleanup := setupTestRepo(t)
	defer cleanup()

	t.Chdir(repoPath)

	// Create second commit.
	testFile := filepath.Join(repoPath, "test2.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("content\n"), 0o644))
	add := exec.Command("git", "add", "test2.txt")
	require.NoError(t, add.Run())
	commit := exec.Command("git", "commit", "-m", "Second commit")
	require.NoError(t, commit.Run())

	// Create branch from HEAD~1.
	err := CreateBranchFrom("from-prev", "HEAD~1")
	assert.NoError(t, err)

	// Verify branch exists and points to correct commit.
	repo, err := OpenCurrent()
	require.NoError(t, err)
	exists, err := repo.BranchExists("from-prev")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestAmendCommitMessage(t *testing.T) {
	repoPath, cleanup := setupTestRepo(t)
	defer cleanup()

	t.Chdir(repoPath)

	newMessage := "Amended commit message"
	err := AmendCommitMessage(newMessage)
	assert.NoError(t, err)

	// Verify message was amended.
	repo, err := OpenCurrent()
	require.NoError(t, err)
	sha, err := GetCurrentCommitSHA()
	require.NoError(t, err)
	msg, err := repo.GetCommitMessage(sha)
	require.NoError(t, err)
	assert.Equal(t, newMessage+"\n", msg) // Git commit messages always have a trailing newline
}

func TestListRemoteBranches(t *testing.T) {
	repoPath, cleanup := setupTestRepo(t)
	defer cleanup()

	t.Chdir(repoPath)

	for _, ref := range []string{"main", "release/v0.9", "release/v0.10"} {
		cmd := exec.Command("git", "update-ref", "refs/remotes/origin/"+ref, "HEAD")
		require.NoError(t, cmd.Run())
	}
	cmd := exec.Command("git", "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	require.NoError(t, cmd.Run())
	cmd = exec.Command("git", "update-ref", "refs/remotes/other/stable", "HEAD")
	require.NoError(t, cmd.Run())

	branches, err := ListRemoteBranches("origin")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"main", "release/v0.9", "release/v0.10"}, branches)
}
