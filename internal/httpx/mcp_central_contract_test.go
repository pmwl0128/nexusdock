package httpx

import (
	"reflect"
	"sort"
	"testing"
)

func TestCentralToolDefinitionsCarryOutputSchemaAndSafetyAnnotations(t *testing.T) {
	expected := map[string]struct {
		readOnly    bool
		destructive bool
		openWorld   bool
	}{
		"agentdock_context":        {readOnly: true},
		"workflow_template_manage": {destructive: true},
		"recall_bootstrap":         {readOnly: true},
		"recall_search":            {readOnly: true},
		"recall_read":              {readOnly: true},
		"recall_write":             {destructive: true},
		"recall_maintain":          {destructive: true},
		"private_note_manage":      {destructive: true},
	}

	seen := make(map[string]bool, len(expected))
	for _, tool := range nexusToolDefinitions() {
		want, ok := expected[tool.Name]
		if !ok {
			continue
		}
		seen[tool.Name] = true
		if tool.OutputSchema == nil {
			t.Fatalf("%s missing outputSchema", tool.Name)
		}
		output, ok := tool.OutputSchema.(map[string]any)
		if !ok || output["type"] != "object" {
			t.Fatalf("%s outputSchema=%#v", tool.Name, tool.OutputSchema)
		}
		wantAdditionalProperties := tool.Name != "agentdock_context"
		if output["additionalProperties"] != wantAdditionalProperties {
			t.Fatalf("%s additionalProperties=%#v want %v", tool.Name, output["additionalProperties"], wantAdditionalProperties)
		}
		if tool.Annotations == nil {
			t.Fatalf("%s missing annotations", tool.Name)
		}
		if tool.Annotations.ReadOnlyHint != want.readOnly {
			t.Fatalf("%s readOnlyHint=%v want %v", tool.Name, tool.Annotations.ReadOnlyHint, want.readOnly)
		}
		if tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint != want.destructive {
			t.Fatalf("%s destructiveHint=%v want %v", tool.Name, tool.Annotations.DestructiveHint, want.destructive)
		}
		if tool.Annotations.OpenWorldHint == nil || *tool.Annotations.OpenWorldHint != want.openWorld {
			t.Fatalf("%s openWorldHint=%v want %v", tool.Name, tool.Annotations.OpenWorldHint, want.openWorld)
		}
		if tool.Annotations.IdempotentHint {
			t.Fatalf("%s idempotentHint drifted from AgentDock", tool.Name)
		}
	}
	for name := range expected {
		if !seen[name] {
			t.Fatalf("central tool %s missing", name)
		}
	}
}

func TestCentralRecallAndWorkflowInputContractParity(t *testing.T) {
	definitions := centralToolDefinitionsByName(t)

	recallMaintain := contractSchemaProperties(t, definitions["recall_maintain"].InputSchema)
	if _, ok := recallMaintain["max_results"]; !ok {
		t.Fatal("recall_maintain missing AgentDock max_results input")
	}

	privateNote := contractSchemaProperties(t, definitions["private_note_manage"].InputSchema)
	assertIntegerBounds(t, "private_note_manage.max_results", privateNote["max_results"], 1, 100)
	assertIntegerBounds(t, "private_note_manage.max_bytes", privateNote["max_bytes"], 1, 1048576)

	workflow := contractSchemaProperties(t, definitions["workflow_template_manage"].InputSchema)
	wantWorkflowFields := []string{
		"action", "template", "template_id", "template_ids", "template_version", "template_status",
		"allow_long_template", "long_template_reason", "goal", "device", "type",
	}
	if got := sortedKeys(workflow); !reflect.DeepEqual(got, wantWorkflowFieldsSorted(wantWorkflowFields)) {
		t.Fatalf("workflow input fields=%v want=%v", got, wantWorkflowFieldsSorted(wantWorkflowFields))
	}
}

func TestCentralOutputContractKeepsRecallCitationAndRenderableFields(t *testing.T) {
	definitions := centralToolDefinitionsByName(t)

	searchOutput := contractSchemaProperties(t, definitions["recall_search"].OutputSchema)
	results := searchOutput["results"].(map[string]any)
	item := results["items"].(map[string]any)
	required := stringSet(item["required"])
	for _, field := range []string{"id", "title", "url"} {
		if !required[field] {
			t.Fatalf("recall_search result item missing required citation field %s: %#v", field, item)
		}
	}

	writeOutput := contractSchemaProperties(t, definitions["recall_write"].OutputSchema)
	for _, field := range []string{"recall_target", "recall_action", "path", "changed", "dry_run", "confirmed", "written", "diff", "updates"} {
		if _, ok := writeOutput[field]; !ok {
			t.Fatalf("recall_write output missing %s", field)
		}
	}

	contextOutput := contractSchemaProperties(t, definitions["agentdock_context"].OutputSchema)
	for _, field := range []string{"nodes", "shared"} {
		if _, ok := contextOutput[field]; !ok {
			t.Fatalf("central fleet context output missing intentional Nexus field %s", field)
		}
	}
}

func centralToolDefinitionsByName(t *testing.T) map[string]anyToolDefinition {
	t.Helper()
	result := make(map[string]anyToolDefinition)
	for _, tool := range nexusToolDefinitions() {
		result[tool.Name] = anyToolDefinition{InputSchema: tool.InputSchema, OutputSchema: tool.OutputSchema}
	}
	return result
}

type anyToolDefinition struct {
	InputSchema  any
	OutputSchema any
}

func contractSchemaProperties(t *testing.T, schema any) map[string]any {
	t.Helper()
	object, ok := schema.(map[string]any)
	if !ok {
		t.Fatalf("schema is %T, want map[string]any", schema)
	}
	properties, ok := object["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties=%#v", object["properties"])
	}
	return properties
}

func assertIntegerBounds(t *testing.T, name string, raw any, minimum, maximum int) {
	t.Helper()
	property, ok := raw.(map[string]any)
	if !ok || property["type"] != "integer" || property["minimum"] != minimum || property["maximum"] != maximum {
		t.Fatalf("%s=%#v", name, raw)
	}
}

func sortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func wantWorkflowFieldsSorted(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}

func stringSet(raw any) map[string]bool {
	result := map[string]bool{}
	switch values := raw.(type) {
	case []string:
		for _, value := range values {
			result[value] = true
		}
	case []any:
		for _, value := range values {
			if text, ok := value.(string); ok {
				result[text] = true
			}
		}
	}
	return result
}
