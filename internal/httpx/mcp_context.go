package httpx

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/uvwt/nexusdock/internal/agentdock"
)

const agentDockContextToolName = "agentdock_context"

type agentDockContext struct {
	Skills            []agentDockContextSkill   `json:"skills"`
	DynamicMCP        []agentDockContextItem    `json:"dynamic_mcp"`
	ACP               *agentDockContextACP      `json:"acp,omitempty"`
	WorkflowTemplates []agentDockContextItem    `json:"workflow_templates"`
	Recall            *agentDockContextRecall   `json:"recall,omitempty"`
	Rules             []string                  `json:"rules"`
	Warnings          []agentDockContextWarning `json:"warnings,omitempty"`
}

type agentDockContextSkill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	File        string `json:"file"`
	Bundled     bool   `json:"bundled,omitempty"`
}

type agentDockContextItem struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type agentDockContextACP struct {
	Enabled     bool   `json:"enabled"`
	Agent       string `json:"agent"`
	Description string `json:"description"`
}

type agentDockContextRecall struct {
	Enabled bool                   `json:"enabled"`
	Items   []agentDockContextItem `json:"items"`
}

type agentDockContextWarning struct {
	Source  string `json:"source"`
	Message string `json:"message"`
}

type fleetAgentDockContext struct {
	Nodes  []fleetAgentDockContextNode `json:"nodes"`
	Shared fleetAgentDockSharedContext `json:"shared"`
}

type fleetAgentDockContextNode struct {
	NodeID       string                     `json:"node_id"`
	Name         string                     `json:"name"`
	Online       bool                       `json:"online"`
	Version      string                     `json:"version,omitempty"`
	OS           string                     `json:"os,omitempty"`
	Arch         string                     `json:"arch,omitempty"`
	Capabilities []string                   `json:"capabilities"`
	Context      *fleetAgentDockNodeContext `json:"context,omitempty"`
	Error        string                     `json:"error,omitempty"`
}

type fleetAgentDockNodeContext struct {
	Skills     []agentDockContextSkill   `json:"skills"`
	DynamicMCP []agentDockContextItem    `json:"dynamic_mcp"`
	ACP        *agentDockContextACP      `json:"acp,omitempty"`
	Warnings   []agentDockContextWarning `json:"warnings,omitempty"`
}

type fleetAgentDockSharedContext struct {
	WorkflowTemplates []agentDockContextItem    `json:"workflow_templates"`
	Recall            *agentDockContextRecall   `json:"recall,omitempty"`
	Rules             []string                  `json:"rules"`
	Warnings          []agentDockContextWarning `json:"warnings,omitempty"`
}

func (s *Server) callFleetAgentDockContext(ctx context.Context) (*mcpsdk.CallToolResult, error) {
	if s.agentDock == nil || s.agentDockHub == nil {
		return gatewayToolResult(agentDockContextToolName, nil, errors.New("AgentDock 节点运行时不可用"))
	}
	nodes, err := s.agentDock.List(ctx)
	if err != nil {
		return gatewayToolResult(agentDockContextToolName, nil, err)
	}
	enabled := make([]agentdock.Node, 0, len(nodes))
	for _, node := range nodes {
		if node.Enabled {
			enabled = append(enabled, node)
		}
	}

	fleet := fleetAgentDockContext{
		Nodes: make([]fleetAgentDockContextNode, len(enabled)),
		Shared: fleetAgentDockSharedContext{
			WorkflowTemplates: []agentDockContextItem{},
			Rules:             []string{},
		},
	}
	providerContexts := make([]*agentDockContext, len(enabled))
	var wait sync.WaitGroup
	for index, node := range enabled {
		index, node := index, node
		fleet.Nodes[index] = fleetAgentDockContextNode{
			NodeID: node.ID, Name: node.Name, Version: node.Version, OS: node.OS, Arch: node.Arch,
			Online: s.agentDockHub.Online(node.ID), Capabilities: append([]string{}, node.Capabilities...),
		}
		if !containsString(node.Capabilities, agentDockContextToolName) {
			fleet.Nodes[index].Error = "节点未提供 agentdock_context"
			continue
		}
		if !fleet.Nodes[index].Online {
			fleet.Nodes[index].Error = agentdock.ErrNodeOffline.Error()
			continue
		}

		wait.Add(1)
		go func() {
			defer wait.Done()
			mismatch, mismatchErr := s.nodeToolContractMismatch(ctx, node, agentDockContextToolName)
			if mismatchErr != nil {
				fleet.Nodes[index].Error = mismatchErr.Error()
				return
			}
			if mismatch != nil {
				fleet.Nodes[index].Error = mismatch.Message
				return
			}
			remote, invokeErr := s.agentDockHub.Invoke(ctx, node.ID, "tool.call", map[string]any{
				"tool": agentDockContextToolName, "arguments": map[string]any{},
			})
			if invokeErr != nil {
				fleet.Nodes[index].Error = invokeErr.Error()
				return
			}
			providerContext, decodeErr := decodeAgentDockContextResult(remote)
			if decodeErr != nil {
				fleet.Nodes[index].Error = decodeErr.Error()
				return
			}
			providerContexts[index] = &providerContext
			fleet.Nodes[index].Context = localAgentDockContext(providerContext)
		}()
	}
	wait.Wait()

	// Provider 调用并行执行，但共享区按稳定节点顺序合并，确保返回顺序和内容可预测。
	for _, providerContext := range providerContexts {
		if providerContext != nil {
			mergeFleetAgentDockSharedContext(&fleet.Shared, *providerContext)
		}
	}
	mapped, err := asMap(fleet)
	if err != nil {
		return nil, err
	}
	return gatewayToolResult(agentDockContextToolName, mapped, nil)
}

