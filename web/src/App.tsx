import { Children, useEffect, useState, type ReactNode } from 'react';
import {
  Activity, ArrowDownToLine, ArrowUpFromLine, ChevronRight,
  CircleAlert, Database, FileArchive, HardDrive, Home, Menu, RefreshCw,
  Server, Settings, ShieldCheck, Sparkles, X,
} from 'lucide-react';
import RecallWorkspace from './RecallWorkspace';
import { AccountSecurity, type WebSession } from './Auth';
import { ApiError, api, setCSRFToken } from './api/client';
import DevicesManagementPage from './components/devices/DevicesPage';
import './nexus.css';

type Section = 'home' | 'devices' | 'recall' | 'files' | 'settings';
type Tone = 'ok' | 'warn' | 'danger' | 'muted';

type NexusDeviceSummary = {
  id: string;
  name?: string;
  platform?: string;
  arch?: string;
  agentdock_version?: string;
  status?: string;
};

type BackupHistory = {
  state: string;
  message?: string;
  started_at?: string;
  completed_at?: string;
  archive?: string;
  archive_size?: number;
  sha256?: string;
  remote_path?: string;
};

type BackupStatus = {
  id: string;
  title: string;
  description?: string;
  provider: string;
  device: string;
  enabled: boolean;
  schedule: string;
  state: string;
  last_started_at?: string;
  last_completed_at?: string;
  next_run_at?: string;
  message?: string;
  archive?: string;
  archive_size?: number;
  sha256?: string;
  remote_path?: string;
  history?: BackupHistory[];
};

type Artifact = {
  id: string;
  source_kind: string;
  source_id?: string;
  filename: string;
  content_type: string;
  status: string;
  cipher_size: number;
  plain_size: number;
  plain_sha256?: string;
  expires_at: string;
  created_at: string;
  updated_at: string;
};

type Delivery = {
  id: string;
  artifact_id: string;
  target_device_id: string;
  status: string;
  local_path?: string;
  error_code?: string;
  error_message?: string;
  created_at: string;
  updated_at: string;
  completed_at?: string;
};

type ArtifactDetail = { artifact: Artifact; deliveries: Delivery[] };

type FetchJob = {
  id: string;
  requester_device_id: string;
  source_device_id: string;
  source_path: string;
  archive_requested: boolean;
  status: string;
  filename?: string;
  plain_size: number;
  plain_sha256?: string;
  error_code?: string;
  error_message?: string;
  expires_at: string;
  created_at: string;
  updated_at: string;
  mounted_at?: string;
};

type SystemStatus = {
  ok: boolean;
  service: string;
  database: string;
  schema_version: number;
  recall_root?: string;
  artifact_root: string;
};

type Resource<T> = { data: T; live: boolean; loading: boolean; error?: string };

const NAV: Array<{ id: Section; label: string; icon: typeof Home }> = [
  { id: 'home', label: '总览', icon: Home },
  { id: 'devices', label: '设备', icon: Server },
  { id: 'recall', label: '召回', icon: Database },
  { id: 'files', label: '文件', icon: FileArchive },
  { id: 'settings', label: '设置', icon: Settings },
];

function sectionFromHash(): Section {
  const value = window.location.hash.replace(/^#\/?/, '').split('/')[0] as Section;
  if (NAV.some((item) => item.id === value)) return value;
  const params = new URLSearchParams(window.location.search);
  if (params.has('tab') || params.has('path') || params.has('prefix') || params.has('q')) return 'recall';
  return 'home';
}

function unpackAPI<T>(body: unknown): T {
  const value = body as { data?: unknown; items?: unknown };
  return (value?.data ?? value?.items ?? body) as T;
}

function messageOf(error: unknown): string {
  if (error instanceof ApiError && error.status === 401) return '登录会话已失效，请重新登录。';
  if (error instanceof ApiError && error.status === 403) return '当前账号没有访问权限。';
  return error instanceof Error ? error.message : '读取 Nexus 数据失败';
}

function useResource<T>(path: string, fallback: T, refreshToken: number): Resource<T> {
  const [state, setState] = useState<Resource<T>>({ data: fallback, live: false, loading: true });
  useEffect(() => {
    let cancelled = false;
    setState((current) => ({ ...current, loading: true }));
    api<unknown>(path).then((body) => {
      if (!cancelled) setState({ data: unpackAPI<T>(body), live: true, loading: false });
    }).catch((error) => {
      if (!cancelled) setState({ data: fallback, live: false, loading: false, error: messageOf(error) });
    });
    return () => { cancelled = true; };
  }, [path, refreshToken]);
  return state;
}

function formatTime(value?: string): string {
  if (!value) return '暂无时间';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'short', timeStyle: 'short' }).format(date);
}

