package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/uvwt/nexusdock/internal/agentdock"
	"github.com/uvwt/nexusdock/internal/privatenotes"
	"github.com/uvwt/nexusdock/internal/recall"
)

const nexusServerInstructions = "NexusDock 可以连接并统一操作多台 AgentDock 设备。" +
	"优先调用 `agentdock_context` 获取可用设备、节点标识以及各设备的核心能力、Skill、动态 MCP、Workflow 模板、重要上下文和长期记忆索引。" +
	"需要操作具体设备时，根据 `agentdock_context` 返回的节点信息选择目标 `node_id`。" +
	"需要查找或读取长期记忆时使用 `recall_*`；需要查找或使用 Workflow 模板时使用 `workflow_template_manage`；" +
	"处理多步骤任务时使用 `task_manage` 记录和维护任务进度。根据用户需求选择合适的设备和能力，检查、操作并验证设备状态。"

var nexusToolNames = map[string]struct{}{
	"agentdock_context": {}, "workflow_template_manage": {},
	"recall_bootstrap": {}, "recall_search": {}, "recall_read": {},
	"recall_write": {}, "recall_maintain": {}, "private_note_manage": {},
}

func (s *Server) initializeMCPGateway() {
	s.mcpServer = mcpsdk.NewServer(
		&mcpsdk.Implementation{Name: "nexusdock", Version: "1"},
		&mcpsdk.ServerOptions{
			Capabilities: &mcpsdk.ServerCapabilities{},
			Instructions: nexusServerInstructions,
		},
	)
	for _, definition := range nexusToolDefinitions() {
		definition := definition
		s.mcpServer.AddTool(definition, func(ctx context.Context, request *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
			arguments, err := toolArguments(request)
			if err != nil {
				return nil, err
			}
			result, err := s.callNexusTool(ctx, definition.Name, arguments)
			response, responseErr := gatewayToolResult(definition.Name, result, err)
			if responseErr == nil && response != nil {
				response.Meta = centralToolResultMeta(definition.Name, arguments)
			}
			return response, responseErr
		})
	}
	if s.agentDockHub != nil {
		s.agentDockHub.SetHelloHandler(s.registerNodeTools)
	}
	if s.agentDock != nil {
		ctx := context.Background()
		if err := s.loadPublishedNodeTools(ctx); err != nil && s.logger != nil {
			s.logger.Warn("恢复 AgentDock 公开工具契约失败", "error", err)
		}
		if nodes, err := s.agentDock.List(ctx); err == nil {
			for _, node := range nodes {
				descriptors, descriptorErr := s.agentDock.ToolDescriptors(ctx, node.ID)
				if descriptorErr == nil {
					s.registerNodeTools(node, agentdock.Hello{Tools: descriptors})
				}
			}
		}
		// 启动时也核对一次已发布目录，清理旧版本遗留但 fleet 已不再提供的 stale tool。
		s.reconcileNodeToolContracts(s.publishedNodeToolNames())
		s.syncMCPAppResources()
	}
	s.mcpHandler = mcpsdk.NewStreamableHTTPHandler(
		func(*http.Request) *mcpsdk.Server { return s.mcpServer },
		&mcpsdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true, MaxRequestBodyBytes: 1 << 20, PropagateRequestCancellation: true},
	)
}

