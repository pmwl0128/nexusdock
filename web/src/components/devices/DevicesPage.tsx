import { useEffect, useMemo, useState } from 'react';
import {
  Check, ChevronRight, Clipboard, Clock3, History, KeyRound,
  Play, RefreshCw, Search, Server, ShieldAlert, ShieldCheck, TerminalSquare, Trash2,
} from 'lucide-react';
import { approveDevice, createEnrollmentToken, revokeDevice } from '../../api/devices';
import { createDeviceCommand, listDeviceCommands } from '../../api/commands';
import type {
  CommandType, DeviceCommand, DeviceCommandCreateRequest, DeviceSnapshot,
  EnrollmentTokenCreateRequest, EnrollmentTokenCreateResponse, RiskLevel,
} from '../../api/types';
import { useCommandPolling } from '../../hooks/useCommandPolling';
import { useDevices } from '../../hooks/useDevices';
import Dialog from '../Dialog';
import EnvManagerPage from '../env/EnvManagerPage';
import { CommandStatusBadge, DeviceStatusBadge } from './StatusBadge';
import {
  buildCommandPayload, COMMAND_LABELS, COMMAND_TYPES, createIdempotencyKey,
  DEFAULT_RISK, formatTime, TERMINAL_COMMAND_STATUSES,
} from './model';

const RISK_RANK: Record<RiskLevel, number> = { low: 1, medium: 2, high: 3 };

export default function DevicesPage({ refreshToken }: { refreshToken: number }) {
  const { devices, loading, error, refresh } = useDevices(refreshToken);
  const [query, setQuery] = useState('');
  const [status, setStatus] = useState('all');
  const [selected, setSelected] = useState<DeviceSnapshot>();
  const [dialog, setDialog] = useState<'enroll' | 'approve' | 'revoke' | 'command' | 'history' | null>(null);
  const [notice, setNotice] = useState('');

  useEffect(() => {
    if (!selected) return;
    const current = devices.find((item) => item.device.id === selected.device.id);
    if (current) setSelected(current);
  }, [devices]);

  const filtered = useMemo(() => {
    const normalized = query.trim().toLowerCase();
    return devices.filter(({ device }) => {
      if (status !== 'all' && device.status !== status) return false;
      if (!normalized) return true;
      return [device.name, device.id, device.platform, device.arch, device.agentdock_version]
        .filter(Boolean).some((value) => String(value).toLowerCase().includes(normalized));
    });
  }, [devices, query, status]);

  function open(next: typeof dialog, snapshot?: DeviceSnapshot) {
    setSelected(snapshot);
    setDialog(next);
    setNotice('');
  }

  function completed(message: string) {
    setNotice(message);
    setDialog(null);
    refresh();
  }

  return (
    <section className="nx-devices-page">
      <div className="section-heading nx-devices-heading">
        <div><h2>设备控制面</h2><p>完成设备注册、审批、心跳状态、命令下发和结果追踪闭环。</p></div>
        <div className="nx-heading-actions">
          <button type="button" className="nx-button is-secondary" onClick={refresh}><RefreshCw size={16} />刷新</button>
          <button type="button" className="nx-button" onClick={() => open('enroll')}><KeyRound size={16} />创建注册 Token</button>
        </div>
      </div>

      {notice && <div className="nx-alert is-success" role="status"><Check size={17} />{notice}</div>}
      {error && <div className="nx-alert is-error" role="alert"><ShieldAlert size={17} />{error}</div>}

      <div className="nx-device-toolbar">
        <label className="nx-filter-search"><Search size={16} /><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索名称、ID、平台或版本" aria-label="搜索设备" /></label>
        <label><span>状态</span><select value={status} onChange={(event) => setStatus(event.target.value)}><option value="all">全部状态</option><option value="pending">待审批</option><option value="online">在线</option><option value="degraded">异常</option><option value="offline">离线</option><option value="revoked">已撤销</option></select></label>
        <span className="nx-device-count">{filtered.length} / {devices.length} 台设备</span>
      </div>

      {loading ? <DeviceEmpty icon={<RefreshCw className="nx-spin" />} title="正在读取设备" text="正在从 Nexus 控制面加载设备和心跳快照。" />
        : filtered.length === 0 ? <DeviceEmpty icon={<Server />} title="暂无匹配设备" text={devices.length === 0 ? '创建一次性注册 Token，让 AgentDock 节点完成 enrollment。' : '调整状态筛选或搜索条件。'} />
          : <div className="nx-device-grid">{filtered.map((snapshot) => <DeviceCard key={snapshot.device.id} snapshot={snapshot} onDetails={() => setSelected(snapshot)} onAction={(next) => open(next, snapshot)} />)}</div>}

      {selected && !dialog && <DeviceDetails snapshot={selected} onClose={() => setSelected(undefined)} onAction={(next) => open(next, selected)} />}
      {dialog === 'enroll' && <EnrollmentDialog onClose={() => setDialog(null)} onComplete={() => completed('一次性注册 Token 已创建。')} />}
      {dialog === 'approve' && selected && <ApproveDialog snapshot={selected} onClose={() => setDialog(null)} onComplete={() => completed(`设备 ${selected.device.name} 已批准，等待真实心跳后转为在线。`)} />}
      {dialog === 'revoke' && selected && <RevokeDialog snapshot={selected} onClose={() => setDialog(null)} onComplete={() => completed(`设备 ${selected.device.name} 已撤销。`)} />}
      {dialog === 'command' && selected && <CommandCreateDialog snapshot={selected} onClose={() => setDialog(null)} onComplete={(command) => { setDialog('history'); setNotice(`命令 ${command.id} 已创建。`); }} />}
      {dialog === 'history' && selected && <CommandHistoryDialog snapshot={selected} onClose={() => setDialog(null)} onCreate={() => setDialog('command')} />}
    </section>
  );
}

