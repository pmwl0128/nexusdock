import { useEffect, useState, type FormEvent } from 'react';
import { Check, KeyRound, Play, RefreshCw, Search, ShieldAlert, Trash2 } from 'lucide-react';
import { createEnvAction, type EnvActionRequest } from '../../api/env';
import { listDeviceCommands } from '../../api/commands';
import type { DeviceCommand, DeviceSnapshot } from '../../api/types';
import { useCommandPolling } from '../../hooks/useCommandPolling';
import { useDevices } from '../../hooks/useDevices';
import { CommandStatusBadge } from '../devices/StatusBadge';
import { formatTime } from '../devices/model';

type EnvEntry = {
  skill: string;
  name: string;
  kind: string;
  source: string;
  configured: boolean;
  length?: number;
  sha256_prefix?: string;
  updated_at?: string;
  last_verified_at?: string;
  verify_ok?: boolean;
  verify_message?: string;
};

type EnvSkillSummary = { skill: string; vars: EnvEntry[] };
type EnvOutput = {
  ok?: boolean;
  action?: string;
  skill?: string;
  skills?: EnvSkillSummary[];
  vars?: EnvEntry[];
  var?: EnvEntry;
  message?: string;
  result?: unknown;
};

const DEFAULT_SKILLS = ['weread-skills', 'openlist', 'dida365-open-api', 'spotify-web-api'];
const DEFAULT_NAMES: Record<string, string[]> = {
  'weread-skills': ['WEREAD_API_KEY'],
  openlist: ['OPENLIST_URL', 'OPENLIST_TOKEN', 'OPENLIST_SESSION_FILE', 'OPENLIST_INSECURE_TLS'],
  'dida365-open-api': ['DIDA365_ACCESS_TOKEN', 'DIDA365_CLIENT_ID', 'DIDA365_CLIENT_SECRET', 'DIDA365_REDIRECT_URI', 'DIDA365_REGION'],
  'spotify-web-api': ['SPOTIFY_ACCESS_TOKEN', 'SPOTIFY_CLIENT_ID', 'SPOTIFY_EXPIRES_AT', 'SPOTIFY_REDIRECT_URI', 'SPOTIFY_REFRESH_TOKEN', 'SPOTIFY_SCOPES'],
};

