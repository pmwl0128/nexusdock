package syncer

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPushRejectsTrackedPrivateNotePlaintextOrKeys(t *testing.T) {
	for _, rel := range []string{
		"private-notes/notes/recovery/secret.md",
		"private-notes/.keys/private-notes-age-identity.txt",
	} {
		t.Run(rel, func(t *testing.T) {
			dir := initRepo(t)
			path := filepath.Join(dir, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("must never be pushed\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			runGit(t, dir, "add", "-f", "--", rel)
			runGit(t, dir, "commit", "-m", "seed unsafe private file")
			if err := os.WriteFile(filepath.Join(dir, "profile.md"), []byte("# Dirty\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			mgr := NewManager(Config{RepoDir: dir, CommitMessage: "recall: must reject"}, slog.Default())
			err := mgr.Push(context.Background())
			if err == nil || !strings.Contains(err.Error(), "tracked or non-ignored private note plaintext or keys") {
				t.Fatalf("Push error = %v, want private note tracking guard", err)
			}
			if got := runGit(t, dir, "ls-remote", "origin", "refs/heads/main"); strings.Contains(got, runGit(t, dir, "rev-parse", "HEAD")) {
				t.Fatal("unsafe local commit unexpectedly reached remote")
			}
		})
	}
}

func TestPushRejectsNonIgnoredUntrackedPrivateNotePlaintextOrKeys(t *testing.T) {
	for _, test := range []struct {
		name      string
		rel       string
		gitignore string
	}{
		{name: "missing ignore rules", rel: "private-notes/notes/recovery/secret.md"},
		{name: "negated plaintext rule", rel: "private-notes/notes/recovery/secret.md", gitignore: "private-notes/notes/*\n!private-notes/notes/recovery/\nprivate-notes/notes/recovery/*\n!private-notes/notes/recovery/secret.md\n"},
		{name: "non-ignored key", rel: "private-notes/.keys/private-notes-age-identity.txt"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := initRepo(t)
			if test.gitignore != "" {
				if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(test.gitignore), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			path := filepath.Join(dir, filepath.FromSlash(test.rel))
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("must never be pushed\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "profile.md"), []byte("# Dirty\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			mgr := NewManager(Config{RepoDir: dir, CommitMessage: "recall: must reject"}, slog.Default())
			err := mgr.Push(context.Background())
			if err == nil || !strings.Contains(err.Error(), "tracked or non-ignored private note plaintext or keys") {
				t.Fatalf("Push error = %v, want non-ignored private note guard", err)
			}
			if staged := runGit(t, dir, "diff", "--cached", "--name-only"); strings.Contains(staged, test.rel) {
				t.Fatalf("unsafe private path was staged: %s", staged)
			}
			if remoteFiles := runGit(t, dir, "ls-tree", "-r", "--name-only", "origin/main"); strings.Contains(remoteFiles, test.rel) {
				t.Fatalf("unsafe private path reached remote: %s", remoteFiles)
			}
		})
	}
}