function DeviceCard({ snapshot, onDetails, onAction }: {
  snapshot: DeviceSnapshot;
  onDetails: () => void;
  onAction: (dialog: 'approve' | 'revoke' | 'command' | 'history') => void;
}) {
  const { device, heartbeat } = snapshot;
  const capabilities = heartbeat?.capabilities ?? device.capabilities ?? [];
  const skills = Array.isArray(heartbeat?.skills) ? heartbeat.skills : [];
  const skillSummary = Array.isArray(heartbeat?.skills) ? `${skills.filter((skill) => skill.active).length} / ${skills.length}` : '未上报';
  return (
    <article className="nx-device-card">
      <div className="nx-device-card-head"><span className="entity-avatar"><Server size={20} /></span><DeviceStatusBadge status={device.status} /></div>
      <button type="button" className="nx-card-title" onClick={onDetails}><strong>{device.name}</strong><ChevronRight size={16} /></button>
      <p className="nx-mono nx-device-id">{device.id}</p>
      <div className="nx-device-meta"><span>{device.platform || '未知平台'} / {device.arch || '未知架构'}</span><span>AgentDock {heartbeat?.agentdock_version || device.agentdock_version || '未知版本'}</span></div>
      <div className="nx-capability-list">{capabilities.length ? capabilities.slice(0, 5).map((capability) => <span key={capability.name} className={capability.enabled ? 'is-on' : 'is-off'}>{capability.name}</span>) : <span className="is-off">暂无能力</span>}</div>
      <dl className="nx-device-stats"><div><dt>Skill</dt><dd>{skillSummary}</dd></div><div><dt>最后心跳</dt><dd>{formatTime(device.last_seen || heartbeat?.received_at)}</dd></div></dl>
      <div className="nx-card-actions">
        {device.status === 'pending' && <button type="button" className="nx-button is-small" onClick={() => onAction('approve')}><ShieldCheck size={15} />批准</button>}
        {!['pending', 'revoked'].includes(device.status) && <button type="button" className="nx-button is-small" onClick={() => onAction('command')}><Play size={15} />命令</button>}
        <button type="button" className="nx-button is-secondary is-small" onClick={() => onAction('history')}><History size={15} />历史</button>
        {device.status !== 'revoked' && <button type="button" className="nx-button is-danger is-small" onClick={() => onAction('revoke')}><Trash2 size={15} />撤销</button>}
      </div>
    </article>
  );
}

