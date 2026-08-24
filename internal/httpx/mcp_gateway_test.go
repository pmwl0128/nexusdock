package httpx

import (
	"log/slog"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/uvwt/nexusdock/internal/agentdock"
	"github.com/uvwt/nexusdock/internal/core"
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

func TestRegisterNodeToolsKeepsFirstPublishedContract(t *testing.T) {
	server := &Server{
		mcpServer: mcpsdk.NewServer(&mcpsdk.Implementation{Name: "test", Version: "1"}, nil),
		mcpTools:  make(map[string]publishedNodeTool),
	}
	first := agentdock.ToolDescriptor{
		Name: "exec_command",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"timeout": map[string]any{"type": "integer"}},
		},
	}
	second := agentdock.ToolDescriptor{
		Name: "exec_command",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"timeout": map[string]any{"type": "number"}},
		},
	}

	server.registerNodeTools(agentdock.Node{ID: "node_old", Version: "1.8.3"}, agentdock.Hello{Tools: []agentdock.ToolDescriptor{first}})
	server.registerNodeTools(agentdock.Node{ID: "node_new", Version: "1.9.0"}, agentdock.Hello{Tools: []agentdock.ToolDescriptor{second}})

	published, ok := server.publishedNodeTool("exec_command")
	if !ok {
		t.Fatal("exec_command was not published")
	}
	firstHash, _ := toolContractHash(first)
	if published.SourceNodeID != "node_old" || published.SourceVersion != "1.8.3" || published.ContractHash != firstHash {
		t.Fatalf("published contract changed: %#v", published)
	}
}

func TestCallNodeToolReturnsContractMismatchBeforeInvoke(t *testing.T) {
	store := newHTTPTestAgentDockStore(t)
	target := pairHTTPTestNode(t, store, "device_abcdefgh", "DockAir", "1.9.0", agentdock.ToolDescriptor{
		Name: "exec_command",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"timeout": map[string]any{"type": "number"}},
		},
	})
	published := agentdock.ToolDescriptor{
		Name: "exec_command",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"timeout": map[string]any{"type": "integer"}},
		},
	}
	server := &Server{
		agentDock:    store,
		agentDockHub: agentdock.NewHub(store),
		mcpServer:    mcpsdk.NewServer(&mcpsdk.Implementation{Name: "test", Version: "1"}, nil),
		mcpTools:     make(map[string]publishedNodeTool),
	}
	server.registerNodeTools(agentdock.Node{ID: "node_old", Version: "1.8.3"}, agentdock.Hello{Tools: []agentdock.ToolDescriptor{published}})

	result, err := server.callNodeTool(t.Context(), "exec_command", map[string]any{"node_id": target.ID, "timeout": 1})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatalf("result should be an MCP tool error: %#v", result)
	}
	details, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structured content = %#v", result.StructuredContent)
	}
	if details["code"] != "TOOL_CONTRACT_MISMATCH" {
		t.Fatalf("code = %#v", details["code"])
	}
	if details["error"] != "AgentDock 设备版本不一致，请将所有设备更新到 1.9.0 后刷新 GPT 工具。" {
		t.Fatalf("error = %#v", details["error"])
	}
	differences, ok := details["differences"].([]any)
	if !ok || len(differences) != 1 {
		t.Fatalf("differences = %#v", details["differences"])
	}
	difference := differences[0].(map[string]any)
	if difference["path"] != "inputSchema.properties.timeout.type" || difference["published"] != "integer" || difference["node"] != "number" {
		t.Fatalf("difference = %#v", difference)
	}
}

func TestCallNodeToolAcceptsSameContractWithDifferentDescription(t *testing.T) {
	store := newHTTPTestAgentDockStore(t)
	descriptor := agentdock.ToolDescriptor{
		Name: "read_file", Description: "macOS description",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}},
	}
	targetDescriptor := descriptor
	targetDescriptor.Description = "Windows description"
	target := pairHTTPTestNode(t, store, "device_ijklmnop", "DockWin", "1.8.3", targetDescriptor)
	server := &Server{
		agentDock:    store,
		agentDockHub: agentdock.NewHub(store),
		mcpServer:    mcpsdk.NewServer(&mcpsdk.Implementation{Name: "test", Version: "1"}, nil),
		mcpTools:     make(map[string]publishedNodeTool),
	}
	server.registerNodeTools(agentdock.Node{ID: "node_source", Version: "1.8.3"}, agentdock.Hello{Tools: []agentdock.ToolDescriptor{descriptor}})

	result, err := server.callNodeTool(t.Context(), "read_file", map[string]any{"node_id": target.ID, "path": "/tmp/a"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatalf("offline target should prove invocation was attempted: %#v", result)
	}
	details := result.StructuredContent.(map[string]any)
	if details["error"] != agentdock.ErrNodeOffline.Error() {
		t.Fatalf("expected compatible contract to reach hub, got %#v", details)
	}
}

