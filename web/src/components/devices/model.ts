import type { CommandStatus, CommandType, DeviceStatus, RiskLevel } from '../../api/types';

export const COMMAND_TYPES: CommandType[] = [
  'health.check', 'skill.install', 'skill.run', 'skill.rollback', 'memory.sync',
  'service.inspect', 'service.restart', 'diagnostics.collect', 'agentdock.reload',
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
  'memory.sync': '同步 Memory',
  'service.inspect': '检查服务',
  'service.restart': '重启服务',
  'diagnostics.collect': '收集诊断',
  'agentdock.reload': '重载 AgentDock',
};

export const DEFAULT_RISK: Record<CommandType, RiskLevel> = {
  'health.check': 'low',
  'skill.install': 'medium',
  'skill.run': 'medium',
  'skill.rollback': 'medium',
  'memory.sync': 'low',
  'service.inspect': 'low',
  'service.restart': 'high',
  'diagnostics.collect': 'low',
  'agentdock.reload': 'high',
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
    case 'skill.install': return compact({ skill: fields.skill, version: fields.version, channel: fields.channel });
    case 'skill.run': {
      let input: unknown = {};
      if (typeof fields.input === 'string' && fields.input.trim()) input = JSON.parse(fields.input);
      return compact({ skill: fields.skill, input });
    }
    case 'skill.rollback': return compact({ skill: fields.skill, target_version: fields.target_version });
    case 'memory.sync': return compact({ direction: fields.direction });
    case 'service.inspect':
    case 'service.restart': return compact({ service: fields.service });
    case 'diagnostics.collect': return compact({ scope: fields.scope, include_logs: fields.include_logs });
    case 'agentdock.reload': return compact({ reason: fields.reason });
  }
}

function compact(values: Record<string, unknown>): Record<string, unknown> {
  return Object.fromEntries(Object.entries(values).filter(([, value]) => value !== '' && value !== undefined));
}