function DeviceDetails({ snapshot, onClose, onAction }: {
  snapshot: DeviceSnapshot;
  onClose: () => void;
  onAction: (dialog: 'approve' | 'revoke' | 'command' | 'history') => void;
}) {
  const { device, heartbeat } = snapshot;
  const capabilities = heartbeat?.capabilities ?? device.capabilities ?? [];
  const skills = Array.isArray(heartbeat?.skills) ? heartbeat.skills : [];
  const [tab, setTab] = useState<'overview' | 'env'>('overview');
  const metrics = heartbeat?.metrics;
  const hasReportedMetrics = Boolean(metrics && (metrics.cpu_percent > 0 || metrics.memory_percent > 0 || metrics.disk_percent > 0));
  return <Dialog title={device.name} description={device.id} onClose={onClose} wide>
    <div className="nx-device-detail-tabs"><button type="button" className={tab === 'overview' ? 'is-active' : ''} onClick={() => setTab('overview')}>概览与能力</button><button type="button" className={tab === 'env' ? 'is-active' : ''} onClick={() => setTab('env')}>环境变量</button></div>
    {tab === 'env' ? <EnvManagerPage fixedDeviceId={device.id} /> : <>
    <div className="nx-detail-header"><DeviceStatusBadge status={device.status} /><span>更新于 {formatTime(device.updated_at)}</span></div>
    <div className="nx-detail-grid">
      <Detail label="平台 / 架构" value={`${device.platform} / ${device.arch}`} />
      <Detail label="AgentDock 版本" value={heartbeat?.agentdock_version || device.agentdock_version || '未知'} />
      <Detail label="注册时间" value={formatTime(device.created_at)} />
      <Detail label="最近心跳" value={formatTime(device.last_seen || heartbeat?.received_at)} />
      <Detail label="批准时间" value={formatTime(device.approved_at)} />
      <Detail label="资源版本" value={String(device.version)} />
    </div>
    <h3 className="nx-subtitle">能力</h3>
    <div className="nx-capability-table">{capabilities.length ? capabilities.map((item) => <div key={item.name}><span className={item.enabled ? 'nx-dot is-on' : 'nx-dot'} /><strong>{item.name}</strong><span>{item.version || '无版本'}</span><span>{item.enabled ? '已启用' : '未启用'}</span></div>) : <p className="nx-muted">尚未上报能力。</p>}</div>
    <h3 className="nx-subtitle">Skills</h3>
    <div className="nx-capability-table">{skills.length ? skills.map((item) => <div key={item.name}><span className={item.active ? 'nx-dot is-on' : 'nx-dot'} /><strong>{item.name}</strong><span>{item.version || '无版本'}</span><span>{item.active ? `已启用${item.channel ? ` · ${item.channel}` : ''}` : '未启用'}</span></div>) : <p className="nx-muted">尚未上报 Skill。</p>}</div>
    {heartbeat && <><h3 className="nx-subtitle">资源指标</h3><div className="nx-metrics"><Metric label="CPU" value={hasReportedMetrics ? metrics?.cpu_percent : undefined} /><Metric label="内存" value={hasReportedMetrics ? metrics?.memory_percent : undefined} /><Metric label="磁盘" value={hasReportedMetrics ? metrics?.disk_percent : undefined} /></div></>}
    <h3 className="nx-subtitle">策略</h3>
    <div className="nx-policy-box"><span>最大风险：{device.policy.max_risk}</span><span>发布通道：{device.policy.release_channel}</span><span>命令：{device.policy.allowed_command_types.join('、') || '无'}</span></div>
    <div className="nx-dialog-actions">
      {device.status === 'pending' && <button type="button" className="nx-button" onClick={() => onAction('approve')}>批准设备</button>}
      {!['pending', 'revoked'].includes(device.status) && <button type="button" className="nx-button" onClick={() => onAction('command')}>下发命令</button>}
      <button type="button" className="nx-button is-secondary" onClick={() => onAction('history')}>命令历史</button>
      {device.status !== 'revoked' && <button type="button" className="nx-button is-danger" onClick={() => onAction('revoke')}>撤销设备</button>}
    </div>
    </>}
  </Dialog>;
}

