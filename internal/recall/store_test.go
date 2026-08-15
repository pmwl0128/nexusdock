package recall

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return store
}

func TestResolveRejectsTraversalAbsoluteAndGitPaths(t *testing.T) {
	store := newTestStore(t)
	bad := []string{"../x", "/abs", ".git/config", "recall/docs/projects/a/../../x.md"}
	for _, path := range bad {
		if _, err := store.resolve(path); err == nil {
			t.Fatalf("resolve(%q) unexpectedly succeeded", path)
		}
	}
	if IsAllowedRecallPath(".git/config") {
		t.Fatalf(".git/config must not be allowed")
	}

	for _, path := range []string{"cards/demo/inbox/runbook/old.md", "notes/questions/index.md", "projects/demo/project.md", "devices/dockmini.md", "ops/nexusdock.md", "inbox/old.md"} {
		if _, err := store.resolve(path); !errors.Is(err, ErrDisallowedPath) {
			t.Fatalf("expected reserved root %q to be disallowed, got %v", path, err)
		}
	}
}

func TestAllowedRecallPaths(t *testing.T) {
	allowed := []string{
		"profile.md",
		"recall/docs/devices/codingmini.md",
		"recall/docs/ops/nexusdock.md",
		"recall/docs/projects/agentdock/project.md",
		"recall/docs/projects/agentdock/environment.md",
		"recall/docs/projects/agentdock/runbooks/deploy.md",
		"recall/managed/cards/chatdock/inbox/project_trap/deploy-check.md",
		"recall/docs/inbox/20260531-recall.md",
	}
	for _, path := range allowed {
		if !IsAllowedRecallPath(path) {
			t.Fatalf("expected %q to be allowed", path)
		}
	}
	rejected := []string{"cards/demo/inbox/runbook/old.md", "notes/questions/index.md", "projects/demo/project.md", "devices/dockmini.md", "ops/nexusdock.md", "inbox/old.md", "shared/profile.md", "journal/today.md", "recall/docs/projects/agentdock/overview.md", "recall/docs/projects/agentdock/decisions/a.md", "recall/docs/projects/agentdock/runbooks/nested/a.md", "recall/managed/cards/chatdock/inbox/project_trap/nested/deploy.md", "recall/managed/notes/index.md", "recall/managed/notes/github-learning/projects/owner__repo/architecture.md"}
	for _, path := range rejected {
		if IsAllowedRecallPath(path) {
			t.Fatalf("expected %q to be rejected", path)
		}
	}
}

func TestWriteOutsideInboxRequiresConfirmation(t *testing.T) {
	store := newTestStore(t)
	_, err := store.Write(WriteRequest{Path: "profile.md", Content: "# Profile"})
	if !errors.Is(err, ErrConfirmationNeeded) {
		t.Fatalf("expected ErrConfirmationNeeded, got %v", err)
	}
	mem, err := store.Write(WriteRequest{Path: "profile.md", Content: "# Profile", Confirmed: true})
	if err != nil {
		t.Fatalf("confirmed write failed: %v", err)
	}
	if mem.Path != "profile.md" {
		t.Fatalf("unexpected path: %s", mem.Path)
	}
}

func TestMoveAndDeleteProtection(t *testing.T) {
	store := newTestStore(t)
	if _, err := store.Write(WriteRequest{Path: "recall/docs/inbox/a.md", Content: "# A"}); err != nil {
		t.Fatalf("write inbox: %v", err)
	}
	if _, err := store.Move("recall/docs/inbox/a.md", "recall/docs/projects/demo/project.md", false, false); !errors.Is(err, ErrConfirmationNeeded) {
		t.Fatalf("move without confirmation got %v", err)
	}
	if _, err := store.Move("recall/docs/inbox/a.md", "recall/docs/projects/demo/project.md", true, false); err != nil {
		t.Fatalf("move confirmed: %v", err)
	}
	if err := store.Delete("recall/docs/projects/demo/project.md", false); !errors.Is(err, ErrConfirmationNeeded) {
		t.Fatalf("delete without confirmation got %v", err)
	}
	if err := store.Delete("recall/docs/projects/demo/project.md", true); err != nil {
		t.Fatalf("delete confirmed: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(store.Root(), ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(".git", true); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("delete .git got %v", err)
	}
}

func TestFrontmatterAndDefaultPath(t *testing.T) {
	store := newTestStore(t)
	mem, err := store.Write(WriteRequest{Content: "# Hello", Type: "runbook", Project: "AgentDock"})
	if err != nil {
		t.Fatalf("default write: %v", err)
	}
	if !strings.HasPrefix(mem.Path, "recall/docs/inbox/") || !strings.HasSuffix(mem.Path, "-runbook.md") {
		t.Fatalf("unexpected default path: %s", mem.Path)
	}
	if mem.Frontmatter["type"] != "runbook" || mem.Frontmatter["project"] != "agentdock" {
		t.Fatalf("unexpected frontmatter: %#v", mem.Frontmatter)
	}
}
