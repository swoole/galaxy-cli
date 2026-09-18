package projectdiff

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"galaxy/service/container"
	"galaxy/service/instance"
	"github.com/go-git/go-git/v5"
)

func TestGitChangedFiles(t *testing.T) {
	root := t.TempDir()
	repository, err := git.PlainInit(root, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tracked.txt"), []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	worktree, err := repository.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Add("tracked.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Commit("initial", &git.CommitOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tracked.txt"), []byte("after\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "untracked.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, resolved, err := gitChangedFiles(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if resolved != "" {
		t.Fatalf("resolved base = %q, want empty", resolved)
	}
	if len(files) != 2 {
		t.Fatalf("changed files = %#v, want 2 files", files)
	}
	if files[0].Path != "tracked.txt" || files[1].Path != "untracked.txt" {
		t.Fatalf("changed files = %#v", files)
	}
}

func TestGitChangedFilesIgnoresNormalizedLineEndings(t *testing.T) {
	root := t.TempDir()
	repository, err := git.PlainInit(root, false)
	if err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(root, "unchanged.md")
	if err := os.WriteFile(filePath, []byte("first\nsecond\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	worktree, err := repository.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Add("unchanged.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Commit("initial", &git.CommitOptions{}); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("git", "-C", root, "config", "core.autocrlf", "input").CombinedOutput(); err != nil {
		t.Fatalf("configure Git line endings: %v: %s", err, output)
	}
	if err := os.WriteFile(filePath, []byte("first\r\nsecond\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Git records the worktree's mixed-line-ending stat after adding it, while
	// the normalized blob remains identical to HEAD.
	if output, err := exec.Command("git", "-C", root, "add", "unchanged.md").CombinedOutput(); err != nil {
		t.Fatalf("refresh Git index: %v: %s", err, output)
	}
	status, err := worktree.Status()
	if err != nil {
		t.Fatal(err)
	}
	if file := status["unchanged.md"]; file == nil || file.Worktree != git.Modified {
		t.Fatalf("test setup must reproduce go-git's false modification: %#v", file)
	}
	files, _, err := gitChangedFiles(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("Git-normalized unchanged file reported as modified: %#v", files)
	}
}

func TestGitChangedFilesSinceCommitIncludesCommittedAndWorktreeChanges(t *testing.T) {
	root := t.TempDir()
	repository, err := git.PlainInit(root, false)
	if err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"committed.txt": "base committed\n",
		"deleted.txt":   "base deleted\n",
		"reverted.txt":  "base reverted\n",
		"staged.txt":    "base staged\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	worktree, err := repository.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"committed.txt", "deleted.txt", "reverted.txt", "staged.txt"} {
		if _, err := worktree.Add(name); err != nil {
			t.Fatal(err)
		}
	}
	base, err := worktree.Commit("base", &git.CommitOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(root, "committed.txt"), []byte("after commit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "reverted.txt"), []byte("changed in HEAD\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"committed.txt", "reverted.txt"} {
		if _, err := worktree.Add(name); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := worktree.Commit("after base", &git.CommitOptions{}); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(root, "reverted.txt"), []byte("base reverted\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "staged.txt"), []byte("staged now\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Add("staged.txt"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "untracked.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "deleted.txt")); err != nil {
		t.Fatal(err)
	}

	files, resolved, err := gitChangedFiles(root, base.String()[:7])
	if err != nil {
		t.Fatal(err)
	}
	if resolved != base.String() {
		t.Fatalf("resolved base = %q, want %q", resolved, base.String())
	}
	got := make(map[string]string, len(files))
	for _, file := range files {
		got[file.Path] = file.GitStatus
	}
	want := map[string]string{
		"committed.txt": "M ",
		"deleted.txt":   "D ",
		"staged.txt":    "M ",
	}
	if len(got) != len(want) {
		t.Fatalf("changed files = %#v, want %#v", got, want)
	}
	for name, status := range want {
		if got[name] != status {
			t.Errorf("%s status = %q, want %q; all files: %#v", name, got[name], status, got)
		}
	}
	if _, exists := got["reverted.txt"]; exists {
		t.Fatalf("file reverted to base commit must not be included: %#v", got)
	}
	if _, exists := got["untracked.txt"]; exists {
		t.Fatalf("untracked file must follow git diff --name-only semantics and not be included: %#v", got)
	}
}

func TestGitDiffNameOnlyHandlesSpacesAndExcludesUntrackedFiles(t *testing.T) {
	root := t.TempDir()
	repository, err := git.PlainInit(root, false)
	if err != nil {
		t.Fatal(err)
	}
	tracked := "path with spaces.php"
	if err := os.WriteFile(filepath.Join(root, tracked), []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	worktree, err := repository.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Add(tracked); err != nil {
		t.Fatal(err)
	}
	base, err := worktree.Commit("base", &git.CommitOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, tracked), []byte("after\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "untracked.php"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := gitDiffNameOnly(root, base.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0] != tracked {
		t.Fatalf("files = %#v, want only %q", files, tracked)
	}
}

func TestGitChangedFilesSinceCommitRejectsUnknownRevision(t *testing.T) {
	root := t.TempDir()
	if _, err := git.PlainInit(root, false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := gitChangedFiles(root, "not-a-commit"); err == nil || !strings.Contains(err.Error(), "无法解析 commit") {
		t.Fatalf("gitChangedFiles() error = %v, want invalid commit error", err)
	}
}

func TestCompleteTreatsPositionalCommitHashAsBase(t *testing.T) {
	hash := "85c6f5bda110dd880da493761db0f5dcfc75683d"
	options := &Options{contextLines: 3}
	if err := options.complete([]string{hash}); err != nil {
		t.Fatal(err)
	}
	if options.baseCommit != hash {
		t.Fatalf("base commit = %q, want %q", options.baseCommit, hash)
	}
	if options.instanceName != "" {
		t.Fatalf("instance name = %q, want empty", options.instanceName)
	}
}

func TestCompleteKeepsInstancePositionalArgument(t *testing.T) {
	options := &Options{contextLines: 3}
	if err := options.complete([]string{"development"}); err != nil {
		t.Fatal(err)
	}
	if options.baseCommit != "" {
		t.Fatalf("base commit = %q, want empty", options.baseCommit)
	}
	if options.instanceName != "development" {
		t.Fatalf("instance name = %q, want development", options.instanceName)
	}
}

func TestCompleteKeepsInstanceWhenCommitFlagIsSet(t *testing.T) {
	options := &Options{
		baseCommit:   "85c6f5b",
		contextLines: 3,
	}
	if err := options.complete([]string{"development"}); err != nil {
		t.Fatal(err)
	}
	if options.baseCommit != "85c6f5b" {
		t.Fatalf("base commit = %q, want 85c6f5b", options.baseCommit)
	}
	if options.instanceName != "development" {
		t.Fatalf("instance name = %q, want development", options.instanceName)
	}
}

func TestUnifiedDiffUsesRuntimeAndWorkspaceLabels(t *testing.T) {
	diff, err := unifiedDiff(
		[]byte("old\n"),
		[]byte("new\n"),
		true,
		true,
		"app/config.php",
		3,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"diff --git 实例/app/config.php 工作区/app/config.php", "--- 实例/app/config.php", "+++ 工作区/app/config.php", "-old", "+new"} {
		if !strings.Contains(diff, expected) {
			t.Fatalf("diff %q does not contain %q", diff, expected)
		}
	}
}

func TestSelectRuntimeRequiresNameWhenMultipleExist(t *testing.T) {
	runtimes := []*instance.Runtime{
		{ID: 1, Name: "开发环境"},
		{ID: 2, Name: "线上环境"},
	}
	if _, err := selectRuntime(runtimes, ""); err == nil {
		t.Fatal("selectRuntime() error = nil, want explicit instance error")
	}
	selected, err := selectRuntime(runtimes, "线上环境")
	if err != nil {
		t.Fatal(err)
	}
	if selected != runtimes[1] {
		t.Fatalf("selected = %#v, want %#v", selected, runtimes[1])
	}
}

func TestSelectRuntimeDefaultsOnlyInstance(t *testing.T) {
	runtime := &instance.Runtime{ID: 8, Name: "swoole-business"}
	selected, err := selectRuntime([]*instance.Runtime{runtime}, "")
	if err != nil {
		t.Fatal(err)
	}
	if selected != runtime {
		t.Fatalf("selected = %#v, want %#v", selected, runtime)
	}
}

func TestColorizeDiffUsesGitStyleColors(t *testing.T) {
	input := "diff --git 实例/a 工作区/a\n--- 实例/a\n+++ 工作区/a\n@@ -1 +1 @@\n-old\n+new\n context\n"
	colored := colorizeDiff(input, true)
	for _, expected := range []string{
		"\x1b[31m--- 实例/a\x1b[0m",
		"\x1b[32m+++ 工作区/a\x1b[0m",
		"\x1b[36m@@ -1 +1 @@\x1b[0m",
		"\x1b[31m-old\x1b[0m",
		"\x1b[32m+new\x1b[0m",
	} {
		if !strings.Contains(colored, expected) {
			t.Fatalf("colored diff %q does not contain %q", colored, expected)
		}
	}
	if got := colorizeDiff(input, false); got != input {
		t.Fatalf("plain diff changed: %q", got)
	}
}

func TestContainerBelongsToRuntime(t *testing.T) {
	runtime := &instance.Runtime{ServiceName: "cg-1-10-web", RuntimeRef: "service-id"}
	tests := []struct {
		container *container.Container
		want      bool
	}{
		{container: &container.Container{ServiceID: "service-id"}, want: true},
		{container: &container.Container{ServiceName: "cg-1-10-web"}, want: true},
		{container: &container.Container{Name: "cg-1-10-web.1.task"}, want: true},
		{container: &container.Container{Name: "other.1.task"}, want: false},
	}
	for _, test := range tests {
		if got := containerBelongsToRuntime(test.container, runtime); got != test.want {
			t.Fatalf("containerBelongsToRuntime(%#v) = %v, want %v", test.container, got, test.want)
		}
	}
}