function EnrollmentDialog({ onClose, onComplete }: { onClose: () => void; onComplete: () => void }) {
  const [request, setRequest] = useState<EnrollmentTokenCreateRequest>({ created_by: 'nexus-web', ttl_seconds: 3600, allowed_command_types: ['health.check', 'diagnostics.collect'], max_risk: 'low' });
  const [result, setResult] = useState<EnrollmentTokenCreateResponse>();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [copyError, setCopyError] = useState('');
  const [copied, setCopied] = useState(false);

  function close() {
    setResult(undefined);
    setRequest((value) => ({ ...value, created_by: '', allowed_command_types: [] }));
    onClose();
  }

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    setLoading(true); setError('');
    try { setResult(await createEnrollmentToken(request)); }
    catch (reason) { setError(messageOf(reason)); }
    finally { setLoading(false); }
  }

  async function copy() {
    if (!result) return;
    setCopyError('');
    try {
      await navigator.clipboard.writeText(result.token);
      setCopied(true);
    } catch {
      setCopyError('浏览器拒绝访问剪贴板，请手动复制 Token。');
    }
  }

  return <Dialog title="创建一次性注册 Token" description="明文 Token 仅在本对话框中显示一次，关闭后立即清除。" onClose={close}>
    {result ? <div className="nx-token-result">
      <div className="nx-alert is-warning"><ShieldAlert size={17} />请立即复制。关闭后无法恢复此 Token。</div>
      <label><span>注册 Token</span><div className="nx-copy-field"><code>{result.token}</code><button type="button" className="nx-icon-button" onClick={copy} aria-label="复制 Token">{copied ? <Check size={18} /> : <Clipboard size={18} />}</button></div></label>
      {copyError && <div className="nx-alert is-error">{copyError}</div>}
      <p>过期时间：{formatTime(result.expires_at)}</p>
      <div className="nx-dialog-actions"><button type="button" className="nx-button" onClick={() => { onComplete(); setResult(undefined); }}>完成</button></div>
    </div> : <form onSubmit={submit} className="nx-form">
      <label><span>创建主体</span><input required value={request.created_by} onChange={(event) => setRequest({ ...request, created_by: event.target.value })} /></label>
      <label><span>有效期（秒）</span><input type="number" min={60} max={604800} required value={request.ttl_seconds} onChange={(event) => setRequest({ ...request, ttl_seconds: Number(event.target.value) })} /></label>
      <label><span>最大风险</span><select value={request.max_risk} onChange={(event) => setRequest({ ...request, max_risk: event.target.value as RiskLevel })}><option value="low">low</option><option value="medium">medium</option><option value="high">high</option></select></label>
      <fieldset><legend>允许的命令类型</legend><div className="nx-check-grid">{COMMAND_TYPES.map((type) => <label key={type}><input type="checkbox" checked={request.allowed_command_types.includes(type)} onChange={(event) => setRequest({ ...request, allowed_command_types: event.target.checked ? [...request.allowed_command_types, type] : request.allowed_command_types.filter((item) => item !== type) })} />{COMMAND_LABELS[type]}</label>)}</div></fieldset>
      {error && <div className="nx-alert is-error">{error}</div>}
      <div className="nx-dialog-actions"><button type="button" className="nx-button is-secondary" onClick={close}>取消</button><button type="submit" className="nx-button" disabled={loading || request.allowed_command_types.length === 0}>{loading ? '创建中…' : '创建 Token'}</button></div>
    </form>}
  </Dialog>;
}

function ApproveDialog({ snapshot, onClose, onComplete }: ActionDialogProps) {
  const [loading, setLoading] = useState(false); const [error, setError] = useState('');
  const device = snapshot.device;
  async function submit() { setLoading(true); setError(''); try { await approveDevice(device.id); onComplete(); } catch (reason) { setError(messageOf(reason)); setLoading(false); } }
  return <Dialog title="批准设备" description="批准只授予设备凭据；状态必须在节点真实上报心跳后才会变为在线。" onClose={onClose}>
    <div className="nx-confirm-card"><Detail label="设备" value={device.name} /><Detail label="平台 / 架构" value={`${device.platform} / ${device.arch}`} /><Detail label="公钥摘要" value={fingerprint(device.public_key)} /><Detail label="注册时间" value={formatTime(device.created_at)} /></div>
    {error && <div className="nx-alert is-error">{error}</div>}
    <div className="nx-dialog-actions"><button type="button" className="nx-button is-secondary" onClick={onClose}>取消</button><button type="button" className="nx-button" onClick={submit} disabled={loading}>{loading ? '批准中…' : '确认批准'}</button></div>
  </Dialog>;
}

