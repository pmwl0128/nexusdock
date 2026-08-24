package httpx

import (
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/uvwt/nexusdock/internal/recall"
	"github.com/uvwt/nexusdock/internal/syncer"
)

func newRecallToolTestServer(t *testing.T) (*Server, *recall.Store) {
	t.Helper()
	store, err := recall.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := syncer.NewManager(syncer.Config{RepoDir: store.Root()}, slog.Default())
	return &Server{store: store, syncer: manager}, store
}

func TestRecallWriteMarkdownDryRunAndPlanNeverPersist(t *testing.T) {
	server, store := newRecallToolTestServer(t)
	path := "recall/docs/inbox/mcp-dry-run.md"

	for _, args := range []map[string]any{
		{"target": "markdown", "action": "create", "path": path, "content": "# Dry run", "confirmed": true, "dry_run": true},
		{"target": "markdown", "action": "create", "path": path, "content": "# Unconfirmed"},
		{"target": "markdown", "action": "plan", "path": path, "content": "# Plan", "confirmed": true},
	} {
		result, err := server.callRecallWrite(t.Context(), args)
		if err != nil {
			t.Fatalf("args=%#v err=%v", args, err)
		}
		if result["dry_run"] != true || result["path"] != path {
			t.Fatalf("args=%#v result=%#v", args, result)
		}
		if _, err := store.Read(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("args=%#v persisted preview: %v", args, err)
		}
	}
}

func TestRecallWriteDryRunProtectsExistingMarkdownAndCard(t *testing.T) {
	server, store := newRecallToolTestServer(t)
	markdownPath := "recall/docs/inbox/existing.md"
	if _, err := store.Write(recall.WriteRequest{Path: markdownPath, Content: "# Existing\n\nvalue: old\n", Confirmed: true}); err != nil {
		t.Fatal(err)
	}

	patched, err := server.callRecallWrite(t.Context(), map[string]any{
		"target": "markdown", "action": "patch", "path": markdownPath,
		"old": "value: old", "new": "value: new", "confirmed": true, "dry_run": true,
	})
	if err != nil || patched["dry_run"] != true {
		t.Fatalf("patch preview=%#v err=%v", patched, err)
	}
	current, err := store.Read(markdownPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(current.Content, "value: new") {
		t.Fatalf("patch dry-run mutated markdown: %q", current.Content)
	}

	deleted, err := server.callRecallWrite(t.Context(), map[string]any{
		"target": "markdown", "action": "delete", "path": markdownPath, "confirmed": true, "dry_run": true,
	})
	if err != nil || deleted["dry_run"] != true || deleted["would_delete"] != true {
		t.Fatalf("delete preview=%#v err=%v", deleted, err)
	}
	if _, err := store.Read(markdownPath); err != nil {
		t.Fatalf("delete dry-run removed markdown: %v", err)
	}

	cardResult, err := server.callRecallWrite(t.Context(), map[string]any{
		"target": "card", "action": "create", "title": "Dry Run Card",
		"content": "A reusable self-test statement with enough detail to pass card validation.",
		"type":    "runbook", "scope": "project", "project": "agentdock",
		"confirmed": true, "dry_run": true,
	})
	if err != nil || cardResult["dry_run"] != true {
		t.Fatalf("card preview=%#v err=%v", cardResult, err)
	}
	card, ok := cardResult["card"].(map[string]any)
	if !ok {
		t.Fatalf("card preview payload=%#v", cardResult["card"])
	}
	if _, err := store.Read(card["path"].(string)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("card dry-run persisted candidate: %v", err)
	}
}