function formatBytes(value?: number): string {
  if (value === undefined || value < 0) return '暂无';
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
  let size = value;
  let unit = 0;
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024;
    unit += 1;
  }
  return `${size.toFixed(unit === 0 ? 0 : 2)} ${units[unit]}`;
}

function toneForStatus(status?: string): Tone {
  if (!status) return 'muted';
  if (['online', 'healthy', 'success', 'succeeded', 'completed', 'ready', 'mounted', 'listed'].includes(status)) return 'ok';
  if (['failed', 'offline', 'blocked', 'revoked', 'expired'].includes(status)) return 'danger';
  if (['degraded', 'pending', 'running', 'queued', 'uploading', 'downloading', 'listing'].includes(status)) return 'warn';
  return 'muted';
}

export default function App() {
  const [section, setSection] = useState<Section>(sectionFromHash);
  const [menuOpen, setMenuOpen] = useState(false);
  const [refreshToken, setRefreshToken] = useState(0);
  const [sessionExpired, setSessionExpired] = useState(false);
  const [session, setSession] = useState<WebSession | null>(null);

  useEffect(() => {
    let cancelled = false;
    api<{ ok: boolean; session: WebSession }>('/v1/auth/session').then((result) => {
      if (cancelled) return;
      setSession(result.session);
      if (result.session.csrf_token) setCSRFToken(result.session.csrf_token);
      if (result.session.must_change_password) {
        const returnTo = `${window.location.pathname}${window.location.search}${window.location.hash}`;
        window.location.replace(`/change-password?return_to=${encodeURIComponent(returnTo)}`);
      }
    }).catch((error) => {
      if (!cancelled && error instanceof ApiError && error.status === 401) setSessionExpired(true);
    });
    const expired = () => setSessionExpired(true);
    window.addEventListener('nexus:session-expired', expired);
    return () => {
      cancelled = true;
      window.removeEventListener('nexus:session-expired', expired);
    };
  }, []);

  useEffect(() => {
    const onHash = () => setSection(sectionFromHash());
    window.addEventListener('hashchange', onHash);
    return () => window.removeEventListener('hashchange', onHash);
  }, []);

  function navigate(next: Section) {
    window.location.hash = next;
    setSection(next);
    setMenuOpen(false);
  }

  if (section === 'recall') {
    return (
      <div className="nexus-recall-mode">
        <button className="nexus-recall-return" onClick={() => navigate('home')}>
          <ChevronRight size={15} /> 返回 Nexus
        </button>
        <RecallWorkspace />
        {sessionExpired && <SessionExpiredDialog />}
      </div>
    );
  }

  const active = NAV.find((item) => item.id === section) ?? NAV[0];
  return (
    <div className="nexus-app">
      <aside className={`nexus-sidebar ${menuOpen ? 'is-open' : ''}`}>
        <div className="nexus-brand">
          <span className="nexus-brand-mark"><Sparkles size={19} /></span>
          <span><strong>AgentDock</strong><small>Nexus</small></span>
        </div>
        <nav aria-label="主导航">
          {NAV.map((item) => {
            const Icon = item.icon;
            return <button key={item.id} className={section === item.id ? 'active' : ''} onClick={() => navigate(item.id)}><Icon size={18} /><span>{item.label}</span></button>;
          })}
        </nav>
        <div className="nexus-sidebar-foot"><ShieldCheck size={16} /><span>个人控制台</span></div>
      </aside>
      {menuOpen && <button className="nexus-scrim" aria-label="关闭菜单" onClick={() => setMenuOpen(false)} />}
      <main className="nexus-main">
        <header className="nexus-topbar">
          <button className="nexus-mobile-menu" aria-label="切换菜单" onClick={() => setMenuOpen((value) => !value)}>{menuOpen ? <X /> : <Menu />}</button>
          <div><span className="nexus-eyebrow">AgentDock Nexus</span><h1>{active.label}</h1></div>
          <div className="nexus-top-actions">
            <button className="icon-button" title="刷新" onClick={() => setRefreshToken((value) => value + 1)}><RefreshCw size={17} /></button>
            <span className="nexus-session-user" title={session?.username || '管理员会话'}>{session?.display_name || session?.username || 'Admin'}</span>
          </div>
        </header>
        <div className="nexus-content">
          {section === 'home' && <HomePage refreshToken={refreshToken} navigate={navigate} />}
          {section === 'devices' && <DevicesManagementPage refreshToken={refreshToken} />}
          {section === 'files' && <FilesPage refreshToken={refreshToken} />}
          {section === 'settings' && <SettingsPage refreshToken={refreshToken} />}
        </div>
      </main>
      {sessionExpired && <SessionExpiredDialog />}
    </div>
  );
}