export default function EnvManagerPage() {
  const { devices, loading, error, refresh } = useDevices(0);
  const [deviceId, setDeviceId] = useState('');
  const [commands, setCommands] = useState<DeviceCommand[]>([]);
  const [selectedCommandId, setSelectedCommandId] = useState('');
  const [commandsLoading, setCommandsLoading] = useState(false);
  const [notice, setNotice] = useState('');
  const [actionError, setActionError] = useState('');
  const selectedDevice = devices.find(({ device }) => device.id === deviceId);
  const latest = commands[0];
  const { command: polledCommand, loading: polling } = useCommandPolling(selectedCommandId || latest?.id, selectedCommandId ? commands.find((item) => item.id === selectedCommandId) : latest);
  const output = parseEnvOutput(polledCommand?.result?.output);
  const summaries = envSummaries(output);

  useEffect(() => {
    if (!deviceId && devices.length > 0) setDeviceId(devices[0].device.id);
  }, [devices, deviceId]);

  useEffect(() => {
    if (!deviceId) return;
    void loadCommands(deviceId);
  }, [deviceId]);

  async function loadCommands(nextDeviceId = deviceId) {
    if (!nextDeviceId) return;
    setCommandsLoading(true);
    setActionError('');
    try {
      const response = await listDeviceCommands(nextDeviceId);
      const envCommands = response.items
        .filter((item) => item.type === 'env.manage')
        .sort((a, b) => Date.parse(b.created_at) - Date.parse(a.created_at));
      setCommands(envCommands);
      setSelectedCommandId((current) => current && envCommands.some((item) => item.id === current) ? current : '');
    } catch (cause) {
      setActionError(messageOf(cause));
    } finally {
      setCommandsLoading(false);
    }
  }

  async function submit(request: EnvActionRequest) {
    if (!deviceId) return;
    setNotice('');
    setActionError('');
    try {
      const command = await createEnvAction(deviceId, request);
      setNotice(`命令 ${command.id} 已排队`);
      setSelectedCommandId(command.id);
      await loadCommands(deviceId);
    } catch (cause) {
      setActionError(messageOf(cause));
    }
  }

  const canManageEnv = selectedDevice ? selectedDevice.device.policy.allowed_command_types.includes('env.manage') : false;
  const riskOK = selectedDevice ? riskRank(selectedDevice.device.policy.max_risk) >= riskRank('medium') : false;

  return (
    <section className="env-manager-page">
      <div className="section-heading">
        <div><h2>Env Manager</h2><p>按设备管理 Skill 环境变量，secret 只允许覆盖写入，状态输出始终脱敏。</p></div>
        <button type="button" className="nx-button is-secondary" onClick={() => { refresh(); void loadCommands(); }}><RefreshCw size={16} />刷新</button>
      </div>

      {error && <div className="nx-alert is-error">{error}</div>}
      {actionError && <div className="nx-alert is-error">{actionError}</div>}
      {notice && <div className="nx-alert is-success"><Check size={17} />{notice}</div>}

      <div className="env-layout">
        <article className="nexus-panel env-control-panel">
          <header><div><h3>控制</h3><p>选择节点并下发 env.manage 命令。</p></div></header>
          <div className="panel-body env-form-stack">
            <label><span>目标设备</span><select value={deviceId} onChange={(event) => setDeviceId(event.target.value)} disabled={loading || devices.length === 0}>{devices.map(({ device }) => <option key={device.id} value={device.id}>{device.name} · {device.status}</option>)}</select></label>
            {selectedDevice && <div className="env-policy-line"><span className={canManageEnv ? 'is-ok' : 'is-bad'}>{canManageEnv ? '允许 env.manage' : '未允许 env.manage'}</span><span className={riskOK ? 'is-ok' : 'is-bad'}>{riskOK ? 'medium 风险可用' : '风险等级不足'}</span></div>}
            {selectedDevice && (!canManageEnv || !riskOK) && <div className="nx-alert is-warning"><ShieldAlert size={17} />设备策略需要允许 env.manage 且 max_risk 至少为 medium。</div>}
            <EnvActions disabled={!deviceId || !canManageEnv || !riskOK} onSubmit={submit} />
          </div>
        </article>

        <article className="nexus-panel env-state-panel">
          <header><div><h3>变量状态</h3><p>{polledCommand ? `${polledCommand.type} · ${formatTime(polledCommand.created_at)}` : '尚无 env.manage 输出'}</p></div>{polledCommand && <CommandStatusBadge status={polledCommand.status} />}</header>
          <div className="panel-body">
            {polling || commandsLoading ? <p className="nx-muted">正在读取 Env 状态…</p> : summaries.length > 0 ? <EnvSummaryList summaries={summaries} /> : <EnvEmpty onRefresh={() => submit({ action: 'list' })} disabled={!deviceId || !canManageEnv || !riskOK} />}
            {output?.message && <div className={`nx-alert ${output.ok === false ? 'is-error' : 'is-success'}`}>{output.message}</div>}
          </div>
        </article>
      </div>

      <article className="nexus-panel env-history-panel">
        <header><div><h3>最近命令</h3><p>这里只展示 env.manage 命令，payload 中的 value 已脱敏。</p></div></header>
        <div className="panel-body env-history-list">
          {commands.length === 0 ? <p className="nx-muted">还没有 Env 命令。</p> : commands.slice(0, 8).map((command) => (
            <button type="button" key={command.id} className={selectedCommandId === command.id ? 'is-active' : ''} onClick={() => setSelectedCommandId(command.id)}>
              <span><strong>{String(command.payload?.action ?? 'env.manage')}</strong><small>{formatTime(command.created_at)}</small></span>
              <CommandStatusBadge status={command.status} />
            </button>
          ))}
        </div>
      </article>
    </section>
  );
}

