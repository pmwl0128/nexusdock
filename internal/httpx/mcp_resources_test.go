package httpx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/uvwt/nexusdock/internal/agentdock"
	"github.com/uvwt/nexusdock/internal/config"
)

func TestSyncMCPAppResourcesPublishesToolUIResources(t *testing.T) {
	const uri = "ui://agentdock/file-change"
	const domain = "https://nexus.example.test"
	sdk := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "test", Version: "1"}, nil)
	server := &Server{
		cfg:       config.Config{PublicURL: domain},
		mcpServer: sdk,
		mcpTools: map[string]publishedNodeTool{
			"file_edit": {Descriptor: agentdock.ToolDescriptor{
				Name: "file_edit", NexusResourceRelay: true,
				Meta: map[string]any{"ui": map[string]any{"resourceUri": uri}},
			}},
		},
		mcpResources: make(map[string]struct{}),
	}
	server.syncMCPAppResources()

	serverTransport, clientTransport := mcpsdk.NewInMemoryTransports()
	serverDone := make(chan error, 1)
	go func() { serverDone <- sdk.Run(t.Context(), serverTransport) }()
	client := mcpsdk.NewClient(
		&mcpsdk.Implementation{Name: "nexus-resource-test", Version: "1"},
		&mcpsdk.ClientOptions{Capabilities: &mcpsdk.ClientCapabilities{}},
	)
	session, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
		if err := <-serverDone; err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("Server.Run() error = %v", err)
		}
	})

	var listed *mcpsdk.Resource
	for resource, err := range session.Resources(t.Context(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		if resource.URI == uri {
			listed = resource
		}
	}
	if listed == nil || listed.MIMEType != mcpAppMIMEType {
		t.Fatalf("listed resource = %#v", listed)
	}
	ui, ok := listed.Meta["ui"].(map[string]any)
	if !ok || ui["prefersBorder"] != true || ui["domain"] != domain {
		t.Fatalf("resource ui meta = %#v", listed.Meta)
	}
}

func TestPublishedMCPAppResourcesAlwaysIncludeCentralApps(t *testing.T) {
	server := &Server{mcpTools: map[string]publishedNodeTool{}}
	resources := server.publishedMCPAppResourceURIs()
	for _, uri := range []string{agentDockContextUIResourceURI, recallUIResourceURI, workflowUIResourceURI} {
		if _, exists := resources[uri]; !exists {
			t.Fatalf("central MCP App resource %s is missing: %#v", uri, resources)
		}
	}
}

func TestCentralWorkflowResultMetaIsActionScoped(t *testing.T) {
	if meta := centralToolResultMeta("workflow_template_manage", map[string]any{"action": "list"}); meta != nil {
		t.Fatalf("list unexpectedly has Workflow UI meta: %#v", meta)
	}
	meta := centralToolResultMeta("workflow_template_manage", map[string]any{"action": "match"})
	ui, ok := meta["ui"].(map[string]any)
	if !ok || ui["resourceUri"] != workflowUIResourceURI {
		t.Fatalf("match Workflow UI meta = %#v", meta)
	}
}

func TestDecodeNodeMCPAppResourceReplacesNodeDomainWithNexusDomain(t *testing.T) {
	const uri = "ui://agentdock/task-progress"
	const domain = "https://nexus.example.test"
	read, err := decodeNodeMCPAppResource(uri, map[string]any{
		"contents": []any{map[string]any{
			"uri": uri, "mimeType": mcpAppMIMEType, "text": "<!doctype html>",
			"_meta": map[string]any{"ui": map[string]any{"domain": "https://dockmini.example.test"}},
		}},
	}, domain)
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Contents) != 1 {
		t.Fatalf("contents = %#v", read.Contents)
	}
	ui, ok := read.Contents[0].Meta["ui"].(map[string]any)
	if !ok || ui["prefersBorder"] != true {
		t.Fatalf("sanitized meta = %#v", read.Contents[0].Meta)
	}
	if ui["domain"] != domain {
		t.Fatalf("Nexus widget domain = %#v", ui)
	}
}