function SessionExpiredDialog() {
  function signInAgain() {
    const returnTo = `${window.location.pathname}${window.location.search}${window.location.hash}`;
    window.location.assign(`/login?return_to=${encodeURIComponent(returnTo)}`);
  }
  return <div className="session-expired-overlay" role="presentation"><section className="session-expired-dialog" role="dialog" aria-modal="true" aria-labelledby="session-expired-title"><span><CircleAlert size={22} /></span><h2 id="session-expired-title">会话已过期</h2><p>当前页面保持不变，失败的写操作不会自动重试。</p><button onClick={signInAgain}>重新登录</button></section></div>;
}

function HomePage({ refreshToken, navigate }: { refreshToken: number; navigate: (section: Section) => void }) {
  const devicesResource = useResource<NexusDeviceSummary[]>('/v1/devices', [], refreshToken);
  const backupResource = useResource<BackupStatus | undefined>('/v1/backup/status', undefined, refreshToken);
  const artifactsResource = useResource<ArtifactDetail[]>('/v1/artifacts?limit=8', [], refreshToken);
  const fetchesResource = useResource<FetchJob[]>('/v1/artifact-fetches?limit=8', [], refreshToken);
  const devices = devicesResource.data ?? [];
  const artifacts = artifactsResource.data ?? [];
  const fetches = fetchesResource.data ?? [];
  const backup = backupResource.data;
  const unhealthyDevices = devices.filter((device) => ['degraded', 'offline', 'pending', 'revoked'].includes(device.status || ''));
  const failedTransfers = artifacts.filter((item) => item.deliveries.some((delivery) => delivery.status === 'failed')).length
    + fetches.filter((item) => item.status === 'failed').length;
  const recentTransferCount = artifacts.length + fetches.length;
  const errors = [devicesResource.error, backupResource.error, artifactsResource.error, fetchesResource.error].filter(Boolean) as string[];

  return <>
    <section className="nexus-hero nexus-workbench-hero">
      <div><span className="nexus-kicker">个人控制台</span><h2>设备、记忆、文件和备份集中管理</h2><p>只展示当前真实运行链路，不再保留未接入生产的 Task、Run 和 Evolution 概念。</p></div>
      <div className="hero-health-stack"><StatusBadge tone={errors.length ? 'danger' : 'ok'}>{errors.length ? '部分数据不可用' : '控制面正常'}</StatusBadge><span>{devices.length} 台设备</span><span>{recentTransferCount} 条近期传输</span></div>
    </section>
    {errors.length > 0 && <InlineAlert tone="danger" title="部分数据读取失败" message={errors.join('；')} />}

    <section className="metric-grid compact-metrics">
      <MetricButton label="在线设备" value={devices.filter((item) => item.status === 'online').length} tone="ok" onClick={() => navigate('devices')} />
      <MetricButton label="需要处理" value={unhealthyDevices.length + (backup?.state === 'failed' ? 1 : 0)} tone={unhealthyDevices.length || backup?.state === 'failed' ? 'danger' : 'muted'} onClick={() => navigate('devices')} />
      <MetricButton label="近期文件" value={recentTransferCount} tone="ok" onClick={() => navigate('files')} />
      <MetricButton label="传输失败" value={failedTransfers} tone={failedTransfers ? 'danger' : 'muted'} onClick={() => navigate('files')} />
    </section>

    <section className="dashboard-grid-nexus">
      <Panel title="设备状态" subtitle={`${devices.length} 台已注册设备`}>
        <SummaryStat label="在线" value={String(devices.filter((device) => device.status === 'online').length)} tone="ok" />
        <SummaryStat label="需关注" value={String(unhealthyDevices.length)} tone={unhealthyDevices.length ? 'danger' : 'muted'} />
        <SummaryList empty="暂无设备。">{devices.slice(0, 5).map((device) => <ObjectRow key={device.id} title={device.name || device.id} detail={`${device.status || 'unknown'} / ${device.platform || 'unknown'}`} tone={toneForStatus(device.status)} />)}</SummaryList>
      </Panel>
      <BackupPanel backup={backup} />
      <Panel title="需要处理" subtitle="只聚合真实设备、备份和文件异常">
        {unhealthyDevices.length === 0 && backup?.state !== 'failed' && failedTransfers === 0 ? <EmptyMini text="当前没有需要立刻处理的对象。" /> : <>
          {unhealthyDevices.slice(0, 4).map((device) => <button type="button" className="attention-row" key={device.id} onClick={() => navigate('devices')}><StatusBadge tone={toneForStatus(device.status)}>{device.status}</StatusBadge><span><strong>{device.name || device.id}</strong><small>{device.platform || 'unknown'} / {device.arch || 'unknown'}</small></span><ChevronRight size={16} /></button>)}
          {backup?.state === 'failed' && <button type="button" className="attention-row" onClick={() => navigate('settings')}><StatusBadge tone="danger">failed</StatusBadge><span><strong>备份失败</strong><small>{backup.message || formatTime(backup.last_completed_at || backup.last_started_at)}</small></span><ChevronRight size={16} /></button>}
          {failedTransfers > 0 && <button type="button" className="attention-row" onClick={() => navigate('files')}><StatusBadge tone="danger">failed</StatusBadge><span><strong>{failedTransfers} 条文件传输失败</strong><small>打开文件页面查看错误和目标设备</small></span><ChevronRight size={16} /></button>}
        </>}
      </Panel>
    </section>
  </>;
}

