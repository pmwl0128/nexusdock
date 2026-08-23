package httpx

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/uvwt/nexusdock/internal/recall"
	"github.com/uvwt/nexusdock/internal/syncer"
)

func TestNodeInputSchemaRequiresNodeID(t *testing.T) {
	schema := nodeInputSchema(map[string]any{
		"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}, "required": []any{"path"},
	})
	properties := schema["properties"].(map[string]any)
	if _, ok := properties["node_id"]; !ok {
		t.Fatal("node_id property is missing")
	}
	required := schema["required"].([]any)
	if len(required) != 2 || required[1] != "node_id" {
		t.Fatalf("required = %#v", required)
	}
}

func TestRecallUpdateFactPreviewsAndWrites(t *testing.T) {
	store, err := recall.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Write(recall.WriteRequest{Path: "profile.md", Content: "# Profile\n\neditor: old\n", Confirmed: true}); err != nil {
		t.Fatal(err)
	}
	manager := syncer.NewManager(syncer.Config{RepoDir: store.Root()}, slog.Default())
	server := &Server{store: store, syncer: manager}
	preview, err := server.updateRecallFacts(t.Context(), "profile.md", map[string]any{"key": "editor", "value": "new"})
	if err != nil || preview["dry_run"] != true {
		t.Fatalf("preview=%#v err=%v", preview, err)
	}
	unchanged, _ := store.Read("profile.md")
	if strings.Contains(unchanged.Content, "editor: new") {
		t.Fatal("preview mutated Recall")
	}
	result, err := server.updateRecallFacts(t.Context(), "profile.md", map[string]any{"key": "editor", "value": "new", "confirmed": true})
	if err != nil || result["written"] != true {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	updated, _ := store.Read("profile.md")
	if !strings.Contains(updated.Content, "editor: new") {
		t.Fatalf("updated content = %q", updated.Content)
	}
}
