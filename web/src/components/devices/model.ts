import type { CommandStatus, CommandType, DeviceStatus, RiskLevel } from '../../api/types';

export const COMMAND_TYPES: CommandType[] = [
  'health.check', 'recall.sync', 'service.inspect', 'service.restart',
  'diagnostics.collect', 'agentdock.reload', 'env.manage',
];

export const TERMINAL_COMMAND_STATUSES = new Set<CommandStatus>(['succeeded', 'failed', 'expired', 'cancelled']);

export const STATUS_LABELS: Record<DeviceStatus, string> = {
  pending: '待审批', online: '在线', degraded: '状态异常', offline: '离线', revoked: '已撤销',
};

export const COMMAND_STATUS_LABELS: Record<CommandStatus, string> = {
  queued: '已排队', leased: '已租用', running: '执行中', succeeded: '成功', failed: '失败', expired: '已过期', cancelled: '已取消',
};

export const COMMAND_LABELS: Record<CommandType, string> = {
  'health.check': '健康检查',
  'skill.install': '安装 Skill',
  'skill.run': '运行 Skill',
  'skill.rollback': '回退 Skill',
  'recall.sync': '同步 Recall',
  'service.inspect': '检查服务',
  'service.restart': '重启服务',
  'diagnostics.collect': '收集诊断',
  'agentdock.reload': '重载 AgentDock',
  'env.manage': '管理 Env',
};

export const DEFAULT_RISK: Record<CommandType, RiskLevel> = {
  'health.check': 'low',
  'skill.install': 'medium',
  'skill.run': 'medium',
  'skill.rollback': 'medium',
  'recall.sync': 'low',
  'service.inspect': 'low',
  'service.restart': 'high',
  'diagnostics.collect': 'low',
  'agentdock.reload': 'high',
  'env.manage': 'medium',
};

export function createIdempotencyKey(): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) return crypto.randomUUID();
  return `nexus-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

export function formatTime(value?: string): string {
  if (!value) return '暂无';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'short', timeStyle: 'medium' }).format(date);
}

export function buildCommandPayload(type: CommandType, fields: Record<string, string | boolean>): Record<string, unknown> {
  switch (type) {
    case 'health.check': return {};
    case 'skill.install':
    case 'skill.run':
    case 'skill.rollback':
      throw new Error('Nexus 人工界面不提供 Skill 生命周期命令');
    case 'recall.sync': return compact({ direction: fields.direction });
    case 'service.inspect':
    case 'service.restart': return compact({ service: fields.service });
    case 'diagnostics.collect': return compact({ scope: fields.scope, include_logs: fields.include_logs });
    case 'env.manage': return compact({ action: fields.action, skill: fields.skill, name: fields.name, kind: fields.kind, value: fields.value, operation: fields.operation, env_file: fields.env_file });
    case 'agentdock.reload': return compact({ reason: fields.reason });
  }
}

function compact(values: Record<string, unknown>): Record<string, unknown> {
  return Object.fromEntries(Object.entries(values).filter(([, value]) => value !== '' && value !== undefined));
}