func (s *Server) registerNodeTools(node agentdock.Node, hello agentdock.Hello) {
	defer s.syncMCPAppResources()
	helloToolNames := make(map[string]struct{}, len(hello.Tools))
	for _, descriptor := range hello.Tools {
		if _, central := nexusToolNames[descriptor.Name]; central || strings.TrimSpace(descriptor.Name) == "" {
			continue
		}
		helloToolNames[descriptor.Name] = struct{}{}
		contractHash, err := toolContractHash(descriptor)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn("计算 AgentDock 工具契约失败", "node_id", node.ID, "tool", descriptor.Name, "error", err)
			}
			continue
		}

		name := descriptor.Name
		candidate := publishedNodeTool{
			Descriptor: descriptor, ContractHash: contractHash,
			AcceptedSemanticHashes: []string{contractHash},
		}
		s.mcpToolsMu.Lock()
		published, exists := s.mcpTools[name]
		if !exists {
			// 首次出现的契约先持久化再公开，确保 Nexus 重启后仍沿用同一个 schema。
			if err := s.persistPublishedNodeTool(context.Background(), candidate); err != nil {
				s.mcpToolsMu.Unlock()
				if s.logger != nil {
					s.logger.Warn("保存 AgentDock 公开工具契约失败", "node_id", node.ID, "tool", name, "error", err)
				}
				continue
			}
			s.mcpServer.AddTool(nodeMCPTool(descriptor), s.nodeToolHandler(name))
			s.mcpTools[name] = candidate
		}
		s.mcpToolsMu.Unlock()
		if exists && (published.ContractHash != contractHash ||
			!containsToolContractHash(published.AcceptedSemanticHashes, contractHash) ||
			!jsonValuesEqual(published.Descriptor.Meta, descriptor.Meta) ||
			!jsonValuesEqual(published.Descriptor.Annotations, descriptor.Annotations) ||
			published.Descriptor.NexusResourceRelay != descriptor.NexusResourceRelay) {
			// schema 不同不等于不兼容；由 Fleet 合并器决定能否安全形成同一代公开契约。
			if err := s.reconcileFleetNodeTool(name); err != nil && s.logger != nil {
				s.logger.Warn("检查 AgentDock 工具契约兼容性失败", "tool", name, "error", err)
			}
		}
	}

	// Hello 是当前节点完整能力快照。已公开但本次不再上报的工具也要重新核对，
	// 这样最后一个 provider 真正移除能力时才会退休工具，而不是永久留下 stale schema。
	missingPublished := make([]string, 0)
	for _, name := range s.publishedNodeToolNames() {
		if _, present := helloToolNames[name]; !present {
			missingPublished = append(missingPublished, name)
		}
	}
	s.reconcileNodeToolContracts(missingPublished)
}

func nodeMCPTool(descriptor agentdock.ToolDescriptor) *mcpsdk.Tool {
	tool := &mcpsdk.Tool{
		Name: descriptor.Name, Title: descriptor.Title, Description: descriptor.Description,
		InputSchema: nodeInputSchema(descriptor.InputSchema), OutputSchema: descriptor.OutputSchema,
	}
	if len(descriptor.Annotations) > 0 {
		encoded, _ := json.Marshal(descriptor.Annotations)
		var annotations mcpsdk.ToolAnnotations
		if json.Unmarshal(encoded, &annotations) == nil {
			tool.Annotations = &annotations
		}
	}
	if len(descriptor.Meta) > 0 {
		tool.Meta = mcpsdk.Meta(descriptor.Meta)
	}
	return tool
}

func (s *Server) nodeToolHandler(name string) mcpsdk.ToolHandler {
	return func(ctx context.Context, request *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		arguments, err := toolArguments(request)
		if err != nil {
			return nil, err
		}
		return s.callNodeTool(ctx, name, arguments)
	}
}

func (s *Server) callNodeTool(ctx context.Context, name string, arguments map[string]any) (*mcpsdk.CallToolResult, error) {
	nodeID, _ := arguments["node_id"].(string)
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return gatewayToolResult(name, nil, errors.New("node_id is required"))
	}
	node, err := s.agentDock.Get(ctx, nodeID)
	if err != nil {
		return gatewayToolResult(name, nil, err)
	}
	if !containsString(node.Capabilities, name) {
		return gatewayToolResult(name, nil, fmt.Errorf("AgentDock node %s does not provide tool %s", nodeID, name))
	}

	mismatch, err := s.nodeToolContractMismatch(ctx, node, name)
	if err != nil {
		return gatewayToolResult(name, nil, err)
	}
	if mismatch != nil {
		details, encodeErr := asMap(mismatch)
		if encodeErr != nil {
			return nil, encodeErr
		}
		return gatewayToolResult(name, details, errors.New(mismatch.Message))
	}

	delete(arguments, "node_id")
	result, err := s.agentDockHub.Invoke(ctx, nodeID, "tool.call", map[string]any{"tool": name, "arguments": arguments})
	return gatewayToolResult(name, result, err)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func truncateRunes(value string, maximum int) string {
	runes := []rune(value)
	if len(runes) <= maximum {
		return value
	}
	return string(runes[:maximum]) + "…"
}