function FilesPage({ refreshToken }: { refreshToken: number }) {
  const artifactsResource = useResource<ArtifactDetail[]>('/v1/artifacts?limit=100', [], refreshToken);
  const fetchesResource = useResource<FetchJob[]>('/v1/artifact-fetches?limit=100', [], refreshToken);
  const artifacts = artifactsResource.data ?? [];
  const fetches = fetchesResource.data ?? [];
  const error = artifactsResource.error || fetchesResource.error;
  return <section>
    <div className="section-heading"><div><h2>文件传输</h2><p>查看真实 Artifact 发送、Delivery 落盘和反向 Fetch 状态。Nexus 只保存密文。</p></div><StatusBadge tone={error ? 'danger' : 'ok'}>{error ? '读取失败' : '实时 API'}</StatusBadge></div>
    {error && <InlineAlert tone="danger" title="文件记录读取失败" message={error} />}
    <div className="file-summary-grid">
      <SummaryCard icon={<ArrowUpFromLine size={18} />} label="发送记录" value={artifacts.length} />
      <SummaryCard icon={<ArrowDownToLine size={18} />} label="Fetch 记录" value={fetches.length} />
      <SummaryCard icon={<HardDrive size={18} />} label="已完成 Delivery" value={artifacts.flatMap((item) => item.deliveries).filter((item) => item.status === 'completed').length} />
    </div>
    <section className="file-section">
      <h3>发送与 Delivery</h3>
      {artifactsResource.loading ? <EmptyState text="正在读取 Artifact 记录…" /> : artifacts.length === 0 ? <EmptyState text="暂无 Artifact 发送记录。" /> : <div className="file-record-list">{artifacts.map((detail) => <ArtifactCard key={detail.artifact.id} detail={detail} />)}</div>}
    </section>
    <section className="file-section">
      <h3>反向 Fetch</h3>
      {fetchesResource.loading ? <EmptyState text="正在读取 Fetch 记录…" /> : fetches.length === 0 ? <EmptyState text="暂无反向 Fetch 记录。" /> : <div className="file-record-list">{fetches.map((fetch) => <FetchCard key={fetch.id} fetch={fetch} />)}</div>}
    </section>
  </section>;
}

