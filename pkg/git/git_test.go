package git

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	gitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/require"
)

func TestGitReadsRepositoryState(t *testing.T) {
	directory := t.TempDir()
	repository, err := gitlib.PlainInit(directory, false)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "README.md"), []byte("galaxy\n"), 0644))

	worktree, err := repository.Worktree()
	require.NoError(t, err)
	_, err = worktree.Add("README.md")
	require.NoError(t, err)
	hash, err := worktree.Commit("initial", &gitlib.CommitOptions{Author: &object.Signature{
		Name: "Galaxy Test", Email: "test@example.com", When: time.Unix(1, 0),
	}})
	require.NoError(t, err)
	_, err = repository.CreateTag("v1.1.1", hash, nil)
	require.NoError(t, err)

	client := &Git{
		projectRoot: directory,
		cfgPath:     filepath.Join(directory, ".git", "config"),
		repo:        repository,
	}
	branch, err := client.CurrentBranch()
	require.NoError(t, err)
	require.NotEmpty(t, branch)
	commit, err := client.CurrentCommit()
	require.NoError(t, err)
	require.Equal(t, hash, commit.Hash)
	tag, err := client.CheckTag("v1.1.1")
	require.NoError(t, err)
	require.Equal(t, hash, tag.Hash())
}
