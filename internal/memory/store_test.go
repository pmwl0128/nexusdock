package memory

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
	bad := []string{"../x", "/abs", ".git/config", "projects/a/../../x.md"}
	for _, path := range bad {
		if _, err := store.resolve(path); err == nil {
			t.Fatalf("resolve(%q) unexpectedly succeeded", path)
		}
	}
	if IsAllowedMemoryPath(".git/config") {
		t.Fatalf(".git/config must not be allowed")
	}
}

func TestAllowedMemoryPaths(t *testing.T) {
	allowed := []string{
		"profile.md",
		"devices/codingmini.md",
		"ops/memorydock.md",
		"projects/agentdock/project.md",
		"projects/agentdock/environment.md",
		"projects/agentdock/runbooks/deploy.md",
		"notes/github-learning/index.md",
		"notes/github-learning/projects/owner__repo/architecture.md",
		"cards/chatdock/inbox/project_trap/deploy-check.md",
		"inbox/20260531-note.md",
	}
	for _, path := range allowed {
		if !IsAllowedMemoryPath(path) {
			t.Fatalf("expected %q to be allowed", path)
		}
	}
	rejected := []string{"shared/profile.md", "journal/today.md", "projects/agentdock/overview.md", "projects/agentdock/decisions/a.md", "projects/agentdock/runbooks/nested/a.md", "cards/chatdock/inbox/project_trap/nested/deploy.md", "notes/.hidden.md", "notes/github-learning/raw.bin"}
	for _, path := range rejected {
		if IsAllowedMemoryPath(path) {
			t.Fatalf("expected %q to be rejected", path)
		}
	}
}

func TestWriteNotesInfersNotesScope(t *testing.T) {
	store := newTestStore(t)
	mem, err := store.Write(WriteRequest{Path: "notes/github-learning/topics/agent.md", Content: "# Agent", Confirmed: true})
	if err != nil {
		t.Fatalf("write notes: %v", err)
	}
	if mem.Frontmatter["scope"] != string(ScopeNotes) {
		t.Fatalf("expected notes scope, got %#v", mem.Frontmatter)
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
	if _, err := store.Write(WriteRequest{Path: "inbox/a.md", Content: "# A"}); err != nil {
		t.Fatalf("write inbox: %v", err)
	}
	if _, err := store.Move("inbox/a.md", "projects/demo/project.md", false, false); !errors.Is(err, ErrConfirmationNeeded) {
		t.Fatalf("move without confirmation got %v", err)
	}
	if _, err := store.Move("inbox/a.md", "projects/demo/project.md", true, false); err != nil {
		t.Fatalf("move confirmed: %v", err)
	}
	if err := store.Delete("projects/demo/project.md", false); !errors.Is(err, ErrConfirmationNeeded) {
		t.Fatalf("delete without confirmation got %v", err)
	}
	if err := store.Delete("projects/demo/project.md", true); err != nil {
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
	mem, err := store.Write(WriteRequest{Content: "# Hello", Type: "note", Project: "AgentDock"})
	if err != nil {
		t.Fatalf("default write: %v", err)
	}
	if !strings.HasPrefix(mem.Path, "inbox/") || !strings.HasSuffix(mem.Path, "-note.md") {
		t.Fatalf("unexpected default path: %s", mem.Path)
	}
	if mem.Frontmatter["type"] != "note" || mem.Frontmatter["project"] != "agentdock" {
		t.Fatalf("unexpected frontmatter: %#v", mem.Frontmatter)
	}
}