func nodeInputSchema(schema map[string]any) map[string]any {
	encoded, _ := json.Marshal(schema)
	cloned := map[string]any{"type": "object"}
	_ = json.Unmarshal(encoded, &cloned)
	properties, _ := cloned["properties"].(map[string]any)
	if properties == nil {
		properties = make(map[string]any)
		cloned["properties"] = properties
	}
	properties["node_id"] = map[string]any{"type": "string", "description": "Target AgentDock node ID from agentdock_context."}
	required, _ := cloned["required"].([]any)
	for _, value := range required {
		if value == "node_id" {
			return cloned
		}
	}
	cloned["required"] = append(required, "node_id")
	return cloned
}

func toolArguments(request *mcpsdk.CallToolRequest) (map[string]any, error) {
	arguments := map[string]any{}
	if request == nil || request.Params == nil || len(request.Params.Arguments) == 0 || string(request.Params.Arguments) == "null" {
		return arguments, nil
	}
	if err := json.Unmarshal(request.Params.Arguments, &arguments); err != nil {
		return nil, errors.New("tool arguments must be a JSON object")
	}
	return arguments, nil
}

func gatewayToolResult(name string, result map[string]any, err error) (*mcpsdk.CallToolResult, error) {
	if err == nil && result != nil {
		if _, hasContent := result["content"]; hasContent {
			if _, hasErrorFlag := result["isError"]; hasErrorFlag {
				encoded, encodeErr := json.Marshal(result)
				if encodeErr != nil {
					return nil, encodeErr
				}
				var proxied mcpsdk.CallToolResult
				if decodeErr := json.Unmarshal(encoded, &proxied); decodeErr != nil {
					return nil, decodeErr
				}
				return &proxied, nil
			}
		}
	}
	if result == nil {
		result = map[string]any{}
	}
	if err != nil {
		// 保留调用方提供的结构化错误详情，避免契约差异等可操作信息被统一错误包装丢失。
		result["tool"] = name
		result["error"] = err.Error()
	}
	encoded, encodeErr := json.Marshal(map[string]any{
		"isError": err != nil, "structuredContent": result,
		"content": []map[string]any{{"type": "text", "text": prettyJSON(result)}},
	})
	if encodeErr != nil {
		return nil, encodeErr
	}
	var response mcpsdk.CallToolResult
	if decodeErr := json.Unmarshal(encoded, &response); decodeErr != nil {
		return nil, decodeErr
	}
	return &response, nil
}

func prettyJSON(value any) string {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(encoded)
}

func (s *Server) callNexusTool(ctx context.Context, name string, args map[string]any) (map[string]any, error) {
	switch name {
	case "agentdock_context":
		return s.callFleetAgentDockContext(ctx)
	case "workflow_template_manage":
		return s.callWorkflowTemplateManage(ctx, args)
	case "recall_bootstrap":
		maxBytes := intArgument(args, "max_bytes", 12000)
		sections, used, err := s.store.Pack("agentdock", maxBytes)
		if err != nil {
			return nil, err
		}
		compacted := make([]map[string]any, 0, len(sections))
		for _, section := range sections {
			item, mapErr := asMap(section)
			if mapErr != nil {
				return nil, mapErr
			}
			content, _ := item["content"].(string)
			delete(item, "content")
			if boolArgument(args, "include_raw") {
				item["raw_content"] = content
			}
			if !boolArgument(args, "include_body") {
				body, _ := item["body"].(string)
				delete(item, "body")
				if body != "" {
					item["body_excerpt"] = truncateRunes(strings.TrimSpace(body), 320)
				}
			}
			compacted = append(compacted, item)
		}
		return asMap(map[string]any{"ok": true, "bootstrap": true, "compact": !boolArgument(args, "include_body"), "sections": compacted, "count": len(compacted), "bytes": used, "max_bytes": maxBytes, "recall_store": "NexusDock Recall"})
	case "recall_search":
		query := stringArgument(args, "query")
		if query == "" {
			return nil, errors.New("query is required")
		}
		prefix := ""
		if strings.EqualFold(stringArgument(args, "kind"), "card") {
			prefix = "recall/managed/cards"
		}
		results, err := s.store.Search(query, prefix, intArgument(args, "max_results", 20))
		if err != nil {
			return nil, err
		}
		return asMap(map[string]any{"ok": true, "query": query, "results": results, "count": len(results), "recall_store": "NexusDock Recall"})
	case "recall_read":
		path := stringArgument(args, "path")
		if strings.HasPrefix(path, "private-notes/") {
			return nil, errors.New("private notes must be read through private_note_manage")
		}
		memory, err := s.store.Read(path)
		if err != nil {
			return nil, err
		}
		item, err := asMap(memory)
		if err != nil {
			return nil, err
		}
		content, _ := item["content"].(string)
		delete(item, "content")
		if boolArgument(args, "include_raw") {
			item["raw_content"] = content
		}
		return asMap(map[string]any{"ok": true, "recall": item, "recall_store": "NexusDock Recall"})
	case "recall_write":
		return s.callRecallWrite(ctx, args)
	case "recall_maintain":
		return s.callRecallMaintain(ctx, args)
	case "private_note_manage":
		return s.callPrivateNote(ctx, args)
	default:
		return nil, fmt.Errorf("unknown NexusDock tool: %s", name)
	}
}