function EnvActions({ disabled, onSubmit }: { disabled: boolean; onSubmit: (request: EnvActionRequest) => Promise<void> }) {
  const [skill, setSkill] = useState('weread-skills');
  const [name, setName] = useState('WEREAD_API_KEY');
  const [kind, setKind] = useState<'secret' | 'plain'>('secret');
  const [value, setValue] = useState('');
  const [operation, setOperation] = useState('status');
  const names = DEFAULT_NAMES[skill] ?? [];

  useEffect(() => {
    if (names.length > 0 && !names.includes(name)) setName(names[0]);
  }, [skill, names, name]);

  async function setVariable(event: FormEvent) {
    event.preventDefault();
    await onSubmit({ action: 'set', skill, name, kind, value });
    setValue('');
  }

  return (
    <div className="env-actions">
      <div className="env-action-row">
        <button type="button" className="nx-button is-secondary" disabled={disabled} onClick={() => onSubmit({ action: 'list' })}><Search size={15} />列出</button>
        <button type="button" className="nx-button is-secondary" disabled={disabled || !skill} onClick={() => onSubmit({ action: 'inspect', skill })}>Inspect</button>
        <button type="button" className="nx-button is-secondary" disabled={disabled || !skill} onClick={() => onSubmit({ action: 'verify', skill, operation })}><Play size={15} />Verify</button>
        <button type="button" className="nx-button is-secondary" disabled={disabled} onClick={() => onSubmit({ action: 'migrate-from-agentdock-env' })}>迁移</button>
      </div>
      <form className="env-set-form" onSubmit={setVariable}>
        <label><span>Skill</span><input list="env-skill-list" value={skill} onChange={(event) => setSkill(event.target.value)} /><datalist id="env-skill-list">{DEFAULT_SKILLS.map((item) => <option key={item} value={item} />)}</datalist></label>
        <label><span>变量</span><input list="env-name-list" value={name} onChange={(event) => setName(event.target.value.toUpperCase())} /><datalist id="env-name-list">{names.map((item) => <option key={item} value={item} />)}</datalist></label>
        <label><span>类型</span><select value={kind} onChange={(event) => setKind(event.target.value as 'secret' | 'plain')}><option value="secret">secret</option><option value="plain">plain</option></select></label>
        <label><span>Verify Operation</span><input value={operation} onChange={(event) => setOperation(event.target.value)} /></label>
        <label className="env-value-field"><span>值</span><input type={kind === 'secret' ? 'password' : 'text'} value={value} onChange={(event) => setValue(event.target.value)} autoComplete="off" /></label>
        <div className="env-action-row">
          <button type="submit" className="nx-button" disabled={disabled || !skill || !name || value === ''}><KeyRound size={15} />保存</button>
          <button type="button" className="nx-button is-danger" disabled={disabled || !skill || !name} onClick={() => onSubmit({ action: 'delete', skill, name })}><Trash2 size={15} />删除</button>
        </div>
      </form>
    </div>
  );
}

function EnvSummaryList({ summaries }: { summaries: EnvSkillSummary[] }) {
  return <div className="env-summary-list">{summaries.map((summary) => (
    <div className="env-skill-block" key={summary.skill}>
      <h4>{summary.skill}</h4>
      <div className="env-var-table">{summary.vars.map((item) => <EnvVarRow key={`${item.skill}:${item.name}`} item={item} />)}</div>
    </div>
  ))}</div>;
}

function EnvVarRow({ item }: { item: EnvEntry }) {
  const verify = item.verify_ok === undefined ? '未验证' : item.verify_ok ? 'ok' : 'failed';
  return (
    <div className="env-var-row">
      <div><strong>{item.name}</strong><small>{item.kind} · {item.source || 'registry'}</small></div>
      <span className={item.configured ? 'env-chip is-ok' : 'env-chip'}>{item.configured ? 'configured' : 'missing'}</span>
      <span>{item.length ? `${item.length} chars` : '0 chars'}</span>
      <code>{item.sha256_prefix || 'no hash'}</code>
      <span className={item.verify_ok ? 'env-chip is-ok' : item.verify_ok === false ? 'env-chip is-bad' : 'env-chip'}>{verify}</span>
      <span>{formatTime(item.updated_at || item.last_verified_at)}</span>
    </div>
  );
}

function EnvEmpty({ disabled, onRefresh }: { disabled: boolean; onRefresh: () => void }) {
  return <div className="env-empty"><p>还没有可展示的 Env Registry 状态。</p><button type="button" className="nx-button" disabled={disabled} onClick={onRefresh}>读取状态</button></div>;
}

function parseEnvOutput(value: unknown): EnvOutput | undefined {
  if (!value || typeof value !== 'object') return undefined;
  return value as EnvOutput;
}

function envSummaries(output?: EnvOutput): EnvSkillSummary[] {
  if (!output) return [];
  if (Array.isArray(output.skills)) return output.skills;
  if (output.skill && Array.isArray(output.vars)) return [{ skill: output.skill, vars: output.vars }];
  if (output.skill && output.var) return [{ skill: output.skill, vars: [output.var] }];
  return [];
}

function riskRank(value: string) {
  return value === 'high' ? 3 : value === 'medium' ? 2 : 1;
}

function messageOf(reason: unknown) {
  return reason instanceof Error ? reason.message : '操作失败';
}
