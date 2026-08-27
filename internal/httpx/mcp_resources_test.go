package httpx

import (
	"context"
	"errors"
	"testing"

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
