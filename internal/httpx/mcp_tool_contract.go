package httpx

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/uvwt/nexusdock/internal/agentdock"
)

const maxToolContractDifferences = 5

type publishedNodeTool struct {
	Descriptor    agentdock.ToolDescriptor
	ContractHash  string
	SourceNodeID  string
	SourceVersion string
}

type toolContractDifference struct {
	Path      string `json:"path"`
	Published any    `json:"published"`
	Node      any    `json:"node"`
}

type toolContractMismatch struct {
	Code             string                   `json:"code"`
	Message          string                   `json:"message"`
	Tool             string                   `json:"tool"`
	NodeID           string                   `json:"node_id"`
	NodeName         string                   `json:"node_name,omitempty"`
	NodeVersion      string                   `json:"node_version,omitempty"`
	PublishedVersion string                   `json:"published_version,omitempty"`
	PublishedHash    string                   `json:"published_hash"`
	NodeHash         string                   `json:"node_hash"`
	Differences      []toolContractDifference `json:"differences,omitempty"`
}

type comparableToolContract struct {
	InputSchema  map[string]any `json:"inputSchema"`
	OutputSchema map[string]any `json:"outputSchema,omitempty"`
	Meta         map[string]any `json:"_meta,omitempty"`
	Annotations  map[string]any `json:"annotations,omitempty"`
}

func toolContractHash(descriptor agentdock.ToolDescriptor) (string, error) {
	encoded, err := json.Marshal(comparableContract(descriptor))
	if err != nil {
		return "", fmt.Errorf("编码工具 %s 契约: %w", descriptor.Name, err)
	}
	sum := sha256.Sum256(encoded)
	return fmt.Sprintf("sha256:%x", sum), nil
}

func comparableContract(descriptor agentdock.ToolDescriptor) comparableToolContract {
	return comparableToolContract{
		InputSchema: descriptor.InputSchema, OutputSchema: descriptor.OutputSchema,
		Meta: descriptor.Meta, Annotations: descriptor.Annotations,
	}
}

func (s *Server) publishedNodeTool(name string) (publishedNodeTool, bool) {
	s.mcpToolsMu.RLock()
	defer s.mcpToolsMu.RUnlock()
	tool, ok := s.mcpTools[name]
	return tool, ok
}

func (s *Server) loadPublishedNodeTools(ctx context.Context) error {
	contracts, err := s.agentDock.ListPublishedToolContracts(ctx)
	if err != nil {
		return err
	}
	for _, contract := range contracts {
		if _, central := nexusToolNames[contract.ToolName]; central || strings.TrimSpace(contract.ToolName) == "" {
			continue
		}
		hash, err := toolContractHash(contract.Descriptor)
		if err != nil {
			return err
		}
		published := publishedNodeTool{
			Descriptor: contract.Descriptor, ContractHash: hash,
			SourceNodeID: contract.SourceNodeID, SourceVersion: contract.SourceVersion,
		}
		s.mcpServer.AddTool(nodeMCPTool(contract.Descriptor), s.nodeToolHandler(contract.ToolName))
		s.mcpTools[contract.ToolName] = published
	}
	return nil
}

func (s *Server) persistPublishedNodeTool(ctx context.Context, published publishedNodeTool) error {
	if s.agentDock == nil {
		return nil
	}
	return s.agentDock.SavePublishedToolContract(ctx, agentdock.PublishedToolContract{
		ToolName: published.Descriptor.Name, Descriptor: published.Descriptor,
		SourceNodeID: published.SourceNodeID, SourceVersion: published.SourceVersion,
	})
}

func (s *Server) promoteConvergedNodeTool(name string) error {
	if s.agentDock == nil {
		return nil
	}
	ctx := context.Background()
	nodes, err := s.agentDock.List(ctx)
	if err != nil {
		return err
	}
	var candidate publishedNodeTool
	for _, node := range nodes {
		if !node.Enabled || !containsString(node.Capabilities, name) {
			continue
		}
		descriptors, err := s.agentDock.ToolDescriptors(ctx, node.ID)
		if err != nil {
			return err
		}
		descriptor, ok := findToolDescriptor(descriptors, name)
		if !ok {
			return fmt.Errorf("AgentDock node %s does not provide tool descriptor %s", node.ID, name)
		}
		hash, err := toolContractHash(descriptor)
		if err != nil {
			return err
		}
		if candidate.ContractHash == "" {
			candidate = publishedNodeTool{
				Descriptor: descriptor, ContractHash: hash,
				SourceNodeID: node.ID, SourceVersion: node.Version,
			}
			continue
		}
		if hash != candidate.ContractHash {
			return nil
		}
	}
	if candidate.ContractHash == "" {
		return nil
	}

	s.mcpToolsMu.Lock()
	defer s.mcpToolsMu.Unlock()
	published, exists := s.mcpTools[name]
	if !exists || published.ContractHash == candidate.ContractHash {
		return nil
	}
	if err := s.persistPublishedNodeTool(ctx, candidate); err != nil {
		return err
	}
	s.mcpServer.AddTool(nodeMCPTool(candidate.Descriptor), s.nodeToolHandler(name))
	s.mcpTools[name] = candidate
	return nil
}

func (s *Server) reconcileNodeToolContracts(names []string) {
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		if err := s.promoteConvergedNodeTool(name); err != nil && s.logger != nil {
			s.logger.Warn("检查 AgentDock 工具契约收敛失败", "tool", name, "error", err)
		}
	}
}