function ArtifactCard({ detail }: { detail: ArtifactDetail }) {
  const { artifact, deliveries } = detail;
  return <article className="file-record-card"><header><span className="file-record-icon"><ArrowUpFromLine size={17} /></span><div><h4>{artifact.filename}</h4><code>{artifact.id}</code></div><StatusBadge tone={toneForStatus(artifact.status)}>{artifact.status}</StatusBadge></header><dl><div><dt>来源</dt><dd>{artifact.source_kind}{artifact.source_id ? ` / ${artifact.source_id}` : ''}</dd></div><div><dt>大小</dt><dd>{formatBytes(artifact.plain_size)}</dd></div><div><dt>创建</dt><dd>{formatTime(artifact.created_at)}</dd></div><div><dt>过期</dt><dd>{formatTime(artifact.expires_at)}</dd></div></dl>{artifact.plain_sha256 && <div className="file-digest"><span>SHA256</span><code>{artifact.plain_sha256}</code></div>}<div className="delivery-list">{deliveries.map((delivery) => <div key={delivery.id}><StatusBadge tone={toneForStatus(delivery.status)}>{delivery.status}</StatusBadge><span><strong>{delivery.target_device_id}</strong><small>{delivery.local_path || delivery.error_message || delivery.id}</small></span><time>{formatTime(delivery.completed_at || delivery.updated_at)}</time></div>)}</div></article>;
}

function FetchCard({ fetch }: { fetch: FetchJob }) {
  return <article className="file-record-card"><header><span className="file-record-icon"><ArrowDownToLine size={17} /></span><div><h4>{fetch.filename || fetch.source_path}</h4><code>{fetch.id}</code></div><StatusBadge tone={toneForStatus(fetch.status)}>{fetch.status}</StatusBadge></header><dl><div><dt>源设备</dt><dd>{fetch.source_device_id}</dd></div><div><dt>请求设备</dt><dd>{fetch.requester_device_id}</dd></div><div><dt>大小</dt><dd>{formatBytes(fetch.plain_size)}</dd></div><div><dt>创建</dt><dd>{formatTime(fetch.created_at)}</dd></div></dl><div className="file-digest"><span>源路径</span><code>{fetch.source_path}</code></div>{fetch.plain_sha256 && <div className="file-digest"><span>SHA256</span><code>{fetch.plain_sha256}</code></div>}{fetch.error_message && <div className="nx-alert is-error">{fetch.error_code || 'FETCH_FAILED'}：{fetch.error_message}</div>}</article>;
}