function RevokeDialog({ snapshot, onClose, onComplete }: ActionDialogProps) {
  const [reason, setReason] = useState(''); const [confirmed, setConfirmed] = useState(false); const [loading, setLoading] = useState(false); const [error, setError] = useState('');
  async function submit(event: React.FormEvent) { event.preventDefault(); setLoading(true); setError(''); try { await revokeDevice(snapshot.device.id, reason); onComplete(); } catch (cause) { setError(messageOf(cause)); setLoading(false); } }
  return <Dialog title="撤销设备" description="该操作会立即使设备凭据失效，并禁止继续租用命令。" onClose={onClose}>
    <form className="nx-form" onSubmit={submit}><div className="nx-alert is-warning"><ShieldAlert size={17} />高风险操作：撤销后需要重新注册设备。</div><label><span>撤销原因</span><textarea required minLength={1} maxLength={1000} rows={4} value={reason} onChange={(event) => setReason(event.target.value)} /></label><label className="nx-confirm-check"><input type="checkbox" checked={confirmed} onChange={(event) => setConfirmed(event.target.checked)} />我确认撤销 {snapshot.device.name}</label>{error && <div className="nx-alert is-error">{error}</div>}<div className="nx-dialog-actions"><button type="button" className="nx-button is-secondary" onClick={onClose}>取消</button><button type="submit" className="nx-button is-danger" disabled={!confirmed || loading}>{loading ? '撤销中…' : '确认撤销'}</button></div></form>
  </Dialog>;
}

function CommandCreateDialog({ snapshot, onClose, onComplete }: { snapshot: DeviceSnapshot; onClose: () => void; onComplete: (command: DeviceCommand) => void }) {
  const allowed = COMMAND_TYPES.filter((type) => type !== 'env.manage' && snapshot.device.policy.allowed_command_types.includes(type) && RISK_RANK[DEFAULT_RISK[type]] <= RISK_RANK[snapshot.device.policy.max_risk]);
  const [type, setType] = useState<CommandType>(allowed[0] ?? 'health.check');
  const [fields, setFields] = useState<Record<string, string | boolean>>({ direction: 'bidirectional', scope: 'standard', include_logs: true });
  const [idempotencyKey] = useState(createIdempotencyKey);
  const [confirmed, setConfirmed] = useState(false);
  const [loading, setLoading] = useState(false); const [error, setError] = useState('');

  function changeType(next: CommandType) { setType(next); setConfirmed(false); }
  async function submit(event: React.FormEvent) {
    event.preventDefault(); setLoading(true); setError('');
    try {
      const now = new Date();
      const risk = DEFAULT_RISK[type];
      const request: DeviceCommandCreateRequest = { type, payload: buildCommandPayload(type, fields), risk, idempotency_key: idempotencyKey, priority: 0, max_attempts: 1, not_before: now.toISOString(), expires_at: new Date(now.getTime() + 5 * 60 * 1000).toISOString() };
      onComplete(await createDeviceCommand(snapshot.device.id, request));
    } catch (cause) { setError(messageOf(cause)); setLoading(false); }
  }

  const highRisk = DEFAULT_RISK[type] === 'high';
  return <Dialog title={`向 ${snapshot.device.name} 下发命令`} description="仅允许公共契约定义的结构化命令，不提供 Shell 输入。" onClose={onClose} wide>
    <form className="nx-form" onSubmit={submit}>
      {snapshot.device.status === 'offline' && <div className="nx-alert is-warning"><Clock3 size={17} />设备离线，命令只能排队，不能视为已执行。</div>}
      {!allowed.length && <div className="nx-alert is-error">设备策略未允许任何可用命令。</div>}
      <label><span>操作</span><select value={type} onChange={(event) => changeType(event.target.value as CommandType)} disabled={!allowed.length}>{allowed.map((item) => <option key={item} value={item}>{COMMAND_LABELS[item]}</option>)}</select></label>
      <div className="nx-alert is-info">风险、有效期和重试策略由 Nexus 使用安全默认值，不在人工界面中调整。</div>
      <CommandFields type={type} fields={fields} setFields={setFields} />
      {highRisk && <><div className="nx-alert is-warning"><ShieldAlert size={17} />高风险命令可能导致服务中断，后端策略仍会执行最终校验。</div><label className="nx-confirm-check"><input type="checkbox" checked={confirmed} onChange={(event) => setConfirmed(event.target.checked)} />我确认执行此高风险命令</label></>}
      {error && <div className="nx-alert is-error">{error}</div>}
      <div className="nx-dialog-actions"><button type="button" className="nx-button is-secondary" onClick={onClose}>取消</button><button type="submit" className="nx-button" disabled={loading || !allowed.length || (highRisk && !confirmed)}>{loading ? '创建中…' : '创建命令'}</button></div>
    </form>
  </Dialog>;
}