func centralToolResultMeta(name string, args map[string]any) mcpsdk.Meta {
	if name == "workflow_template_manage" && strings.EqualFold(stringArgument(args, "action"), "match") {
		return centralToolUIResourceMeta(workflowUIResourceURI)
	}
	return nil
}

func (s *Server) callRecallWrite(ctx context.Context, args map[string]any) (map[string]any, error) {
	target, action := strings.ToLower(stringArgument(args, "target")), strings.ToLower(stringArgument(args, "action"))
	dryRun := boolArgument(args, "dry_run")
	confirmed := boolArgument(args, "confirmed")
	previewOnly := dryRun || !confirmed
	if target == "card" {
		var request recall.CardRequest
		if err := decodeMap(args, &request); err != nil {
			return nil, err
		}
		if action == "plan" || (action == "create" && previewOnly) {
			result, err := s.store.CaptureCard(request)
			mapped, mapErr := asMap(result, err)
			if mapErr != nil {
				return nil, mapErr
			}
			mapped["dry_run"] = true
			return mapped, nil
		}
		if action != "create" {
			return nil, errors.New("card only supports plan and create")
		}
		result, err := s.store.WriteCard(request)
		if err == nil {
			s.versions.MarkChanged(ctx)
		}
		return asMap(result, err)
	}
	if target != "markdown" {
		return nil, errors.New("target must be card or markdown")
	}
	path := stringArgument(args, "path")
	if action == "delete" {
		if previewOnly {
			current, err := s.store.Read(path)
			if err != nil {
				return nil, err
			}
			return asMap(map[string]any{"ok": true, "path": path, "dry_run": true, "would_delete": true, "size_bytes": current.SizeBytes})
		}
		err := s.store.Delete(path, true)
		if err == nil {
			s.versions.MarkChanged(ctx)
		}
		return asMap(map[string]any{"ok": err == nil, "path": path, "deleted": err == nil}, err)
	}
	if action == "update_fact" {
		return s.updateRecallFacts(ctx, path, args)
	}
	var request recall.WriteRequest
	if err := decodeMap(args, &request); err != nil {
		return nil, err
	}
	switch action {
	case "plan", "create":
	case "replace":
		request.Overwrite = true
	case "append", "patch", "diff":
		current, err := s.store.Read(path)
		if err != nil {
			return nil, err
		}
		content := current.Content
		if action == "append" {
			content += stringArgument(args, "append")
		} else {
			old, replacement := stringArgument(args, "old"), stringArgument(args, "new")
			if old == "" || !strings.Contains(content, old) {
				return nil, errors.New("patch old text was not found")
			}
			content = strings.Replace(content, old, replacement, 1)
		}
		if action == "diff" {
			return asMap(map[string]any{"ok": true, "path": path, "changed": content != current.Content, "proposed_content": content})
		}
		request.Content, request.Overwrite = content, true
	default:
		return nil, fmt.Errorf("unsupported markdown action: %s", action)
	}
	if action == "plan" || previewOnly {
		preview, err := s.store.PreviewWrite(request)
		if err != nil {
			return nil, err
		}
		return asMap(map[string]any{
			"ok": true, "dry_run": true, "path": preview.Path,
			"proposed_content": preview.ProposedContent, "overwrite": preview.Overwrite,
		})
	}
	result, err := s.store.Write(request)
	if err == nil {
		s.versions.MarkChanged(ctx)
	}
	return asMap(map[string]any{"ok": err == nil, "recall": result, "recall_store": "NexusDock Recall"}, err)
}

