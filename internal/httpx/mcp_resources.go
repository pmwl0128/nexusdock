package httpx

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/uvwt/nexusdock/internal/agentdock"
)

const agentDockMCPAppResourcePrefix = "ui://agentdock/"
const mcpAppMIMEType = "text/html;profile=mcp-app"

func (s *Server) syncMCPAppResources() {
	if s == nil || s.mcpServer == nil {
		return
	}

	desired := s.publishedMCPAppResourceURIs()
	s.mcpResourcesMu.Lock()
	defer s.mcpResourcesMu.Unlock()
	if s.mcpResources == nil {
		s.mcpResources = make(map[string]struct{})
	}

	for uri := range s.mcpResources {
		if _, ok := desired[uri]; ok {
			continue
		}
		s.mcpServer.RemoveResources(uri)
		delete(s.mcpResources, uri)
	}
	for _, uri := range sortedMCPAppResourceURIs(desired) {
		if _, ok := s.mcpResources[uri]; ok {
			continue
		}
		uri := uri
		s.mcpServer.AddResource(&mcpsdk.Resource{
			URI:         uri,
			Name:        mcpAppResourceName(uri),
			Title:       "AgentDock MCP App",
			Description: "MCP App resource relayed from a compatible AgentDock node.",
			MIMEType:    mcpAppMIMEType,
			Meta:        nexusMCPAppResourceMeta(s.cfg.PublicURL),
		}, func(ctx context.Context, request *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
			if request == nil || request.Params == nil || request.Params.URI != uri {
				return nil, mcpsdk.ResourceNotFoundError(uri)
			}
			return s.readPublishedMCPAppResource(ctx, uri)
		})
		s.mcpResources[uri] = struct{}{}
	}
}

func (s *Server) publishedMCPAppResourceURIs() map[string]struct{} {
	s.mcpToolsMu.RLock()
	defer s.mcpToolsMu.RUnlock()
	uris := map[string]struct{}{
		agentDockContextUIResourceURI: {},
		recallUIResourceURI:           {},
		workflowUIResourceURI:         {},
	}
	for _, published := range s.mcpTools {
		if !published.Descriptor.NexusResourceRelay {
			continue
		}
		if uri := toolUIResourceURI(published.Descriptor); uri != "" {
			uris[uri] = struct{}{}
		}
	}
	return uris
}

func (s *Server) readPublishedMCPAppResource(ctx context.Context, uri string) (*mcpsdk.ReadResourceResult, error) {
	if s.agentDock == nil || s.agentDockHub == nil {
		return nil, mcpsdk.ResourceNotFoundError(uri)
	}
	nodes, err := s.agentDock.List(ctx)
	if err != nil {
		return nil, err
	}
	if isCentralMCPAppResourceURI(uri) {
		return s.readCentralMCPAppResource(ctx, nodes, uri)
	}

	foundProvider := false
	var lastErr error
	for _, node := range nodes {
		descriptors, descriptorErr := s.agentDock.ToolDescriptors(ctx, node.ID)
		if descriptorErr != nil {
			lastErr = descriptorErr
			continue
		}
		if !s.nodeProvidesPublishedMCPAppResource(descriptors, uri) {
			continue
		}
		foundProvider = true
		if !node.Enabled || !s.agentDockHub.Online(node.ID) {
			continue
		}

		result, invokeErr := s.agentDockHub.Invoke(ctx, node.ID, "resource.read", map[string]any{"uri": uri})
		if invokeErr != nil {
			lastErr = invokeErr
			continue
		}
		read, decodeErr := decodeNodeMCPAppResource(uri, result, s.cfg.PublicURL)
		if decodeErr != nil {
			lastErr = decodeErr
			continue
		}
		return read, nil
	}

	if !foundProvider {
		return nil, mcpsdk.ResourceNotFoundError(uri)
	}
	if lastErr != nil {
		return nil, fmt.Errorf("读取 AgentDock MCP App resource %s: %w", uri, lastErr)
	}
	return nil, fmt.Errorf("AgentDock MCP App resource %s 当前没有在线 provider", uri)
}

