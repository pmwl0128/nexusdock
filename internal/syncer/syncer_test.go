package syncer

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(out))
	}
	return strings.TrimSpace(string(out))
}

func initRepo(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	dir := filepath.Join(base, "work")
	remote := filepath.Join(base, "remote.git")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, base, "init", "--bare", "remote.git")
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.email", "nexusdock@example.invalid")
	runGit(t, dir, "config", "user.name", "NexusDock Test")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Recall\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "init")
	runGit(t, dir, "remote", "add", "origin", remote)
	runGit(t, dir, "push", "-u", "origin", "main")
	return dir
}

func TestStatusAndDiffOnNonGitRepo(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(Config{RepoDir: dir}, slog.Default())
	ctx := context.Background()
	status := mgr.Status(ctx)
	if !status.OK || status.GitRepo || status.Dirty {
		t.Fatalf("unexpected non-git status: %#v", status)
	}
	diff, err := mgr.Diff(ctx)
	if err != nil {
		t.Fatalf("Diff non-git: %v", err)
	}
	if !diff.OK || diff.GitRepo {
		t.Fatalf("unexpected non-git diff: %#v", diff)
	}
	if got, err := mgr.Discard(ctx, "", true); err != nil || got.GitRepo {
		t.Fatalf("Discard non-git status=%#v err=%v", got, err)
	}
}