function CommandFields({ type, fields, setFields }: { type: CommandType; fields: Record<string, string | boolean>; setFields: (value: Record<string, string | boolean>) => void }) {
  const field = (key: string, value: string | boolean) => setFields({ ...fields, [key]: value });
  if (type === 'health.check') return <p className="nx-muted">健康检查无需额外参数。</p>;
  if (type === 'memory.sync') return <label><span>同步方向</span><select value={String(fields.direction ?? 'bidirectional')} onChange={(event) => field('direction', event.target.value)}><option value="bidirectional">双向</option><option value="pull">拉取</option><option value="push">推送</option></select></label>;
  if (type === 'service.inspect' || type === 'service.restart') return <TextField label="受控服务名" required value={String(fields.service ?? '')} onChange={(value) => field('service', value)} />;
  if (type === 'diagnostics.collect') return <div className="nx-form-grid"><TextField label="诊断范围" value={String(fields.scope ?? 'standard')} onChange={(value) => field('scope', value)} /><label className="nx-confirm-check"><input type="checkbox" checked={Boolean(fields.include_logs)} onChange={(event) => field('include_logs', event.target.checked)} />包含脱敏日志</label></div>;
  return <TextField label="重载原因" required value={String(fields.reason ?? '')} onChange={(value) => field('reason', value)} />;
}