func TestNodeProvidesPublishedMCPAppResourceRequiresAcceptedExecutionContract(t *testing.T) {
	const uri = "ui://agentdock/file-change"
	descriptor := agentdock.ToolDescriptor{
		Name: "file_edit", NexusResourceRelay: true,
		InputSchema: map[string]any{
			"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}},
		},
		Meta: map[string]any{"ui": map[string]any{"resourceUri": uri}},
	}
	hash, err := toolContractHash(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{mcpTools: map[string]publishedNodeTool{
		"file_edit": {Descriptor: descriptor, ContractHash: hash, AcceptedSemanticHashes: []string{hash}},
	}}
	if !server.nodeProvidesPublishedMCPAppResource([]agentdock.ToolDescriptor{descriptor}, uri) {
		t.Fatal("compatible resource provider was rejected")
	}
	legacy, err := cloneToolDescriptor(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	legacy.NexusResourceRelay = false
	if server.nodeProvidesPublishedMCPAppResource([]agentdock.ToolDescriptor{legacy}, uri) {
		t.Fatal("legacy provider without resource relay support was accepted")
	}

	drifted, err := cloneToolDescriptor(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	drifted.InputSchema["properties"].(map[string]any)["path"].(map[string]any)["type"] = "array"
	if server.nodeProvidesPublishedMCPAppResource([]agentdock.ToolDescriptor{drifted}, uri) {
		t.Fatal("resource provider with an unaccepted execution contract was accepted")
	}
}

func TestToolUIResourceURIRejectsForeignResources(t *testing.T) {
	descriptor := agentdock.ToolDescriptor{Meta: map[string]any{
		"ui": map[string]any{"resourceUri": "https://example.test/widget"},
	}}
	if got := toolUIResourceURI(descriptor); got != "" {
		t.Fatalf("foreign resource URI = %q", got)
	}
}

func TestNodeProvidesCentralMCPAppResourceRequiresExplicitRendererContract(t *testing.T) {
	modern := agentdock.ToolDescriptor{
		Name: "agentdock_context", NexusResourceRelay: true, NexusResourceContract: contextMCPAppResourceContract,
		Meta: map[string]any{"ui": map[string]any{"resourceUri": agentDockContextUIResourceURI}},
	}
	if !nodeProvidesCentralMCPAppResource([]agentdock.ToolDescriptor{modern}, agentDockContextUIResourceURI, contextMCPAppResourceContract) {
		t.Fatal("modern central renderer contract was rejected")
	}
	legacy := modern
	legacy.NexusResourceContract = ""
	if nodeProvidesCentralMCPAppResource([]agentdock.ToolDescriptor{legacy}, agentDockContextUIResourceURI, contextMCPAppResourceContract) {
		t.Fatal("legacy provider without renderer contract was accepted")
	}
	wrong := modern
	wrong.NexusResourceContract = "agentdock.context.fleet.v0"
	if nodeProvidesCentralMCPAppResource([]agentdock.ToolDescriptor{wrong}, agentDockContextUIResourceURI, contextMCPAppResourceContract) {
		t.Fatal("provider with wrong renderer contract was accepted")
	}
}

func TestCentralMCPAppResourceSkipsLegacyOnlineProvider(t *testing.T) {
	store := newHTTPTestAgentDockStore(t)
	legacyDescriptor := agentdock.ToolDescriptor{
		Name: "agentdock_context", NexusResourceRelay: true,
		Meta: map[string]any{"ui": map[string]any{"resourceUri": agentDockContextUIResourceURI}},
	}
	modernDescriptor := legacyDescriptor
	modernDescriptor.NexusResourceContract = contextMCPAppResourceContract
	legacy := pairHTTPTestNode(t, store, "device_resource_legacy", "A Legacy", "1.0.0", legacyDescriptor)
	modern := pairHTTPTestNode(t, store, "device_resource_modern", "B Modern", "2.0.0", modernDescriptor)
	hub := agentdock.NewHub(store)
	legacyInvoked := connectResourceTestNode(t, hub, legacy, legacyDescriptor, "<html>legacy</html>", false)
	modernInvoked := connectResourceTestNode(t, hub, modern, modernDescriptor, "<html>modern</html>", true)
	nodes, err := store.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{agentDock: store, agentDockHub: hub, cfg: config.Config{PublicURL: "https://nexus.example.test"}}
	read, err := server.readCentralMCPAppResourceWithTimeout(t.Context(), nodes, agentDockContextUIResourceURI, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Contents) != 1 || read.Contents[0].Text != "<html>modern</html>" {
		t.Fatalf("central resource=%#v", read.Contents)
	}
	select {
	case <-modernInvoked:
	case <-time.After(time.Second):
		t.Fatal("modern provider was not invoked")
	}
	select {
	case <-legacyInvoked:
		t.Fatal("legacy provider was invoked despite missing renderer contract")
	default:
	}
}

func TestCentralMCPAppResourceBoundsStalledCompatibleProvider(t *testing.T) {
	store := newHTTPTestAgentDockStore(t)
	descriptor := agentdock.ToolDescriptor{
		Name: "agentdock_context", NexusResourceRelay: true, NexusResourceContract: contextMCPAppResourceContract,
		Meta: map[string]any{"ui": map[string]any{"resourceUri": agentDockContextUIResourceURI}},
	}
	node := pairHTTPTestNode(t, store, "device_resource_stalled", "Stalled", "2.0.0", descriptor)
	hub := agentdock.NewHub(store)
	connectStalledResourceTestNode(t, hub, node, descriptor)
	nodes, err := store.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{agentDock: store, agentDockHub: hub}
	started := time.Now()
	_, err = server.readCentralMCPAppResourceWithTimeout(t.Context(), nodes, agentDockContextUIResourceURI, 30*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "resource timeout") {
		t.Fatalf("timeout err=%v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("resource timeout took %v", elapsed)
	}
}

func connectResourceTestNode(t *testing.T, hub *agentdock.Hub, node agentdock.Node, descriptor agentdock.ToolDescriptor, html string, shouldRespond bool) <-chan struct{} {
	t.Helper()
	connected := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := hub.Accept(w, r, node.ID); err != nil {
			t.Errorf("accept resource node: %v", err)
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
		"hello": agentdock.Hello{DeviceID: node.DeviceID, Version: node.Version, ProtocolVersion: agentdock.ConnectionProtocolVersion, OS: node.OS, Arch: node.Arch, Capabilities: []string{descriptor.Name}, Tools: []agentdock.ToolDescriptor{descriptor}},
	}); err != nil {
		t.Fatal(err)
	}
	var ready map[string]any
	if err := socket.ReadJSON(&ready); err != nil || ready["type"] != "node.ready" {
		t.Fatalf("ready=%#v err=%v", ready, err)
	}
	<-connected
	invoked := make(chan struct{}, 1)
	go func() {
		var request struct {
			Type      string `json:"type"`
			RequestID string `json:"request_id"`
			Operation string `json:"operation"`
		}
		if err := socket.ReadJSON(&request); err != nil {
			return
		}
		invoked <- struct{}{}
		if request.Operation != "resource.read" || !shouldRespond {
			return
		}
		_ = socket.WriteJSON(map[string]any{
			"type": "tool.result", "request_id": request.RequestID,
			"result": map[string]any{"contents": []any{map[string]any{"uri": agentDockContextUIResourceURI, "mimeType": mcpAppMIMEType, "text": html}}},
		})
	}()
	return invoked
}

func connectStalledResourceTestNode(t *testing.T, hub *agentdock.Hub, node agentdock.Node, descriptor agentdock.ToolDescriptor) {
	t.Helper()
	_ = connectResourceTestNode(t, hub, node, descriptor, "", false)
}