func TestPushCommitsDirtyRepoAndDiscardPath(t *testing.T) {
	dir := initRepo(t)
	mgr := NewManager(Config{RepoDir: dir, CommitMessage: "recall: test sync"}, slog.Default())
	ctx := context.Background()
	if err := os.WriteFile(filepath.Join(dir, "profile.md"), []byte("# Profile\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	status := mgr.Status(ctx)
	if !status.GitRepo || !status.Dirty {
		t.Fatalf("expected dirty git repo before push: %#v", status)
	}
	if err := mgr.Push(ctx); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if got := runGit(t, dir, "log", "-1", "--pretty=%s"); got != "recall: test sync" {
		t.Fatalf("unexpected commit subject: %q", got)
	}
	status = mgr.Status(ctx)
	if status.PendingPush || status.Dirty || status.LastPushAt == "" {
		t.Fatalf("unexpected status after push: %#v", status)
	}

	if err := os.WriteFile(filepath.Join(dir, "ops.md"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Discard(ctx, "../outside", true); err == nil {
		t.Fatalf("expected discard path validation error")
	}
	if _, err := mgr.Discard(ctx, "ops.md", true); err != nil {
		t.Fatalf("discard file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "ops.md")); !os.IsNotExist(err) {
		t.Fatalf("expected untracked file to be removed, stat err=%v", err)
	}
}

func TestMarkChangedAndManualDiscardClearsPending(t *testing.T) {
	dir := initRepo(t)
	mgr := NewManager(Config{RepoDir: dir, AutoSync: false, PushDebounce: time.Millisecond}, slog.Default())
	ctx := context.Background()
	mgr.MarkChanged(ctx)
	if status := mgr.Status(ctx); !status.PendingPush {
		t.Fatalf("expected pending push after mark changed: %#v", status)
	}
	if err := os.WriteFile(filepath.Join(dir, "inbox.md"), []byte("# Inbox\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Discard(ctx, "", true); err != nil {
		t.Fatalf("discard all: %v", err)
	}
	if status := mgr.Status(ctx); status.PendingPush || status.Dirty {
		t.Fatalf("expected pending and dirty cleared: %#v", status)
	}
}

func TestRuntimeStateExcludedFromStatusPushAndDiscard(t *testing.T) {
	dir := initRepo(t)
	ctx := context.Background()
	runtimeDir := filepath.Join(dir, ".nexus")
	if err := os.MkdirAll(filepath.Join(runtimeDir, "runtime-cache", "payloads"), 0o755); err != nil {
		t.Fatal(err)
	}
	databasePath := filepath.Join(runtimeDir, "control-plane.db")
	if err := os.WriteFile(databasePath, []byte("initial database"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".nexus/control-plane.db")
	runGit(t, dir, "commit", "-m", "track legacy runtime database")
	runGit(t, dir, "push")

	if err := os.WriteFile(databasePath, []byte("live database"), 0o600); err != nil {
		t.Fatal(err)
	}
	payloadPath := filepath.Join(runtimeDir, "runtime-cache", "payloads", "payload.bin")
	if err := os.WriteFile(payloadPath, []byte("runtime payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "profile.md"), []byte("# Profile\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	mgr := NewManager(Config{RepoDir: dir, CommitMessage: "recall: safe sync"}, slog.Default())
	status := mgr.Status(ctx)
	if !status.Dirty {
		t.Fatalf("expected recall document change to be dirty: %#v", status)
	}
	diff, err := mgr.Diff(ctx)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	for _, file := range diff.Files {
		if strings.HasPrefix(file.Path, ".nexus/") {
			t.Fatalf("runtime state leaked into diff: %#v", diff.Files)
		}
	}
	if err := mgr.Push(ctx); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if files := runGit(t, dir, "show", "--pretty=", "--name-only", "HEAD"); files != "profile.md" {
		t.Fatalf("unexpected files in sync commit: %q", files)
	}

	if err := os.WriteFile(filepath.Join(dir, "inbox.md"), []byte("temporary\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Discard(ctx, "", true); err != nil {
		t.Fatalf("Discard: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "inbox.md")); !os.IsNotExist(err) {
		t.Fatalf("expected recall draft to be discarded, stat err=%v", err)
	}
	if _, err := os.Stat(databasePath); err != nil {
		t.Fatalf("runtime database was removed: %v", err)
	}
	if _, err := os.Stat(payloadPath); err != nil {
		t.Fatalf("runtime payload was removed: %v", err)
	}
	if status := mgr.Status(ctx); status.Dirty {
		t.Fatalf("runtime-only changes must not mark recall dirty: %#v", status)
	}
}

func TestPushIgnoresIgnoredRuntimeDirectory(t *testing.T) {
	dir := initRepo(t)
	ctx := context.Background()
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(".nexus/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".gitignore")
	runGit(t, dir, "commit", "-m", "ignore runtime")
	runGit(t, dir, "push")

	if err := os.MkdirAll(filepath.Join(dir, ".nexus"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".nexus", "control-plane.db"), []byte("runtime"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "profile.md"), []byte("# Profile\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	mgr := NewManager(Config{RepoDir: dir, CommitMessage: "recall: sync doc only"}, slog.Default())
	if err := mgr.Push(ctx); err != nil {
		t.Fatalf("Push with ignored runtime directory: %v", err)
	}
	if files := runGit(t, dir, "show", "--pretty=", "--name-only", "HEAD"); files != "profile.md" {
		t.Fatalf("unexpected files in sync commit: %q", files)
	}
}

func TestPushRejectsZeroByteTrackedMarkdown(t *testing.T) {
	dir := initRepo(t)
	mgr := NewManager(Config{RepoDir: dir, CommitMessage: "recall: unsafe sync"}, slog.Default())
	ctx := context.Background()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Push(ctx); err == nil || !strings.Contains(err.Error(), "zero-byte tracked markdown") {
		t.Fatalf("expected zero-byte markdown guard, got %v", err)
	}
	if got := runGit(t, dir, "log", "-1", "--pretty=%s"); got != "init" {
		t.Fatalf("unsafe sync created a commit: %q", got)
	}
	if status := mgr.Status(ctx); !status.Dirty || !status.Conflict || status.LastError == "" {
		t.Fatalf("expected dirty conflict status after rejected push: %#v", status)
	}
}

func TestPushRejectsSuspiciousBulkMarkdownDeletion(t *testing.T) {
	dir := initRepo(t)
	ctx := context.Background()
	for i := 0; i < 20; i++ {
		path := filepath.Join(dir, "recall", "docs", "bulk", fmt.Sprintf("doc-%02d.md", i))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		content := strings.Repeat(fmt.Sprintf("line %02d\n", i), 20)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGit(t, dir, "add", "recall/docs/bulk")
	runGit(t, dir, "commit", "-m", "seed docs")
	runGit(t, dir, "push")

	if err := os.RemoveAll(filepath.Join(dir, "recall", "docs", "bulk")); err != nil {
		t.Fatal(err)
	}
	mgr := NewManager(Config{RepoDir: dir, CommitMessage: "recall: unsafe bulk sync"}, slog.Default())
	if err := mgr.Push(ctx); err == nil || !strings.Contains(err.Error(), "bulk markdown") {
		t.Fatalf("expected bulk markdown deletion guard, got %v", err)
	}
	if got := runGit(t, dir, "log", "-1", "--pretty=%s"); got != "seed docs" {
		t.Fatalf("unsafe sync created a commit: %q", got)
	}
	if status := mgr.Status(ctx); !status.Dirty || !status.Conflict || status.LastError == "" {
		t.Fatalf("expected dirty conflict status after rejected push: %#v", status)
	}
}