func isCentralMCPAppResourceURI(uri string) bool {
	switch uri {
	case agentDockContextUIResourceURI, recallUIResourceURI, workflowUIResourceURI:
		return true
	default:
		return false
	}
}

func (s *Server) readCentralMCPAppResource(ctx context.Context, nodes []agentdock.Node, uri string) (*mcpsdk.ReadResourceResult, error) {
	var lastErr error
	for _, node := range nodes {
		if !node.Enabled || !s.agentDockHub.Online(node.ID) {
			continue
		}
		result, invokeErr := s.agentDockHub.Invoke(ctx, node.ID, "resource.read", map[string]any{"uri": uri})
		if invokeErr != nil {
			lastErr = invokeErr
			continue
		}
		read, decodeErr := decodeNodeMCPAppResource(uri, result, s.cfg.PublicURL)
		if decodeErr != nil {
			lastErr = decodeErr
			continue
		}
		return read, nil
	}
	if lastErr != nil {
		return nil, fmt.Errorf("读取中央 MCP App resource %s: %w", uri, lastErr)
	}
	return nil, fmt.Errorf("中央 MCP App resource %s 当前没有在线 AgentDock provider", uri)
}

func (s *Server) nodeProvidesPublishedMCPAppResource(descriptors []agentdock.ToolDescriptor, uri string) bool {
	for _, descriptor := range descriptors {
		if !descriptor.NexusResourceRelay || toolUIResourceURI(descriptor) != uri {
			continue
		}
		published, ok := s.publishedNodeTool(descriptor.Name)
		if !ok || toolUIResourceURI(published.Descriptor) != uri {
			continue
		}
		hash, err := toolContractHash(descriptor)
		if err == nil && containsToolContractHash(published.AcceptedSemanticHashes, hash) {
			return true
		}
	}
	return false
}

func decodeNodeMCPAppResource(uri string, result map[string]any, publicURL string) (*mcpsdk.ReadResourceResult, error) {
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("编码节点 MCP App resource: %w", err)
	}
	var read mcpsdk.ReadResourceResult
	if err := json.Unmarshal(encoded, &read); err != nil {
		return nil, fmt.Errorf("解析节点 MCP App resource: %w", err)
	}
	if len(read.Contents) == 0 {
		return nil, fmt.Errorf("节点 MCP App resource %s 内容为空", uri)
	}
	for _, content := range read.Contents {
		if content == nil || content.URI != uri || content.MIMEType != mcpAppMIMEType || content.Text == "" {
			return nil, fmt.Errorf("节点 MCP App resource %s 返回了无效内容", uri)
		}
		// Resource 由 Nexus 对外提供，不能沿用节点域；组件必须使用 Nexus 自己的唯一公网 origin。
		content.Meta = nexusMCPAppResourceMeta(publicURL)
	}
	return &read, nil
}

func toolUIResourceURI(descriptor agentdock.ToolDescriptor) string {
	ui, ok := descriptor.Meta["ui"].(map[string]any)
	if !ok {
		return ""
	}
	uri, _ := ui["resourceUri"].(string)
	uri = strings.TrimSpace(uri)
	if !strings.HasPrefix(uri, agentDockMCPAppResourcePrefix) {
		return ""
	}
	return uri
}

func mcpAppResourceName(uri string) string {
	name := strings.TrimPrefix(uri, agentDockMCPAppResourcePrefix)
	name = strings.Trim(strings.ReplaceAll(name, "/", "-"), "-")
	if name == "" {
		return "agentdock-mcp-app"
	}
	return "agentdock-" + name
}

func nexusMCPAppResourceMeta(publicURL string) mcpsdk.Meta {
	ui := map[string]any{
		"csp": map[string]any{
			"connectDomains":  []string{},
			"resourceDomains": []string{},
		},
		"prefersBorder": true,
	}
	if domain := strings.TrimRight(strings.TrimSpace(publicURL), "/"); domain != "" {
		ui["domain"] = domain
	}
	return mcpsdk.Meta{"ui": ui}
}

func sortedMCPAppResourceURIs(resources map[string]struct{}) []string {
	uris := make([]string, 0, len(resources))
	for uri := range resources {
		uris = append(uris, uri)
	}
	sort.Strings(uris)
	return uris
}