function CommandHistoryDialog({ snapshot, onClose, onCreate }: { snapshot: DeviceSnapshot; onClose: () => void; onCreate: () => void }) {
  const [commands, setCommands] = useState<DeviceCommand[]>([]);
  const [selected, setSelected] = useState<DeviceCommand>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [refreshToken, setRefreshToken] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setError('');
    listDeviceCommands(snapshot.device.id, controller.signal)
      .then(({ items }) => {
        const nextCommands = [...items].sort((a, b) => Date.parse(b.created_at) - Date.parse(a.created_at));
        setCommands(nextCommands);
        setSelected((current) => current && nextCommands.some((item) => item.id === current.id) ? current : nextCommands[0]);
      })
      .catch((cause) => {
        if (!controller.signal.aborted) setError(messageOf(cause));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [snapshot.device.id, refreshToken]);
  return <Dialog title={`${snapshot.device.name} · 命令历史`} description="查看设备操作的状态、结果和错误。" onClose={onClose} wide>
    <div className="nx-history-toolbar"><button type="button" className="nx-button" onClick={onCreate}><Play size={15} />下发命令</button><button type="button" className="nx-button is-secondary" onClick={() => setRefreshToken((value) => value + 1)}><RefreshCw size={15} />刷新</button></div>
    {error && <div className="nx-alert is-error">{error}</div>}
    {loading ? <p className="nx-muted">正在读取命令…</p> : commands.length === 0 ? <DeviceEmpty icon={<TerminalSquare />} title="暂无命令" text="从此处创建第一条受控命令。" /> : <div className="nx-command-layout"><div className="nx-command-list">{commands.map((command) => <button type="button" key={command.id} className={selected?.id === command.id ? 'is-active' : ''} onClick={() => setSelected(command)}><span><strong>{COMMAND_LABELS[command.type]}</strong><small>{formatTime(command.created_at)}</small></span><CommandStatusBadge status={command.status} /></button>)}</div>{selected && <CommandDetails commandId={selected.id} seed={selected} />}</div>}
  </Dialog>;
}

function CommandDetails({ commandId, seed }: { commandId: string; seed: DeviceCommand }) {
  const { command, error, loading, refresh } = useCommandPolling(commandId, seed);
  if (!command) return <div className="nx-command-detail"><p className="nx-muted">选择命令查看详情。</p></div>;
  const steps = ['queued', 'leased', 'running', command.status === 'failed' ? 'failed' : 'succeeded'];
  const current = steps.indexOf(command.status);
  return <div className="nx-command-detail">
    <div className="nx-command-title"><div><h3>{COMMAND_LABELS[command.type]}</h3><code>{command.id}</code></div><CommandStatusBadge status={command.status} /></div>
    {loading && <p className="nx-muted">正在刷新命令状态…</p>}{error && <div className="nx-alert is-error">{error}<button type="button" onClick={refresh}>重试</button></div>}
    <div className="nx-command-timeline">{steps.map((step, index) => <div key={step} className={index <= current || TERMINAL_COMMAND_STATUSES.has(command.status) && step === command.status ? 'is-complete' : ''}><span>{index + 1}</span><small>{step}</small></div>)}</div>
    <div className="nx-detail-grid"><Detail label="风险" value={command.risk} /><Detail label="尝试次数" value={`${command.attempts ?? command.attempt ?? 0} / ${command.max_attempts}`} /><Detail label="创建时间" value={formatTime(command.created_at)} /><Detail label="过期时间" value={formatTime(command.expires_at)} /></div>
    {command.progress && <div className="nx-progress-box"><div><span>{command.progress.message || '执行中'}</span><strong>{command.progress.percent}%</strong></div><progress max={100} value={command.progress.percent} /></div>}
    <JsonPanel title="Payload" value={command.payload} />
    {command.result?.output !== undefined && <JsonPanel title="结构化输出" value={command.result.output} />}
    {command.result?.error && <div className="nx-error-result"><strong>{command.result.error_code || 'COMMAND_FAILED'}</strong><p>{command.result.error}</p></div>}
  </div>;
}

function JsonPanel({ title, value }: { title: string; value: unknown }) { return <details className="nx-json-panel"><summary>{title}</summary><pre>{safeJSON(value)}</pre></details>; }
function TextField({ label, value, onChange, required = false }: { label: string; value: string; onChange: (value: string) => void; required?: boolean }) { return <label><span>{label}</span><input required={required} value={value} onChange={(event) => onChange(event.target.value)} /></label>; }
function Detail({ label, value }: { label: string; value: string }) { return <div className="nx-detail"><span>{label}</span><strong>{value || '暂无'}</strong></div>; }
function Metric({ label, value }: { label: string; value?: number }) {
  const reported = typeof value === 'number';
  return <div><span>{label}</span><strong>{reported ? `${value.toFixed(1)}%` : '未上报'}</strong><progress max={100} value={reported ? value : 0} /></div>;
}
function DeviceEmpty({ icon, title, text }: { icon: React.ReactNode; title: string; text: string }) { return <div className="empty-state nx-device-empty"><span>{icon}</span><h3>{title}</h3><p>{text}</p></div>; }
type ActionDialogProps = { snapshot: DeviceSnapshot; onClose: () => void; onComplete: () => void };
function messageOf(reason: unknown) { return reason instanceof Error ? reason.message : '操作失败'; }
function fingerprint(value: string) { if (!value) return '未提供'; const compact = value.replace(/\s+/g, ''); return `${compact.slice(0, 12)}…${compact.slice(-12)}`; }
function safeJSON(value: unknown) { try { return JSON.stringify(value, null, 2).slice(0, 100000); } catch { return '无法序列化结果'; } }