func decodeAgentDockContextResult(result map[string]any) (agentDockContext, error) {
	if result == nil {
		return agentDockContext{}, errors.New("agentdock_context 返回空结果")
	}
	if isError, _ := result["isError"].(bool); isError {
		return agentDockContext{}, errors.New(agentDockContextResultError(result))
	}
	structured, ok := result["structuredContent"].(map[string]any)
	if !ok {
		return agentDockContext{}, errors.New("agentdock_context 缺少 structuredContent")
	}
	var decoded agentDockContext
	if err := decodeMap(structured, &decoded); err != nil {
		return agentDockContext{}, fmt.Errorf("解析 agentdock_context: %w", err)
	}
	if decoded.Skills == nil || decoded.DynamicMCP == nil || decoded.WorkflowTemplates == nil || decoded.Rules == nil {
		return agentDockContext{}, errors.New("agentdock_context structuredContent 不符合当前结构化契约")
	}
	return decoded, nil
}

func agentDockContextResultError(result map[string]any) string {
	if structured, ok := result["structuredContent"].(map[string]any); ok {
		for _, key := range []string{"message", "error"} {
			if value, _ := structured[key].(string); strings.TrimSpace(value) != "" {
				return value
			}
		}
	}
	return "agentdock_context 调用失败"
}

func localAgentDockContext(context agentDockContext) *fleetAgentDockNodeContext {
	warnings := make([]agentDockContextWarning, 0, len(context.Warnings))
	for _, warning := range context.Warnings {
		if warning.Source == "workflow_templates" || warning.Source == "recall" {
			continue
		}
		warnings = append(warnings, warning)
	}
	return &fleetAgentDockNodeContext{
		Skills: context.Skills, DynamicMCP: context.DynamicMCP, ACP: context.ACP, Warnings: warnings,
	}
}

func mergeFleetAgentDockSharedContext(shared *fleetAgentDockSharedContext, context agentDockContext) {
	shared.WorkflowTemplates = mergeContextItems(shared.WorkflowTemplates, context.WorkflowTemplates)
	if context.Recall != nil && context.Recall.Enabled {
		if shared.Recall == nil {
			shared.Recall = &agentDockContextRecall{Enabled: true, Items: []agentDockContextItem{}}
		}
		shared.Recall.Items = mergeContextItems(shared.Recall.Items, context.Recall.Items)
	}
	shared.Rules = mergeContextStrings(shared.Rules, context.Rules)
	for _, warning := range context.Warnings {
		if warning.Source != "workflow_templates" && warning.Source != "recall" {
			continue
		}
		duplicate := false
		for _, existing := range shared.Warnings {
			if existing == warning {
				duplicate = true
				break
			}
		}
		if !duplicate {
			shared.Warnings = append(shared.Warnings, warning)
		}
	}
}

func mergeContextItems(existing, incoming []agentDockContextItem) []agentDockContextItem {
	byName := make(map[string]agentDockContextItem, len(existing)+len(incoming))
	for _, item := range append(append([]agentDockContextItem{}, existing...), incoming...) {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		current, exists := byName[name]
		if !exists || (current.Description == "" && item.Description != "") {
			item.Name = name
			byName[name] = item
		}
	}
	items := make([]agentDockContextItem, 0, len(byName))
	for _, item := range byName {
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}

func mergeContextStrings(existing, incoming []string) []string {
	seen := make(map[string]struct{}, len(existing)+len(incoming))
	merged := make([]string, 0, len(existing)+len(incoming))
	for _, value := range append(append([]string{}, existing...), incoming...) {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		merged = append(merged, value)
	}
	return merged
}

func fleetAgentDockContextOutputSchema(providerSchema map[string]any) map[string]any {
	properties, _ := providerSchema["properties"].(map[string]any)
	property := func(name string) any {
		if schema, ok := properties[name]; ok {
			return schema
		}
		return map[string]any{}
	}
	localContextSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"skills": property("skills"), "dynamic_mcp": property("dynamic_mcp"),
			"acp": property("acp"), "warnings": property("warnings"),
		},
		"required": []string{"skills", "dynamic_mcp"}, "additionalProperties": false,
	}
	sharedSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"workflow_templates": property("workflow_templates"), "recall": property("recall"),
			"rules": property("rules"), "warnings": property("warnings"),
		},
		"required": []string{"workflow_templates", "rules"}, "additionalProperties": false,
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"nodes": map[string]any{
				"type": "array", "description": "Enabled AgentDock nodes and their node-local context.",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"node_id": map[string]any{"type": "string"}, "name": map[string]any{"type": "string"},
						"online": map[string]any{"type": "boolean"}, "version": map[string]any{"type": "string"},
						"os": map[string]any{"type": "string"}, "arch": map[string]any{"type": "string"},
						"capabilities": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"context":      localContextSchema, "error": map[string]any{"type": "string"},
					},
					"required": []string{"node_id", "name", "online", "capabilities"}, "additionalProperties": false,
				},
			},
			"shared": sharedSchema,
		},
		"required": []string{"nodes", "shared"}, "additionalProperties": false,
	}
}