func newHTTPTestAgentDockStore(t *testing.T) *agentdock.Store {
	t.Helper()
	db, err := core.OpenSQLite(t.Context(), ":memory:", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := core.EnsureSchema(t.Context(), db); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	store, err := agentdock.NewStore(db)
	if err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return store
}

func pairHTTPTestNode(t *testing.T, store *agentdock.Store, deviceID, name, version string, descriptor agentdock.ToolDescriptor) agentdock.Node {
	t.Helper()
	pairing, err := store.CreatePairingCode(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	node, err := store.Pair(t.Context(), agentdock.PairInput{Code: pairing.Code, DeviceID: deviceID, Name: name})
	if err != nil {
		t.Fatal(err)
	}
	node, err = store.UpdateHello(t.Context(), node.ID, agentdock.Hello{
		DeviceID: node.DeviceID, Version: version, ProtocolVersion: agentdock.ConnectionProtocolVersion,
		Capabilities: []string{descriptor.Name}, Tools: []agentdock.ToolDescriptor{descriptor},
	})
	if err != nil {
		t.Fatal(err)
	}
	return node
}

func TestRegisterNodeToolsPromotesOnlyAfterProvidersConverge(t *testing.T) {
	store := newHTTPTestAgentDockStore(t)
	oldDescriptor := agentdock.ToolDescriptor{
		Name: "exec_command",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"timeout": map[string]any{"type": "integer"}},
		},
	}
	newDescriptor := agentdock.ToolDescriptor{
		Name: "exec_command",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"timeout": map[string]any{"type": "number"}},
		},
	}
	first := pairHTTPTestNode(t, store, "device_qrstuvwx", "DockMini", "1.8.3", oldDescriptor)
	second := pairHTTPTestNode(t, store, "device_yzabcdef", "DockAir", "1.8.3", oldDescriptor)
	server := &Server{
		agentDock: store,
		mcpServer: mcpsdk.NewServer(&mcpsdk.Implementation{Name: "test", Version: "1"}, nil),
		mcpTools:  make(map[string]publishedNodeTool),
	}
	server.registerNodeTools(first, agentdock.Hello{Tools: []agentdock.ToolDescriptor{oldDescriptor}})
	oldHash, _ := toolContractHash(oldDescriptor)
	newHash, _ := toolContractHash(newDescriptor)

	first = updateHTTPTestNodeContract(t, store, first, "1.9.0", newDescriptor)
	server.registerNodeTools(first, agentdock.Hello{Tools: []agentdock.ToolDescriptor{newDescriptor}})
	published, _ := server.publishedNodeTool("exec_command")
	if published.ContractHash != oldHash {
		t.Fatalf("mixed providers changed public contract: %#v", published)
	}

	second = updateHTTPTestNodeContract(t, store, second, "1.9.0", newDescriptor)
	server.registerNodeTools(second, agentdock.Hello{Tools: []agentdock.ToolDescriptor{newDescriptor}})
	published, _ = server.publishedNodeTool("exec_command")
	if published.ContractHash != newHash || published.SourceVersion != "1.9.0" {
		t.Fatalf("converged providers did not promote new contract: %#v", published)
	}
}

func updateHTTPTestNodeContract(t *testing.T, store *agentdock.Store, node agentdock.Node, version string, descriptor agentdock.ToolDescriptor) agentdock.Node {
	t.Helper()
	updated, err := store.UpdateHello(t.Context(), node.ID, agentdock.Hello{
		DeviceID: node.DeviceID, Version: version, ProtocolVersion: agentdock.ConnectionProtocolVersion,
		Capabilities: []string{descriptor.Name}, Tools: []agentdock.ToolDescriptor{descriptor},
	})
	if err != nil {
		t.Fatal(err)
	}
	return updated
}

func TestInitializeMCPGatewayRestoresPublishedContractBeforeNodeOrder(t *testing.T) {
	store := newHTTPTestAgentDockStore(t)
	oldDescriptor := agentdock.ToolDescriptor{
		Name: "exec_command",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"timeout": map[string]any{"type": "integer"}},
		},
	}
	newDescriptor := agentdock.ToolDescriptor{
		Name: "exec_command",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"timeout": map[string]any{"type": "number"}},
		},
	}
	oldNode := pairHTTPTestNode(t, store, "device_restart_old", "ZuluOld", "1.8.3", oldDescriptor)
	_ = pairHTTPTestNode(t, store, "device_restart_new", "AlphaNew", "1.9.0", newDescriptor)

	first := &Server{
		agentDock: store,
		mcpServer: mcpsdk.NewServer(&mcpsdk.Implementation{Name: "test", Version: "1"}, nil),
		mcpTools:  make(map[string]publishedNodeTool),
	}
	first.registerNodeTools(oldNode, agentdock.Hello{Tools: []agentdock.ToolDescriptor{oldDescriptor}})
	first.registerNodeTools(agentdock.Node{ID: "node_new", Version: "1.9.0"}, agentdock.Hello{Tools: []agentdock.ToolDescriptor{newDescriptor}})
	oldHash, _ := toolContractHash(oldDescriptor)

	restarted := &Server{agentDock: store, mcpTools: make(map[string]publishedNodeTool)}
	restarted.initializeMCPGateway()
	published, ok := restarted.publishedNodeTool("exec_command")
	if !ok || published.ContractHash != oldHash || published.SourceVersion != "1.8.3" {
		t.Fatalf("restart changed published contract: %#v", published)
	}
}
