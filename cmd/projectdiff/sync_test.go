package projectdiff

import (
	"errors"
	"galaxy/service/instance"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildSyncItemsSkipsDeletedFiles(t *testing.T) {
	root := t.TempDir()
	existing := filepath.Join(root, "app", "existing.php")
	if err := os.MkdirAll(filepath.Dir(existing), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(existing, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	items, pending, err := buildSyncItems(root, "/var/www/html", []changedFile{
		{Path: "app/existing.php", GitStatus: " M"},
		{Path: "app/deleted.php", GitStatus: " D"},
	}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if pending != 1 || len(items) != 2 {
		t.Fatalf("pending=%d items=%#v", pending, items)
	}
	if items[0].skip || items[0].remote != "/var/www/html/app/existing.php" {
		t.Fatalf("unexpected existing item: %#v", items[0])
	}
	if !items[1].skip || items[1].Result != "跳过：本地已删除" {
		t.Fatalf("unexpected deleted item: %#v", items[1])
	}
}

func TestBuildSyncItemsDeletesOnlyGitDeletionsWhenEnabled(t *testing.T) {
	root := t.TempDir()
	items, pending, err := buildSyncItems(root, "/app", []changedFile{
		{Path: "deleted.php", GitStatus: " D"},
		{Path: "staged-deletion.php", GitStatus: "D "},
		{Path: "missing.php", GitStatus: " M"},
	}, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if pending != 2 || len(items) != 3 {
		t.Fatalf("pending=%d items=%#v", pending, items)
	}
	for _, item := range items[:2] {
		if item.skip || !item.delete || item.Result != "待删除" {
			t.Errorf("deleted file = %#v", item)
		}
	}
	if !items[2].skip || items[2].delete {
		t.Errorf("missing non-deleted file = %#v", items[2])
	}
}

func TestBuildSyncItemsDoesNotDeleteIgnoredFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, syncIgnoreFile), []byte("secrets/**\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rules, err := loadSyncIgnoreRules(root)
	if err != nil {
		t.Fatal(err)
	}
	items, pending, err := buildSyncItems(root, "/app", []changedFile{
		{Path: "secrets/key.txt", GitStatus: " D"},
		{Path: syncIgnoreFile, GitStatus: " D"},
	}, rules, true)
	if err != nil {
		t.Fatal(err)
	}
	if pending != 0 {
		t.Fatalf("pending = %d, want 0", pending)
	}
	for _, item := range items {
		if !item.skip || item.delete || item.Result != "跳过：.galaxyignore" {
			t.Errorf("ignored deletion = %#v", item)
		}
	}
}

func TestBuildSyncItemsAppliesGalaxyIgnoreRules(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{
		".galaxyignore":        "# 本地敏感配置\n.env\nconfig/database*.php\nsecrets/**\n!secrets/example.env\n",
		".env":                 "DB_PASSWORD=secret\n",
		"config/database.php":  "<?php return ['password' => 'secret'];\n",
		"secrets/account.json": "{\"password\":\"secret\"}\n",
		"secrets/example.env":  "DB_PASSWORD=example\n",
		"app/Service.php":      "<?php\n",
	} {
		filePath := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rules, err := loadSyncIgnoreRules(root)
	if err != nil {
		t.Fatal(err)
	}
	if !rules.loaded {
		t.Fatal("ignore rules must be marked as loaded")
	}
	changed := []changedFile{
		{Path: ".galaxyignore", GitStatus: " M"},
		{Path: ".env", GitStatus: " M"},
		{Path: "config/database.php", GitStatus: " M"},
		{Path: "secrets/account.json", GitStatus: " M"},
		{Path: "secrets/example.env", GitStatus: " M"},
		{Path: "app/Service.php", GitStatus: " M"},
	}
	items, pending, err := buildSyncItems(root, "/app", changed, rules, false)
	if err != nil {
		t.Fatal(err)
	}
	if pending != 2 {
		t.Fatalf("pending = %d, want 2; items=%#v", pending, items)
	}
	results := make(map[string]*syncItem, len(items))
	for _, item := range items {
		results[item.Path] = item
	}
	for _, ignored := range []string{".galaxyignore", ".env", "config/database.php", "secrets/account.json"} {
		if item := results[ignored]; item == nil || !item.skip || item.Result != "跳过：.galaxyignore" {
			t.Errorf("%s must be ignored, got %#v", ignored, item)
		}
	}
	for _, included := range []string{"secrets/example.env", "app/Service.php"} {
		if item := results[included]; item == nil || item.skip || item.Result != "待同步" {
			t.Errorf("%s must be synchronized, got %#v", included, item)
		}
	}
}

func TestCheckSyncTargetDirectoriesStopsBeforeWriting(t *testing.T) {
	items := []*syncItem{
		{Path: "app/first.php", remote: "/var/www/html/app/first.php"},
		{Path: "app/ignored.php", remote: "/var/www/html/app/ignored.php", skip: true},
		{Path: "app/conflict.php", remote: "/var/www/html/app/conflict.php"},
		{Path: "app/later.php", remote: "/var/www/html/app/later.php"},
	}
	var checked []string
	err := checkSyncTargetDirectories(items, func(remotePath string) (bool, error) {
		checked = append(checked, remotePath)
		return remotePath == "/var/www/html/app/conflict.php", nil
	})
	if err == nil || !strings.Contains(err.Error(), "app/conflict.php") || !strings.Contains(err.Error(), "手动处理") {
		t.Fatalf("conflict error = %v", err)
	}
	if len(checked) != 2 || checked[0] != items[0].remote || checked[1] != items[2].remote {
		t.Fatalf("checked paths = %#v", checked)
	}
}

func TestCheckSyncTargetDirectoriesRejectsDeletedDirectory(t *testing.T) {
	item := &syncItem{Path: "old/file.php", remote: "/app/old/file.php", delete: true}
	err := checkSyncTargetDirectories([]*syncItem{item}, func(remotePath string) (bool, error) {
		if remotePath != item.remote {
			t.Fatalf("checked %q, want %q", remotePath, item.remote)
		}
		return true, nil
	})
	if err == nil || !strings.Contains(err.Error(), "拒绝删除") || !strings.Contains(err.Error(), "是目录") {
		t.Fatalf("directory deletion error = %v", err)
	}
}

func TestBuildSyncItemsRejectsPathsOutsideProject(t *testing.T) {
	for _, filePath := range []string{"../outside", "/absolute", "sub/../../outside"} {
		_, _, err := buildSyncItems(t.TempDir(), "/app", []changedFile{{Path: filePath, GitStatus: " D"}}, nil, true)
		if err == nil || !strings.Contains(err.Error(), "无效的 Git 文件路径") {
			t.Errorf("path %q error = %v", filePath, err)
		}
	}
}

func TestCheckSyncTargetDirectoriesFailsClosedOnCheckError(t *testing.T) {
	checkError := errors.New("remote check failed")
	err := checkSyncTargetDirectories([]*syncItem{
		{Path: "app/file.php", remote: "/var/www/html/app/file.php"},
	}, func(string) (bool, error) {
		return false, checkError
	})
	if !errors.Is(err, checkError) {
		t.Fatalf("check error = %v", err)
	}
}

func TestLoadSyncIgnoreRulesAllowsMissingFile(t *testing.T) {
	rules, err := loadSyncIgnoreRules(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if rules.loaded || rules.matches("app/Service.php") {
		t.Fatalf("missing ignore file must not exclude regular files: %#v", rules)
	}
	if !rules.matches(syncIgnoreFile) {
		t.Fatal(".galaxyignore itself must never be synchronized")
	}
}

func TestSyncCompleteRequiresAbsoluteRemoteRoot(t *testing.T) {
	options := &SyncOptions{remoteRoot: "var/www/html"}
	if err := options.complete(nil); err == nil {
		t.Fatal("complete() error = nil, want absolute remote root error")
	}
}

func TestSyncCompleteTreatsPositionalCommitHashAsBase(t *testing.T) {
	hash := "85c6f5bda110dd880da493761db0f5dcfc75683d"
	options := &SyncOptions{}
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

func TestSyncCompleteKeepsInstancePositionalArgument(t *testing.T) {
	options := &SyncOptions{}
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

func TestSyncCompleteKeepsInstanceWhenCommitFlagIsSet(t *testing.T) {
	options := &SyncOptions{baseCommit: "85c6f5b"}
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

func TestSelectRuntimeUsesSyncCommandInMultipleInstanceError(t *testing.T) {
	_, err := selectRuntimeForCommand([]*instance.Runtime{
		{ID: 1, Name: "development"},
		{ID: 2, Name: "production"},
	}, "", "sync")
	if err == nil || !strings.Contains(err.Error(), "galaxy sync <INSTANCE>") {
		t.Fatalf("error = %v, want sync command hint", err)
	}
}
