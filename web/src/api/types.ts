export type EntryType = 'file' | 'directory';

export type MemoryEntry = {
  path: string;
  name: string;
  type: EntryType;
  size_bytes?: number;
};

export type Memory = {
  path: string;
  content: string;
};

export type ChangedFile = {
  status: string;
  path: string;
};

export type GitDiff = {
  ok: boolean;
  git_repo: boolean;
  dirty: boolean;
  status: string;
  stat: string;
  diff: string;
  cached_diff: string;
  files?: ChangedFile[];
};

export type GitCommit = {
  hash: string;
  short_hash: string;
  date: string;
  author: string;
  subject: string;
};

export type SyncStatus = Record<string, unknown> & {
  dirty?: boolean;
  ahead?: string;
  behind?: string;
  pending_push?: boolean;
};

export type DeviceStatus = 'pending' | 'online' | 'degraded' | 'offline' | 'revoked';
export type RiskLevel = 'low' | 'medium' | 'high';
export type CommandType =
  | 'health.check'
  | 'skill.install'
  | 'skill.run'
  | 'skill.rollback'
  | 'recall.sync'
  | 'service.inspect'
  | 'service.restart'
  | 'diagnostics.collect'
  | 'agentdock.reload'
  | 'env.manage';
export type CommandStatus = 'queued' | 'leased' | 'running' | 'succeeded' | 'failed' | 'expired' | 'cancelled';

export type DeviceCapability = {
  name: string;
  version?: string;
  enabled: boolean;
  metadata?: Record<string, string>;
};

export type DeviceSkillSummary = {
  name: string;
  version: string;
  channel?: string;
  active: boolean;
};

export type DeviceMemorySyncSummary = {
  last_success_at?: string;
  pending: number;
  conflicts: number;
  last_error?: string;
};

export type DevicePolicy = {
  allowed_command_types: CommandType[];
  max_risk: RiskLevel;
  release_channel: 'stable' | 'canary';
  auto_update: boolean;
  allowed_skills?: string[];
};

export type NexusDevice = {
  id: string;
  name: string;
  platform: string;
  arch: string;
  public_key: string;
  labels: Record<string, string>;
  policy: DevicePolicy;
  status: DeviceStatus;
  agentdock_version?: string;
  capabilities?: DeviceCapability[];
  last_seen?: string;
  approved_at?: string;
  revoked_at?: string;
  created_at: string;
  updated_at: string;
  version: number;
};

export type DeviceHeartbeat = {
  device_id?: string;
  sent_at: string;
  received_at: string;
  uptime_seconds: number;
  agentdock_version: string;
  metrics: { cpu_percent: number; memory_percent: number; disk_percent: number };
  capabilities: DeviceCapability[];
  skills?: DeviceSkillSummary[] | null;
  memory_sync: DeviceMemorySyncSummary;
};

export type DeviceSnapshot = { device: NexusDevice; heartbeat?: DeviceHeartbeat };
export type ItemsResponse<T> = { items: T[] };

export type EnrollmentTokenCreateRequest = {
  created_by: string;
  ttl_seconds: number;
  allowed_command_types: CommandType[];
  max_risk: RiskLevel;
};

export type EnrollmentTokenCreateResponse = { token: string; expires_at: string };
export type DeviceRevokeRequest = { reason: string };

export type CommandProgress = {
  percent: number;
  message?: string;
  details?: unknown;
  updated_at: string;
};

export type CommandResult = {
  success: boolean;
  output?: unknown;
  error_code?: string;
  error?: string;
  retryable: boolean;
  evidence_ids?: string[];
  finished_at: string;
};

export type DeviceCommand = {
  id: string;
  device_id: string;
  type: CommandType;
  risk: RiskLevel;
  payload: Record<string, unknown>;
  status: CommandStatus;
  idempotency_key: string;
  priority: number;
  max_attempts: number;
  attempts?: number;
  attempt?: number;
  not_before?: string;
  expires_at: string;
  lease_expires_at?: string;
  progress?: CommandProgress;
  result?: CommandResult;
  created_by?: string;
  created_at: string;
  updated_at?: string;
  started_at?: string;
  completed_at?: string;
  version?: number;
};

export type DeviceCommandCreateRequest = {
  type: CommandType;
  payload: Record<string, unknown>;
  risk: RiskLevel;
  idempotency_key: string;
  priority: number;
  max_attempts: number;
  not_before: string;
  expires_at: string;
};