func (s *Server) callRecallMaintain(ctx context.Context, args map[string]any) (map[string]any, error) {
	action := strings.ToLower(stringArgument(args, "action"))
	if action == "" {
		action = "list"
	}
	switch action {
	case "list":
		entries, err := s.store.List(stringArgument(args, "prefix"), intArgument(args, "max_entries", 200))
		return asMap(map[string]any{"ok": err == nil, "entries": entries, "count": len(entries)}, err)
	case "lint":
		return s.lintRecall(args)
	case "embedding_status":
		if s.currentEmbedding() == nil {
			return asMap(map[string]any{"ok": true, "enabled": false})
		}
		return asMap(s.currentEmbedding().Status(ctx))
	case "reindex", "reindex_cards":
		if s.currentEmbedding() == nil {
			return nil, errors.New("embedding service is not configured")
		}
		prefix := stringArgument(args, "prefix")
		if action == "reindex_cards" && prefix == "" {
			prefix = "recall/managed/cards"
		}
		result, err := s.currentEmbedding().Reindex(ctx, recall.EmbeddingReindexRequest{Prefix: prefix})
		return asMap(result, err)
	default:
		return nil, fmt.Errorf("unsupported recall maintenance action: %s", action)
	}
}

func (s *Server) updateRecallFacts(ctx context.Context, path string, args map[string]any) (map[string]any, error) {
	if path == "" {
		return nil, errors.New("path is required")
	}
	facts := make(map[string]string)
	if key := stringArgument(args, "key"); key != "" {
		value, exists := args["value"]
		if !exists {
			return nil, errors.New("value is required when key is provided")
		}
		facts[key] = fmt.Sprint(value)
	}
	if values, ok := args["facts"].(map[string]any); ok {
		for key, value := range values {
			if key = strings.TrimSpace(key); key != "" {
				facts[key] = fmt.Sprint(value)
			}
		}
	}
	if len(facts) == 0 {
		return nil, errors.New("provide key/value or facts")
	}
	current, err := s.store.Read(path)
	if err != nil {
		return nil, err
	}
	updated := current.Content
	keys := make([]string, 0, len(facts))
	for key := range facts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	missing := make([]string, 0)
	for _, key := range keys {
		pattern := regexp.MustCompile(`(?m)^(\s*(?:[-*]\s*)?` + regexp.QuoteMeta(key) + `\s*[:=]\s*).*$`)
		if pattern.MatchString(updated) {
			updated = pattern.ReplaceAllStringFunc(updated, func(line string) string {
				matches := pattern.FindStringSubmatch(line)
				return matches[1] + facts[key]
			})
			continue
		}
		if boolArgument(args, "append_if_missing") {
			updated = strings.TrimRight(updated, "\n") + "\n" + key + ": " + facts[key] + "\n"
			continue
		}
		missing = append(missing, key)
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("facts not found: %s", strings.Join(missing, ", "))
	}
	preview := map[string]any{"ok": true, "path": path, "changed": updated != current.Content, "proposed_content": updated}
	if boolArgument(args, "dry_run") || !boolArgument(args, "confirmed") || updated == current.Content {
		preview["dry_run"] = true
		return asMap(preview)
	}
	result, err := s.store.Write(recall.WriteRequest{Path: path, Content: updated, Confirmed: true, Overwrite: true})
	if err == nil {
		s.versions.MarkChanged(ctx)
	}
	return asMap(map[string]any{"ok": err == nil, "path": path, "changed": true, "written": err == nil, "recall": result}, err)
}

