package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/uvwt/nexusdock/internal/agentdock"
)

func TestAgentDockContextToolPublishesFleetSchemaWithoutNodeID(t *testing.T) {
	descriptor := fleetContextTestDescriptor()
	tool := nodeMCPTool(descriptor)

	input := tool.InputSchema.(map[string]any)
	properties := input["properties"].(map[string]any)
	if _, exists := properties["node_id"]; exists {
		t.Fatalf("fleet context input must not expose node_id: %#v", input)
	}
	if tool.Title != "AgentDock fleet context" || !strings.Contains(tool.Description, "all enabled AgentDock nodes") {
		t.Fatalf("fleet tool presentation = title %q description %q", tool.Title, tool.Description)
	}
	output := tool.OutputSchema.(map[string]any)
	outputProperties := output["properties"].(map[string]any)
	for _, field := range []string{"nodes", "shared"} {
		if _, exists := outputProperties[field]; !exists {
			t.Fatalf("fleet output schema missing %s: %#v", field, output)
		}
	}
	if _, exists := outputProperties["skills"]; exists {
		t.Fatalf("fleet output must not pretend one node context is the public result: %#v", output)
	}
}

func TestFleetContextSharedDataIsDeduplicatedAndNodeLocalDataStaysLocal(t *testing.T) {
	first := agentDockContext{
		Skills:            []agentDockContextSkill{{Name: "desktop", Description: "Desktop", File: "skill://desktop/SKILL.md"}},
		DynamicMCP:        []agentDockContextItem{{Name: "github", Description: "GitHub"}},
		WorkflowTemplates: []agentDockContextItem{{Name: "deploy", Description: "Deploy"}},
		Recall:            &agentDockContextRecall{Enabled: true, Items: []agentDockContextItem{{Name: "profile.md", Description: "Profile"}}},
		Rules:             []string{"rule-a", "rule-b"},
		Warnings: []agentDockContextWarning{
			{Source: "skills", Message: "skill warning"},
			{Source: "recall", Message: "recall warning"},
		},
	}
	second := agentDockContext{
		Skills:            []agentDockContextSkill{{Name: "desktop", Description: "Desktop", File: "skill://desktop/SKILL.md"}},
		DynamicMCP:        []agentDockContextItem{{Name: "dida365", Description: "Tasks"}},
		WorkflowTemplates: []agentDockContextItem{{Name: "deploy", Description: "Deploy"}, {Name: "review", Description: "Review"}},
		Recall:            &agentDockContextRecall{Enabled: true, Items: []agentDockContextItem{{Name: "profile.md", Description: "Profile"}}},
		Rules:             []string{"rule-b", "rule-c"},
		Warnings:          []agentDockContextWarning{{Source: "recall", Message: "recall warning"}},
	}

	shared := fleetAgentDockSharedContext{WorkflowTemplates: []agentDockContextItem{}, Rules: []string{}}
	mergeFleetAgentDockSharedContext(&shared, first)
	mergeFleetAgentDockSharedContext(&shared, second)
	if got := []string{shared.WorkflowTemplates[0].Name, shared.WorkflowTemplates[1].Name}; !reflect.DeepEqual(got, []string{"deploy", "review"}) {
		t.Fatalf("workflow templates = %#v", shared.WorkflowTemplates)
	}
	if shared.Recall == nil || len(shared.Recall.Items) != 1 || shared.Recall.Items[0].Name != "profile.md" {
		t.Fatalf("recall = %#v", shared.Recall)
	}
	if !reflect.DeepEqual(shared.Rules, []string{"rule-a", "rule-b", "rule-c"}) {
		t.Fatalf("rules = %#v", shared.Rules)
	}
	if len(shared.Warnings) != 1 || shared.Warnings[0].Source != "recall" {
		t.Fatalf("shared warnings = %#v", shared.Warnings)
	}
	local := localAgentDockContext(first)
	if len(local.Skills) != 1 || len(local.DynamicMCP) != 1 || len(local.Warnings) != 1 || local.Warnings[0].Source != "skills" {
		t.Fatalf("local context = %#v", local)
	}
}

