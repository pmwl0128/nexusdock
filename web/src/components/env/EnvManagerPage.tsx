import { useCallback, useEffect, useState, type FormEvent } from 'react';
import { Check, KeyRound, Play, RefreshCw, Search, ShieldAlert, Trash2 } from 'lucide-react';
import { createEnvAction, type EnvActionRequest } from '../../api/env';
import { listDeviceCommands } from '../../api/commands';
import type { DeviceCommand } from '../../api/types';
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

export default function EnvManagerPage({ fixedDeviceId }: { fixedDeviceId?: string }) {
  const { devices, loading, error, refresh } = useDevices(0);
  const [manualDeviceId, setManualDeviceId] = useState('');
  const [commands, setCommands] = useState<DeviceCommand[]>([]);
  const [selectedCommandId, setSelectedCommandId] = useState('');
  const [commandsLoading, setCommandsLoading] = useState(false);
  const [actionBusy, setActionBusy] = useState('');
  const [notice, setNotice] = useState('');
  const [actionError, setActionError] = useState('');
  const deviceId = fixedDeviceId || manualDeviceId || devices[0]?.device.id || '';
  const selectedDevice = devices.find(({ device }) => device.id === deviceId);
  const latest = commands[0];
  const { command: polledCommand, loading: polling } = useCommandPolling(selectedCommandId || latest?.id, selectedCommandId ? commands.find((item) => item.id === selectedCommandId) : latest);
  const output = parseEnvOutput(polledCommand?.result?.output);
  const summaries = envSummaries(output);

  const loadCommands = useCallback(async (nextDeviceId = deviceId) => {
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
  }, [deviceId]);

  useEffect(() => {
    if (!deviceId) return;
    void loadCommands(deviceId);
  }, [deviceId, loadCommands]);

  async function submit(request: EnvActionRequest): Promise<boolean> {
    if (!deviceId || actionBusy) return false;
    setActionBusy(request.action);
    setNotice('');
    setActionError('');
    try {
      const command = await createEnvAction(deviceId, request);
      setNotice(`命令 ${command.id} 已排队`);
      setSelectedCommandId(command.id);
      await loadCommands(deviceId);
      return true;
    } catch (cause) {
      setActionError(messageOf(cause));
      return false;
    } finally {
      setActionBusy('');
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
            {fixedDeviceId ? <div className="env-fixed-device"><span>目标设备</span><strong>{selectedDevice?.device.name || fixedDeviceId}</strong></div> : <label><span>目标设备</span><select value={deviceId} onChange={(event) => setManualDeviceId(event.target.value)} disabled={loading || devices.length === 0}>{devices.map(({ device }) => <option key={device.id} value={device.id}>{device.name} · {device.status}</option>)}</select></label>}
            {selectedDevice && <div className="env-policy-line"><span className={canManageEnv ? 'is-ok' : 'is-bad'}>{canManageEnv ? '允许 env.manage' : '未允许 env.manage'}</span><span className={riskOK ? 'is-ok' : 'is-bad'}>{riskOK ? 'medium 风险可用' : '风险等级不足'}</span></div>}
            {selectedDevice && (!canManageEnv || !riskOK) && <div className="nx-alert is-warning"><ShieldAlert size={17} />设备策略需要允许 env.manage 且 max_risk 至少为 medium。</div>}
            <EnvActions summaries={summaries} disabled={!deviceId || !canManageEnv || !riskOK || Boolean(actionBusy)} onSubmit={submit} />
          </div>
        </article>

        <article className="nexus-panel env-state-panel">
          <header><div><h3>变量状态</h3><p>{polledCommand ? `${polledCommand.type} · ${formatTime(polledCommand.created_at)}` : '尚无 env.manage 输出'}</p></div>{polledCommand && <CommandStatusBadge status={polledCommand.status} />}</header>
          <div className="panel-body">
            {polling || commandsLoading ? <p className="nx-muted">正在读取 Env 状态…</p> : summaries.length > 0 ? <EnvSummaryList summaries={summaries} /> : <EnvEmpty onRefresh={() => void submit({ action: 'list' })} disabled={!deviceId || !canManageEnv || !riskOK || Boolean(actionBusy)} />}
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

function EnvActions({ summaries, disabled, onSubmit }: {
  summaries: EnvSkillSummary[];
  disabled: boolean;
  onSubmit: (request: EnvActionRequest) => Promise<boolean>;
}) {
  const skills = summaries.flatMap((item) => item.skill ? [item.skill] : []);
  const [selectedSkill, setSelectedSkill] = useState('');
  const [selectedName, setSelectedName] = useState('');
  const [kindOverride, setKindOverride] = useState<'secret' | 'plain' | ''>('');
  const [value, setValue] = useState('');
  const [operation, setOperation] = useState('status');
  const skill = selectedSkill && skills.includes(selectedSkill) ? selectedSkill : skills[0] || '';
  const entries = summaries.find((item) => item.skill === skill)?.vars ?? [];
  const names = entries.flatMap((item) => item.name ? [item.name] : []);
  const name = selectedName && names.includes(selectedName) ? selectedName : names[0] || '';
  const selectedEntry = entries.find((item) => item.name === name);
  const defaultKind = selectedEntry?.kind === 'plain' || selectedEntry?.kind === 'secret' ? selectedEntry.kind : 'secret';
  const kind = kindOverride || defaultKind;

  function chooseSkill(nextSkill: string) {
    const nextEntries = summaries.find((item) => item.skill === nextSkill)?.vars ?? [];
    const nextName = nextEntries[0]?.name || '';
    setSelectedSkill(nextSkill);
    setSelectedName(nextName);
    setKindOverride('');
  }

  function chooseName(nextName: string) {
    setSelectedName(nextName);
    setKindOverride('');
  }

  async function setVariable(event: FormEvent) {
    event.preventDefault();
    const ok = await onSubmit({ action: 'set', skill, name, kind, value });
    if (ok) setValue('');
  }

  const registryUnavailable = summaries.length === 0;
  return (
    <div className="env-actions">
      {registryUnavailable && <div className="nx-alert is-info">先读取设备 Env Registry，再从 Runtime 上报的 Skill 和变量定义中选择。</div>}
      <div className="env-action-row">
        <button type="button" className="nx-button is-secondary" disabled={disabled} onClick={() => void onSubmit({ action: 'list' })}><Search size={15} />读取 Registry</button>
        <button type="button" className="nx-button is-secondary" disabled={disabled || !skill} onClick={() => void onSubmit({ action: 'inspect', skill })}>查看 Skill</button>
        <button type="button" className="nx-button is-secondary" disabled={disabled || !skill} onClick={() => void onSubmit({ action: 'verify', skill, operation })}><Play size={15} />验证</button>
        <button type="button" className="nx-button is-secondary" disabled={disabled} onClick={() => void onSubmit({ action: 'migrate-from-agentdock-env' })}>迁移旧配置</button>
      </div>
      <form className="env-set-form" onSubmit={setVariable}>
        <label><span>Skill</span><select value={skill} onChange={(event) => chooseSkill(event.target.value)} disabled={registryUnavailable}>{skills.map((item) => <option key={item} value={item}>{item}</option>)}</select></label>
        <label><span>变量</span><select value={name} onChange={(event) => chooseName(event.target.value)} disabled={!skill || names.length === 0}>{names.map((item) => <option key={item} value={item}>{item}</option>)}</select></label>
        <label><span>类型</span><select value={kind} onChange={(event) => setKindOverride(event.target.value as 'secret' | 'plain')} disabled={!name}><option value="secret">secret</option><option value="plain">plain</option></select></label>
        <label><span>验证 Operation</span><input value={operation} onChange={(event) => setOperation(event.target.value)} disabled={!skill} /></label>
        <label className="env-value-field"><span>值</span><input type={kind === 'secret' ? 'password' : 'text'} value={value} onChange={(event) => setValue(event.target.value)} autoComplete="off" disabled={!name} /></label>
        <div className="env-action-row">
          <button type="submit" className="nx-button" disabled={disabled || !skill || !name || value === ''}><KeyRound size={15} />保存</button>
          <button type="button" className="nx-button is-danger" disabled={disabled || !skill || !name} onClick={() => void onSubmit({ action: 'delete', skill, name })}><Trash2 size={15} />删除</button>
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
