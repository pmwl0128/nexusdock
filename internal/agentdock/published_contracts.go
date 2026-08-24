package agentdock

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// PublishedToolContract 是 Nexus 对外保持稳定的单工具契约。
// 它独立于具体节点生命周期保存，避免来源设备删除或 Nexus 重启导致公开 schema 漂移。
type PublishedToolContract struct {
	ToolName      string
	Descriptor    ToolDescriptor
	SourceNodeID  string
	SourceVersion string
	UpdatedAt     time.Time
}

func (s *Store) ListPublishedToolContracts(ctx context.Context) ([]PublishedToolContract, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT tool_name, descriptor_json, source_node_id, source_version, updated_at
		FROM agentdock_published_tool_contracts ORDER BY tool_name`)
	if err != nil {
		return nil, fmt.Errorf("列出 AgentDock 公开工具契约: %w", err)
	}
	defer rows.Close()

	contracts := make([]PublishedToolContract, 0)
	for rows.Next() {
		var contract PublishedToolContract
		var descriptorJSON, updatedAt string
		if err := rows.Scan(&contract.ToolName, &descriptorJSON, &contract.SourceNodeID, &contract.SourceVersion, &updatedAt); err != nil {
			return nil, fmt.Errorf("读取 AgentDock 公开工具契约: %w", err)
		}
		if err := json.Unmarshal([]byte(descriptorJSON), &contract.Descriptor); err != nil {
			return nil, fmt.Errorf("解析 AgentDock 公开工具契约 %s: %w", contract.ToolName, err)
		}
		if contract.Descriptor.Name != contract.ToolName {
			return nil, fmt.Errorf("AgentDock 公开工具契约名称不一致: %s", contract.ToolName)
		}
		contract.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
		if err != nil {
			return nil, fmt.Errorf("解析 AgentDock 公开工具契约更新时间: %w", err)
		}
		contracts = append(contracts, contract)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 AgentDock 公开工具契约: %w", err)
	}
	return contracts, nil
}

func (s *Store) SavePublishedToolContract(ctx context.Context, contract PublishedToolContract) error {
	contract.ToolName = strings.TrimSpace(contract.ToolName)
	if contract.ToolName == "" || contract.Descriptor.Name != contract.ToolName {
		return invalid("公开工具契约名称无效")
	}
	descriptorJSON, err := json.Marshal(contract.Descriptor)
	if err != nil {
		return fmt.Errorf("编码 AgentDock 公开工具契约 %s: %w", contract.ToolName, err)
	}
	now := s.now().UTC()
	_, err = s.db.ExecContext(ctx, `INSERT INTO agentdock_published_tool_contracts(
		tool_name, descriptor_json, source_node_id, source_version, updated_at
	) VALUES(?, ?, ?, ?, ?) ON CONFLICT(tool_name) DO UPDATE SET
		descriptor_json = excluded.descriptor_json,
		source_node_id = excluded.source_node_id,
		source_version = excluded.source_version,
		updated_at = excluded.updated_at`,
		contract.ToolName, string(descriptorJSON), strings.TrimSpace(contract.SourceNodeID), strings.TrimSpace(contract.SourceVersion), now.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("保存 AgentDock 公开工具契约 %s: %w", contract.ToolName, err)
	}
	return nil
}

func (s *Store) DeletePublishedToolContract(ctx context.Context, toolName string) error {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return invalid("公开工具契约名称无效")
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM agentdock_published_tool_contracts WHERE tool_name = ?`, toolName); err != nil {
		return fmt.Errorf("删除 AgentDock 公开工具契约 %s: %w", toolName, err)
	}
	return nil
}