function SettingsPage({ refreshToken }: { refreshToken: number }) {
  const system = useResource<SystemStatus>('/v1/system/status', { ok: false, service: 'recalldock', database: 'unknown', schema_version: 0, recall_root: '', artifact_root: '' }, refreshToken);
  const backup = useResource<BackupStatus | undefined>('/v1/backup/status', undefined, refreshToken);
  return <>
    <AccountSecurity />
    <section className="settings-grid compact-settings">
      <Panel title="系统状态" subtitle="Nexus 运行与 SQLite 健康">
        <SettingValue label="服务" value={system.data.service || 'recalldock'} tone={system.data.ok ? 'ok' : 'danger'} />
        <SettingValue label="数据库" value={system.data.database || 'unknown'} tone={system.data.database === 'ok' ? 'ok' : 'danger'} />
        <SettingValue label="Schema" value={String(system.data.schema_version || 0)} />
        <SettingValue label="召回仓库" value={system.data.recall_root || '暂无'} mono />
        <SettingValue label="密文目录" value={system.data.artifact_root || '暂无'} mono />
      </Panel>
      <BackupPanel backup={backup.data} />
    </section>
  </>;
}

function BackupPanel({ backup }: { backup?: BackupStatus }) {
  return <Panel title="备份状态" subtitle={backup ? `${backup.device} · ${backup.schedule}` : '等待备份状态'}>
    {backup ? <>
      <SettingValue label="状态" value={backup.state || 'unknown'} tone={toneForStatus(backup.state)} />
      <SettingValue label="最近完成" value={formatTime(backup.last_completed_at)} />
      <SettingValue label="下次运行" value={formatTime(backup.next_run_at)} />
      <SettingValue label="归档大小" value={formatBytes(backup.archive_size)} />
      <SettingValue label="远端路径" value={backup.remote_path || '暂无'} mono />
      <SettingValue label="SHA256" value={backup.sha256 || '暂无'} mono />
    </> : <EmptyMini text="暂无备份状态。" />}
  </Panel>;
}

function MetricButton({ label, value, tone, onClick }: { label: string; value: number; tone: Tone; onClick: () => void }) { return <button className="metric-card" onClick={onClick}><span className={`metric-icon tone-${tone}`}><Activity size={20} /></span><span className="metric-value">{value}</span><span className="metric-label">{label}</span><ChevronRight size={17} className="metric-arrow" /></button>; }
function SummaryCard({ icon, label, value }: { icon: ReactNode; label: string; value: number }) { return <article className="file-summary-card"><span>{icon}</span><div><strong>{value}</strong><small>{label}</small></div></article>; }
function Panel({ title, subtitle, children }: { title: string; subtitle: string; children: ReactNode }) { return <article className="nexus-panel"><header><div><h3>{title}</h3><p>{subtitle}</p></div></header><div className="panel-body">{children}</div></article>; }
function StatusBadge({ tone, children }: { tone: Tone; children: ReactNode }) { return <span className={`status-badge tone-${tone}`}><span />{children}</span>; }
function InlineAlert({ tone, title, message }: { tone: Tone; title: string; message: string }) { return <div className={`nexus-inline-alert tone-${tone}`}><strong>{title}</strong><span>{message}</span></div>; }
function EmptyState({ text }: { text: string }) { return <div className="empty-state"><span><Activity size={24} /></span><h3>等待数据</h3><p>{text}</p></div>; }
function SummaryStat({ label, value, tone }: { label: string; value: string; tone: Tone }) { return <div className="summary-stat"><span className={`metric-icon tone-${tone}`}><Activity size={15} /></span><div><strong>{value}</strong><small>{label}</small></div></div>; }
function SummaryList({ empty, children }: { empty: string; children: ReactNode }) { return <div className="summary-list">{Children.count(children) ? children : <EmptyMini text={empty} />}</div>; }
function ObjectRow({ title, detail, tone }: { title: string; detail: string; tone: Tone }) { return <div className="object-row"><StatusBadge tone={tone}>{tone}</StatusBadge><span><strong>{title}</strong><small>{detail}</small></span></div>; }
function EmptyMini({ text }: { text: string }) { return <p className="empty-mini">{text}</p>; }
function SettingValue({ label, value, tone = 'muted', mono = false }: { label: string; value: string; tone?: Tone; mono?: boolean }) { return <div className="setting-value"><span>{label}</span><div>{tone !== 'muted' && <StatusBadge tone={tone}>{value}</StatusBadge>}{tone === 'muted' && <strong className={mono ? 'nx-mono' : ''}>{value}</strong>}</div></div>; }