func toolDescriptorNames(descriptors []agentdock.ToolDescriptor) []string {
	names := make([]string, 0, len(descriptors))
	for _, descriptor := range descriptors {
		if name := strings.TrimSpace(descriptor.Name); name != "" {
			names = append(names, name)
		}
	}
	return names
}

func findToolDescriptor(descriptors []agentdock.ToolDescriptor, name string) (agentdock.ToolDescriptor, bool) {
	for _, descriptor := range descriptors {
		if descriptor.Name == name {
			return descriptor, true
		}
	}
	return agentdock.ToolDescriptor{}, false
}

func (s *Server) nodeToolContractMismatch(ctx context.Context, node agentdock.Node, name string) (*toolContractMismatch, error) {
	published, ok := s.publishedNodeTool(name)
	if !ok {
		return nil, fmt.Errorf("Nexus 公开工具契约不存在: %s", name)
	}
	descriptors, err := s.agentDock.ToolDescriptors(ctx, node.ID)
	if err != nil {
		return nil, err
	}
	target, ok := findToolDescriptor(descriptors, name)
	if !ok {
		return nil, fmt.Errorf("AgentDock node %s does not provide tool descriptor %s", node.ID, name)
	}
	nodeHash, err := toolContractHash(target)
	if err != nil {
		return nil, err
	}
	if nodeHash == published.ContractHash {
		return nil, nil
	}

	return &toolContractMismatch{
		Code:             "TOOL_CONTRACT_MISMATCH",
		Message:          toolContractMismatchMessage(node.Version, published.SourceVersion),
		Tool:             name,
		NodeID:           node.ID,
		NodeName:         node.Name,
		NodeVersion:      node.Version,
		PublishedVersion: published.SourceVersion,
		PublishedHash:    published.ContractHash,
		NodeHash:         nodeHash,
		Differences:      toolContractDifferences(published.Descriptor, target),
	}, nil
}

func toolContractMismatchMessage(nodeVersion, publishedVersion string) string {
	if version, ok := newerAgentDockVersion(nodeVersion, publishedVersion); ok && strings.TrimSpace(nodeVersion) != strings.TrimSpace(publishedVersion) {
		return fmt.Sprintf("AgentDock 设备版本不一致，请将所有设备更新到 %s 后刷新 GPT 工具。", version)
	}
	return "AgentDock 工具契约不一致，请更新所有设备后刷新 GPT 工具。"
}

func newerAgentDockVersion(left, right string) (string, bool) {
	leftParts, leftDisplay, leftOK := parseAgentDockVersion(left)
	rightParts, rightDisplay, rightOK := parseAgentDockVersion(right)
	if !leftOK || !rightOK {
		return "", false
	}
	for index := 0; index < len(leftParts); index++ {
		if leftParts[index] > rightParts[index] {
			return leftDisplay, true
		}
		if leftParts[index] < rightParts[index] {
			return rightDisplay, true
		}
	}
	return leftDisplay, true
}

func parseAgentDockVersion(value string) ([4]int, string, bool) {
	var parsed [4]int
	display := strings.TrimSpace(value)
	normalized := strings.TrimPrefix(strings.TrimPrefix(display, "v"), "V")
	parts := strings.Split(normalized, ".")
	if normalized == "" || len(parts) > len(parsed) {
		return parsed, "", false
	}
	for index, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil || number < 0 {
			return parsed, "", false
		}
		parsed[index] = number
	}
	return parsed, display, true
}

func toolContractDifferences(published, node agentdock.ToolDescriptor) []toolContractDifference {
	publishedValue, publishedOK := normalizedContractValue(published)
	nodeValue, nodeOK := normalizedContractValue(node)
	if !publishedOK || !nodeOK {
		return nil
	}
	differences := make([]toolContractDifference, 0, maxToolContractDifferences)
	collectToolContractDifferences("", publishedValue, nodeValue, &differences)
	return differences
}

func normalizedContractValue(descriptor agentdock.ToolDescriptor) (map[string]any, bool) {
	encoded, err := json.Marshal(comparableContract(descriptor))
	if err != nil {
		return nil, false
	}
	var value map[string]any
	if json.Unmarshal(encoded, &value) != nil {
		return nil, false
	}
	return value, true
}

func collectToolContractDifferences(path string, published, node any, differences *[]toolContractDifference) {
	if len(*differences) >= maxToolContractDifferences || reflect.DeepEqual(published, node) {
		return
	}
	publishedMap, publishedIsMap := published.(map[string]any)
	nodeMap, nodeIsMap := node.(map[string]any)
	if publishedIsMap && nodeIsMap {
		keys := make(map[string]struct{}, len(publishedMap)+len(nodeMap))
		for key := range publishedMap {
			keys[key] = struct{}{}
		}
		for key := range nodeMap {
			keys[key] = struct{}{}
		}
		ordered := make([]string, 0, len(keys))
		for key := range keys {
			ordered = append(ordered, key)
		}
		sort.Strings(ordered)
		for _, key := range ordered {
			nextPath := key
			if path != "" {
				nextPath = path + "." + key
			}
			publishedValue, publishedExists := publishedMap[key]
			nodeValue, nodeExists := nodeMap[key]
			if !publishedExists || !nodeExists {
				*differences = append(*differences, toolContractDifference{Path: nextPath, Published: publishedValue, Node: nodeValue})
			} else {
				collectToolContractDifferences(nextPath, publishedValue, nodeValue, differences)
			}
			if len(*differences) >= maxToolContractDifferences {
				return
			}
		}
		return
	}
	*differences = append(*differences, toolContractDifference{Path: path, Published: published, Node: node})
}
