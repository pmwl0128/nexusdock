package httpx

import (
	"testing"

	"github.com/uvwt/nexusdock/internal/config"
)

func TestCentralWorkflowTemplateManageUsesNexusRegistry(t *testing.T) {
	server := &Server{cfg: config.Config{NexusDataDir: t.TempDir()}}
	for _, id := range []string{"development.demo", "development.review"} {
		if _, err := server.publishWorkflowTemplateValue(testWorkflowTemplate(id, "1.0.0")); err != nil {
			t.Fatal(err)
		}
	}

	listed, err := server.callWorkflowTemplateManage(t.Context(), map[string]any{"action": "list", "template_status": "active"})
	if err != nil || listed["count"] != 2 {
		t.Fatalf("list=%#v err=%v", listed, err)
	}

	loaded, err := server.callWorkflowTemplateManage(t.Context(), map[string]any{"action": "get", "template_id": "development.demo"})
	if err != nil {
		t.Fatal(err)
	}
	template, ok := loaded["template"].(workflowTemplate)
	if !ok || template.ID != "development.demo" || template.Status != workflowTemplateActive {
		t.Fatalf("get=%#v", loaded)
	}

	many, err := server.callWorkflowTemplateManage(t.Context(), map[string]any{
		"action": "get_many", "template_ids": []string{"development.demo", "development.review"},
	})
	if err != nil || many["count"] != 2 || many["composition_required"] != true || many["next_required_action"] != workflowCompositionNextAction {
		t.Fatalf("get_many=%#v err=%v", many, err)
	}

	matched, err := server.callWorkflowTemplateManage(t.Context(), map[string]any{
		"action": "match", "goal": "demo development", "device": "DockMini", "type": "development",
	})
	if err != nil {
		t.Fatal(err)
	}
	if matched["action"] != "match" || matched["count"].(int) == 0 {
		t.Fatalf("match=%#v", matched)
	}
}

func TestCentralWorkflowTemplateManageSchemaHasNoNodeID(t *testing.T) {
	for _, tool := range nexusToolDefinitions() {
		if tool.Name != "workflow_template_manage" {
			continue
		}
		input := tool.InputSchema.(map[string]any)
		properties := input["properties"].(map[string]any)
		if _, exists := properties["node_id"]; exists {
			t.Fatalf("central workflow schema exposes node_id: %#v", input)
		}
		for _, name := range []string{"action", "template", "template_id", "template_ids", "template_version", "template_status", "goal", "device", "type"} {
			if _, exists := properties[name]; !exists {
				t.Fatalf("central workflow schema missing %s: %#v", name, properties)
			}
		}
		return
	}
	t.Fatal("central workflow_template_manage tool is missing")
}