func (s *Server) lintRecall(args map[string]any) (map[string]any, error) {
	terms := stringSliceArgument(args, "terms")
	if len(terms) == 0 {
		terms = []string{"Connector", "connector", "CONNECTOR", "connectors", "connector_"}
	}
	entries, err := s.store.List(stringArgument(args, "prefix"), intArgument(args, "max_entries", 200))
	if err != nil {
		return nil, err
	}
	maximum := intArgument(args, "max_findings", 200)
	findings := make([]map[string]any, 0)
	filesScanned := 0
	for _, entry := range entries {
		if len(findings) >= maximum || !strings.HasSuffix(strings.ToLower(entry.Path), ".md") {
			continue
		}
		memory, readErr := s.store.Read(entry.Path)
		if readErr != nil {
			continue
		}
		filesScanned++
		for lineIndex, line := range strings.Split(memory.Content, "\n") {
			for _, term := range terms {
				matched := strings.Contains(line, term)
				if boolArgument(args, "regex") {
					expression, compileErr := regexp.Compile(term)
					if compileErr != nil {
						return nil, fmt.Errorf("invalid lint regular expression %q: %w", term, compileErr)
					}
					matched = expression.MatchString(line)
				}
				if matched {
					findings = append(findings, map[string]any{"path": entry.Path, "line": lineIndex + 1, "term": term, "text": line})
				}
				if len(findings) >= maximum {
					break
				}
			}
		}
	}
	return asMap(map[string]any{"ok": true, "terms": terms, "regex": boolArgument(args, "regex"), "files_scanned": filesScanned, "finding_count": len(findings), "findings": findings, "truncated": len(findings) >= maximum})
}

func (s *Server) callPrivateNote(ctx context.Context, args map[string]any) (map[string]any, error) {
	if s.privateNotes == nil {
		return nil, errors.New("private notes are not configured")
	}
	action := strings.ToLower(stringArgument(args, "action"))
	switch action {
	case "search":
		results, err := s.privateNotes.Search(ctx, stringArgument(args, "query"), intArgument(args, "max_results", 8))
		return asMap(map[string]any{"ok": err == nil, "action": action, "results": results, "count": len(results)}, err)
	case "read":
		result, err := s.privateNotes.Read(stringArgument(args, "path"), intArgument(args, "max_bytes", 256000))
		return asMap(result, err)
	case "write":
		var request privatenotes.WriteRequest
		if err := decodeMap(args, &request); err != nil {
			return nil, err
		}
		result, err := s.privateNotes.Write(request)
		return asMap(result, err)
	case "delete":
		result, err := s.privateNotes.Delete(stringArgument(args, "path"), boolArgument(args, "confirmed"))
		return asMap(result, err)
	case "status":
		result, err := s.privateNotes.Status(ctx, stringArgumentDefault(args, "status_action", "check"))
		return asMap(result, err)
	case "maintain":
		result, err := s.privateNotes.Maintain(ctx, stringArgumentDefault(args, "maintenance_action", "sync-encrypted"))
		return asMap(result, err)
	default:
		return nil, fmt.Errorf("unsupported private note action: %s", action)
	}
}

func asMap(value any, optionalErr ...error) (map[string]any, error) {
	if len(optionalErr) > 0 && optionalErr[0] != nil {
		return nil, optionalErr[0]
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(encoded, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func decodeMap(value map[string]any, destination any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(encoded, destination)
}

func stringArgument(args map[string]any, key string) string {
	value, _ := args[key].(string)
	return strings.TrimSpace(value)
}

func stringArgumentDefault(args map[string]any, key, fallback string) string {
	if value := stringArgument(args, key); value != "" {
		return value
	}
	return fallback
}

func intArgument(args map[string]any, key string, fallback int) int {
	switch value := args[key].(type) {
	case float64:
		if value > 0 {
			return int(value)
		}
	case int:
		if value > 0 {
			return value
		}
	}
	return fallback
}

func boolArgument(args map[string]any, key string) bool {
	value, _ := args[key].(bool)
	return value
}

func stringSliceArgument(args map[string]any, key string) []string {
	var values []any
	switch typed := args[key].(type) {
	case []any:
		values = typed
	case []string:
		values = make([]any, len(typed))
		for index := range typed {
			values[index] = typed[index]
		}
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if text := strings.TrimSpace(fmt.Sprint(value)); text != "" {
			result = append(result, text)
		}
	}
	return result
}