func TestCallFleetAgentDockContextAggregatesOnlineAndOfflineNodes(t *testing.T) {
	store := newHTTPTestAgentDockStore(t)
	descriptor := fleetContextTestDescriptor()
	online := pairHTTPTestNode(t, store, "device_context_online", "DockMini", "2.0.0", descriptor)
	offline := pairHTTPTestNode(t, store, "device_context_offline", "DockWin", "2.0.0", descriptor)
	disabled := pairHTTPTestNode(t, store, "device_context_disabled", "DockAir", "2.0.0", descriptor)
	enabled := false
	if _, err := store.Update(t.Context(), disabled.ID, agentdock.UpdateInput{Enabled: &enabled}); err != nil {
		t.Fatal(err)
	}

	hub := agentdock.NewHub(store)
	connectFleetContextTestNode(t, hub, online, descriptor, map[string]any{
		"skills":             []any{map[string]any{"name": "desktop", "description": "Desktop", "file": "skill://desktop/SKILL.md"}},
		"dynamic_mcp":        []any{map[string]any{"name": "github", "description": "GitHub"}},
		"workflow_templates": []any{map[string]any{"name": "deploy", "description": "Deploy"}},
		"recall":             map[string]any{"enabled": true, "items": []any{map[string]any{"name": "profile.md", "description": "Profile"}}},
		"rules":              []any{"rule-a", "rule-b"},
	})
	hash, err := toolContractHash(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		agentDock: store, agentDockHub: hub,
		mcpTools: map[string]publishedNodeTool{agentDockContextToolName: {
			Descriptor: descriptor, ContractHash: hash, AcceptedSemanticHashes: []string{hash},
		}},
	}

	result, err := server.callFleetAgentDockContext(t.Context())
	if err != nil || result.IsError {
		t.Fatalf("fleet context result=%#v err=%v", result, err)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structuredContent = %#v", result.StructuredContent)
	}
	var fleet fleetAgentDockContext
	if err := decodeMap(structured, &fleet); err != nil {
		t.Fatal(err)
	}
	if len(fleet.Nodes) != 2 {
		t.Fatalf("enabled fleet nodes = %#v", fleet.Nodes)
	}
	if fleet.Nodes[0].Name != "DockMini" || !fleet.Nodes[0].Online || fleet.Nodes[0].Context == nil || len(fleet.Nodes[0].Context.Skills) != 1 {
		t.Fatalf("online node context = %#v", fleet.Nodes[0])
	}
	if fleet.Nodes[1].Name != offline.Name || fleet.Nodes[1].Online || fleet.Nodes[1].Error != agentdock.ErrNodeOffline.Error() || fleet.Nodes[1].Context != nil {
		t.Fatalf("offline node context = %#v", fleet.Nodes[1])
	}
	if len(fleet.Shared.WorkflowTemplates) != 1 || fleet.Shared.WorkflowTemplates[0].Name != "deploy" || fleet.Shared.Recall == nil || !reflect.DeepEqual(fleet.Shared.Rules, []string{"rule-a", "rule-b"}) {
		t.Fatalf("shared context = %#v", fleet.Shared)
	}
}

func TestDecodeAgentDockContextRejectsLegacyMarkdownResult(t *testing.T) {
	_, err := decodeAgentDockContextResult(map[string]any{
		"isError":           false,
		"structuredContent": map[string]any{"context": "# AgentDock Context"},
	})
	if err == nil || !strings.Contains(err.Error(), "结构化契约") {
		t.Fatalf("legacy context should be rejected, got %v", err)
	}
}

func fleetContextTestDescriptor() agentdock.ToolDescriptor {
	return agentdock.ToolDescriptor{
		Name:        agentDockContextToolName,
		Title:       "AgentDock context",
		Description: "Return structured AgentDock bootstrap context.",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": true},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"skills":             map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
				"dynamic_mcp":        map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
				"acp":                map[string]any{"type": "object"},
				"workflow_templates": map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
				"recall":             map[string]any{"type": "object"},
				"rules":              map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"warnings":           map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
			},
			"required": []any{"skills", "dynamic_mcp", "workflow_templates", "rules"}, "additionalProperties": true,
		},
	}
}

func connectFleetContextTestNode(t *testing.T, hub *agentdock.Hub, node agentdock.Node, descriptor agentdock.ToolDescriptor, structured map[string]any) {
	t.Helper()
	connected := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := hub.Accept(w, r, node.ID); err != nil {
			t.Errorf("accept context node: %v", err)
			return
		}
		close(connected)
	}))
	t.Cleanup(server.Close)

	socket, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = socket.Close() })
	if err := socket.WriteJSON(map[string]any{
		"type": "node.hello", "protocol_version": agentdock.ConnectionProtocolVersion,
		"hello": agentdock.Hello{
			DeviceID: node.DeviceID, Version: node.Version, ProtocolVersion: agentdock.ConnectionProtocolVersion,
			OS: node.OS, Arch: node.Arch, Capabilities: []string{descriptor.Name}, Tools: []agentdock.ToolDescriptor{descriptor},
		},
	}); err != nil {
		t.Fatal(err)
	}
	var ready map[string]any
	if err := socket.ReadJSON(&ready); err != nil || ready["type"] != "node.ready" {
		t.Fatalf("ready=%#v err=%v", ready, err)
	}
	<-connected

	go func() {
		var invoke struct {
			Type      string          `json:"type"`
			RequestID string          `json:"request_id"`
			Operation string          `json:"operation"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := socket.ReadJSON(&invoke); err != nil {
			return
		}
		_ = socket.WriteJSON(map[string]any{
			"type": "tool.result", "request_id": invoke.RequestID,
			"result": map[string]any{
				"isError": false, "structuredContent": structured,
				"content": []map[string]any{{"type": "text", "text": "context"}},
			},
		})
	}()
}
