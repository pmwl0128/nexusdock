import { useEffect, useMemo, useState, type FormEvent, type ReactNode } from 'react';
import {
  Activity,
  BellRing,
  Bot,
  Boxes,
  CalendarClock,
  ChevronRight,
  CircleAlert,
  CircleCheck,
  Database,
  Home,
  Menu,
  PlayCircle,
  RefreshCw,
  Search,
  Server,
  Settings,
  ShieldCheck,
  Sparkles,
  X,
} from 'lucide-react';
import MemoryWorkspace from './MemoryWorkspace';
import { ApiError, api } from './api/client';
import DevicesManagementPage from './components/devices/DevicesPage';
import EnvManagerPage from './components/env/EnvManagerPage';
import './nexus.css';

type Section = 'home' | 'inbox' | 'devices' | 'memory' | 'skills' | 'runs' | 'schedules' | 'settings';
type Tone = 'ok' | 'warn' | 'danger' | 'muted';

type Overview = {
  agent_tasks: number;
  user_tasks: number;
  device_alerts: number;
  skill_candidates: number;
  memory_conflicts: number;
  recent_failures: number;
};

type Task = {
  id: string;
  title: string;
  type: string;
  status: string;
  source?: string;
  updated_at?: string;
};

type Skill = {
  id: string;
  name: string;
  version?: string;
  trust?: string;
  maturity?: string;
  installations?: number;
};

type Run = {
  id: string;
  title?: string;
  status: string;
  device?: string;
  skill?: string;
  started_at?: string;
};

type ScheduleHistory = {
  state: string;
  message?: string;
  started_at?: string;
  completed_at?: string;
  archive?: string;
  archive_size?: number;
  sha256?: string;
  remote_path?: string;
};

type Schedule = {
  id: string;
  title: string;
  description?: string;
  provider: string;
  device: string;
  enabled: boolean;
  schedule: string;
  schedule_type: string;
  state: string;
  last_started_at?: string;
  last_completed_at?: string;
  next_run_at?: string;
  message?: string;
  archive?: string;
  archive_size?: number;
  sha256?: string;
  remote_path?: string;
  history?: ScheduleHistory[];
};

type NexusDeviceSummary = {
  id: string;
  name?: string;
  platform?: string;
  arch?: string;
  agentdock_version?: string;
  status?: string;
};

type Resource<T> = { data: T; live: boolean; loading: boolean; error?: string };
type SearchResult = { id: string; label: string; description: string; target: Section; source: 'section' | 'device' | 'schedule' };

const NAV: Array<{ id: Section; label: string; icon: typeof Home }> = [
  { id: 'home', label: 'Home', icon: Home },
  { id: 'inbox', label: 'Inbox', icon: BellRing },
  { id: 'devices', label: 'Devices', icon: Server },
  { id: 'memory', label: 'Memory', icon: Database },
  { id: 'skills', label: 'Skills', icon: Boxes },
  { id: 'runs', label: 'Runs', icon: PlayCircle },
  { id: 'schedules', label: '计划任务', icon: CalendarClock },
  { id: 'settings', label: 'Settings', icon: Settings },
];

const SEARCH_HINTS: Record<Section, string[]> = {
  home: ['overview', 'dashboard', '总览', '首页', '态势'],
  inbox: ['task', 'todo', 'agent', '待办', '任务', 'review'],
  devices: ['device', 'node', 'control plane', '设备', '节点', '命令', '注册'],
  memory: ['memory', 'memories', 'note', '记忆', '文件', '同步'],
  skills: ['skill', 'catalog', '插件', '能力', '安装'],
  runs: ['run', 'evidence', 'history', '运行', '证据', '失败'],
  schedules: ['schedule', 'backup', 'launchd', '计划', '定时', '备份', '天翼云盘'],
  settings: ['setting', 'auth', 'token', '设置', '认证', '权限'],
};

const EMPTY_OVERVIEW: Overview = {
  agent_tasks: 0,
  user_tasks: 0,
  device_alerts: 0,
  skill_candidates: 0,
  memory_conflicts: 0,
  recent_failures: 0,
};

function sectionFromHash(): Section {
  const value = window.location.hash.replace(/^#\/?/, '').split('/')[0] as Section;
  if (NAV.some((item) => item.id === value)) return value;
  const params = new URLSearchParams(window.location.search);
  if (params.has('tab') || params.has('path') || params.has('prefix') || params.has('q')) return 'memory';
  return 'home';
}

function unpackAPI<T>(body: unknown): T {
  const value = body as { data?: unknown; items?: unknown };
  return (value?.data ?? value?.items ?? body) as T;
}

function isCompatibilityMiss(error: unknown): boolean {
  return error instanceof ApiError && (error.status === 404 || error.status === 405 || error.code === 'INVALID_JSON');
}

function messageOf(error: unknown): string {
  if (error instanceof ApiError && error.status === 401) return 'API 鉴权未通过，请刷新页面并确认浏览器已通过 Basic Auth。';
  if (error instanceof ApiError && error.status === 403) return '当前账号没有访问这个 Nexus API 的权限。';
  return error instanceof Error ? error.message : '读取 Nexus 数据失败';
}

function useResource<T>(paths: string[], fallback: T, refreshToken: number): Resource<T> {
  const [state, setState] = useState<Resource<T>>({ data: fallback, live: false, loading: true });
  const key = paths.join('|');

  useEffect(() => {
    let cancelled = false;
    setState((current) => ({ ...current, loading: true }));
    void (async () => {
      let lastError: unknown;
      for (const path of paths) {
        try {
          const body = await api<unknown>(path);
          if (!cancelled) setState({ data: unpackAPI<T>(body), live: true, loading: false });
          return;
        } catch (error) {
          lastError = error;
          if (isCompatibilityMiss(error)) continue;
          break;
        }
      }
      if (!cancelled) {
        setState({
          data: fallback,
          live: false,
          loading: false,
          error: lastError && !isCompatibilityMiss(lastError) ? messageOf(lastError) : undefined,
        });
      }
    })();
    return () => { cancelled = true; };
  }, [key, refreshToken]);

  return state;
}

function useOverview(refreshToken: number): Resource<Overview> {
  const [state, setState] = useState<Resource<Overview>>({ data: EMPTY_OVERVIEW, live: false, loading: true });

  useEffect(() => {
    let cancelled = false;
    setState((current) => ({ ...current, loading: true }));
    void (async () => {
      try {
        const body = await api<unknown>('/api/v1/nexus/overview');
        if (!cancelled) setState({ data: { ...EMPTY_OVERVIEW, ...unpackAPI<Overview>(body) }, live: true, loading: false });
        return;
      } catch (error) {
        if (!isCompatibilityMiss(error)) {
          if (!cancelled) setState({ data: EMPTY_OVERVIEW, live: false, loading: false, error: messageOf(error) });
          return;
        }
      }

      const [devicesResult, schedulesResult] = await Promise.allSettled([
        api<{ items: NexusDeviceSummary[] }>('/v1/devices'),
        api<{ items: Schedule[] }>('/v1/schedules'),
      ]);
      if (cancelled) return;

      const firstFailure = [devicesResult, schedulesResult].find((result) => result.status === 'rejected') as PromiseRejectedResult | undefined;
      const hardFailure = firstFailure && !isCompatibilityMiss(firstFailure.reason);
      if (hardFailure) {
        setState({ data: EMPTY_OVERVIEW, live: false, loading: false, error: messageOf(firstFailure.reason) });
        return;
      }

      const devices = devicesResult.status === 'fulfilled' ? devicesResult.value.items ?? [] : [];
      const schedules = schedulesResult.status === 'fulfilled' ? schedulesResult.value.items ?? [] : [];
      const overview: Overview = {
        ...EMPTY_OVERVIEW,
        device_alerts: devices.filter((device) => ['degraded', 'offline', 'pending', 'revoked'].includes(device.status || '')).length,
        recent_failures: schedules.filter((schedule) => toneForStatus(schedule.state) === 'danger').length,
      };
      setState({ data: overview, live: devicesResult.status === 'fulfilled' || schedulesResult.status === 'fulfilled', loading: false });
    })();
    return () => { cancelled = true; };
  }, [refreshToken]);

  return state;
}

function formatTime(value?: string): string {
  if (!value) return '暂无时间';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'short', timeStyle: 'short' }).format(date);
}

function toneForStatus(status?: string): Tone {
  if (!status) return 'muted';
  if (['online', 'healthy', 'success', 'succeeded', 'completed', 'stable', 'active', 'ready'].includes(status)) return 'ok';
  if (['failed', 'offline', 'blocked', 'revoked', 'conflicted'].includes(status)) return 'danger';
  if (['degraded', 'pending', 'running', 'queued', 'candidate', 'canary'].includes(status)) return 'warn';
  return 'muted';
}

function matchesQuery(query: string, values: Array<string | undefined>): boolean {
  return values.filter(Boolean).join(' ').toLowerCase().includes(query);
}

function searchSections(query: string): SearchResult[] {
  const normalized = query.trim().toLowerCase();
  if (!normalized) return [];
  return NAV
    .map((item) => ({
      id: `section:${item.id}`,
      label: item.label,
      description: SEARCH_HINTS[item.id].slice(-3).join(' / '),
      target: item.id,
      source: 'section' as const,
      haystack: [item.id, item.label, ...SEARCH_HINTS[item.id]].join(' ').toLowerCase(),
    }))
    .filter((item) => item.haystack.includes(normalized))
    .slice(0, 6);
}

function useLiveSearch(query: string): Resource<SearchResult[]> {
  const [state, setState] = useState<Resource<SearchResult[]>>({ data: [], live: false, loading: false });
  const normalized = query.trim().toLowerCase();

  useEffect(() => {
    if (!normalized) {
      setState({ data: [], live: false, loading: false });
      return;
    }
    let cancelled = false;
    setState((current) => ({ ...current, loading: true }));
    void (async () => {
      const [devicesResult, schedulesResult] = await Promise.allSettled([
        api<{ items: NexusDeviceSummary[] }>('/v1/devices'),
        api<{ items: Schedule[] }>('/v1/schedules'),
      ]);
      if (cancelled) return;

      const firstFailure = [devicesResult, schedulesResult].find((result) => result.status === 'rejected') as PromiseRejectedResult | undefined;
      const hardFailure = firstFailure && !isCompatibilityMiss(firstFailure.reason);
      const devices = devicesResult.status === 'fulfilled' ? devicesResult.value.items ?? [] : [];
      const schedules = schedulesResult.status === 'fulfilled' ? schedulesResult.value.items ?? [] : [];
      const results: SearchResult[] = [
        ...devices
          .filter((device) => matchesQuery(normalized, [device.name, device.id, device.platform, device.arch, device.agentdock_version, device.status]))
          .map((device) => ({
            id: `device:${device.id}`,
            label: device.name || device.id,
            description: `${device.status || 'unknown'} / ${device.platform || 'unknown'} / ${device.arch || 'unknown'}`,
            target: 'devices' as Section,
            source: 'device' as const,
          })),
        ...schedules
          .filter((schedule) => matchesQuery(normalized, [schedule.title, schedule.id, schedule.device, schedule.provider, schedule.state, schedule.message]))
          .map((schedule) => ({
            id: `schedule:${schedule.id}`,
            label: schedule.title,
            description: `${schedule.state || 'unknown'} / ${schedule.device} / ${schedule.schedule}`,
            target: 'schedules' as Section,
            source: 'schedule' as const,
          })),
      ].slice(0, 8);

      setState({
        data: results,
        live: devicesResult.status === 'fulfilled' || schedulesResult.status === 'fulfilled',
        loading: false,
        error: hardFailure ? messageOf(firstFailure.reason) : undefined,
      });
    })();
    return () => { cancelled = true; };
  }, [normalized]);

  return state;
}

export default function App() {
  const [section, setSection] = useState<Section>(sectionFromHash);
  const [menuOpen, setMenuOpen] = useState(false);
  const [refreshToken, setRefreshToken] = useState(0);
  const [globalQuery, setGlobalQuery] = useState('');

  useEffect(() => {
    const onHash = () => setSection(sectionFromHash());
    window.addEventListener('hashchange', onHash);
    return () => window.removeEventListener('hashchange', onHash);
  }, []);

  function navigate(next: Section) {
    window.location.hash = next;
    setSection(next);
    setMenuOpen(false);
    setGlobalQuery('');
  }

  const sectionResults = useMemo(() => searchSections(globalQuery), [globalQuery]);
  const liveSearch = useLiveSearch(globalQuery);
  const searchResults = useMemo(() => {
    const seen = new Set<string>();
    return [...liveSearch.data, ...sectionResults].filter((result) => {
      if (seen.has(result.id)) return false;
      seen.add(result.id);
      return true;
    }).slice(0, 8);
  }, [liveSearch.data, sectionResults]);

  function submitSearch(event: FormEvent) {
    event.preventDefault();
    if (searchResults[0]) navigate(searchResults[0].target);
  }

  if (section === 'memory') {
    return (
      <div className="nexus-memory-mode">
        <button className="nexus-memory-return" onClick={() => navigate('home')}>
          <ChevronRight size={15} /> 返回 Nexus
        </button>
        <MemoryWorkspace />
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
            return (
              <button key={item.id} className={section === item.id ? 'active' : ''} onClick={() => navigate(item.id)}>
                <Icon size={18} /><span>{item.label}</span>
              </button>
            );
          })}
        </nav>
        <div className="nexus-sidebar-foot"><ShieldCheck size={16} /><span>Control Plane</span></div>
      </aside>
      {menuOpen && <button className="nexus-scrim" aria-label="关闭菜单" onClick={() => setMenuOpen(false)} />}
      <main className="nexus-main">
        <header className="nexus-topbar">
          <button className="nexus-mobile-menu" aria-label="切换菜单" onClick={() => setMenuOpen((value) => !value)}>{menuOpen ? <X /> : <Menu />}</button>
          <div><span className="nexus-eyebrow">AgentDock Nexus</span><h1>{active.label}</h1></div>
          <div className="nexus-top-actions">
            <form className="nexus-search-wrap" onSubmit={submitSearch}>
              <label className="nexus-search"><Search size={16} /><input value={globalQuery} onChange={(event) => setGlobalQuery(event.target.value)} placeholder="搜索设备、Skill、Run" aria-label="全局搜索" /></label>
              {globalQuery.trim() && (
                <div className="nexus-search-popover">
                  {searchResults.length ? searchResults.map((result) => (
                    <button type="button" key={result.id} onClick={() => navigate(result.target)}>
                      <strong>{result.label}</strong><span>{result.source === 'section' ? '入口' : result.source === 'device' ? '设备' : '计划任务'} / {result.description}</span>
                    </button>
                  )) : liveSearch.loading ? <p>正在搜索真实 Nexus 数据…</p> : <p>没有匹配的控制面入口或真实对象</p>}
                  {liveSearch.error && <small>{liveSearch.error}</small>}
                </div>
              )}
            </form>
            <button className="icon-button" title="刷新" onClick={() => setRefreshToken((value) => value + 1)}><RefreshCw size={17} /></button>
          </div>
        </header>
        <div className="nexus-content">
          {section === 'home' && <HomePage refreshToken={refreshToken} navigate={navigate} />}
          {section === 'inbox' && <InboxPage refreshToken={refreshToken} />}
          {section === 'devices' && <DevicesPage refreshToken={refreshToken} />}
          {section === 'skills' && <SkillsPage refreshToken={refreshToken} />}
          {section === 'runs' && <RunsPage refreshToken={refreshToken} />}
          {section === 'schedules' && <SchedulesPage refreshToken={refreshToken} />}
          {section === 'settings' && <SettingsPage />}
        </div>
      </main>
    </div>
  );
}

function HomePage({ refreshToken, navigate }: { refreshToken: number; navigate: (section: Section) => void }) {
  const resource = useOverview(refreshToken);
  const overview = { ...EMPTY_OVERVIEW, ...resource.data };
  const cards = useMemo(() => [
    ['Agent 待办', overview.agent_tasks, Bot, 'inbox' as Section, 'warn' as Tone],
    ['用户待办', overview.user_tasks, BellRing, 'inbox' as Section, 'warn' as Tone],
    ['设备异常', overview.device_alerts, Server, 'devices' as Section, 'danger' as Tone],
    ['Skill 候选', overview.skill_candidates, Sparkles, 'skills' as Section, 'ok' as Tone],
    ['Memory 冲突', overview.memory_conflicts, Database, 'memory' as Section, 'danger' as Tone],
    ['最近失败', overview.recent_failures, CircleAlert, 'runs' as Section, 'danger' as Tone],
  ] as const, [overview.agent_tasks, overview.user_tasks, overview.device_alerts, overview.skill_candidates, overview.memory_conflicts, overview.recent_failures]);

  return (
    <>
      <section className="nexus-hero">
        <div><span className="nexus-kicker">统一控制面</span><h2>设备、记忆、Skill 与任务，一处掌控</h2><p>从 Agent Inbox 到多设备运行证据，形成可审计、可回退的完整闭环。</p></div>
        <StatusBadge tone={resource.error ? 'danger' : resource.live ? 'ok' : 'warn'}>{resource.error ? 'API 访问受限' : resource.live ? '实时数据已连接' : '等待 Nexus API 合并'}</StatusBadge>
      </section>
      {resource.error && <InlineAlert tone="danger" title="概览数据读取失败" message={resource.error} />}
      <section className="metric-grid">
        {cards.map(([label, value, Icon, target, tone]) => (
          <button className="metric-card" key={label} onClick={() => navigate(target)}>
            <span className={`metric-icon tone-${tone}`}><Icon size={20} /></span>
            <span className="metric-value">{value}</span><span className="metric-label">{label}</span><ChevronRight size={17} className="metric-arrow" />
          </button>
        ))}
      </section>
      <section className="two-column">
        <Panel title="系统态势" subtitle="当前 Nexus 集成状态">
          <TimelineItem icon={<CircleCheck size={16} />} title="Memory 工作区已接入" detail="保留目录、Diff、时间线和移动端体验" tone="ok" />
          <TimelineItem icon={<Activity size={16} />} title="控制面数据自动探测" detail="后端分支合并后自动显示实时状态" tone="warn" />
          <TimelineItem icon={<ShieldCheck size={16} />} title="安全与迁移测试入口已建立" detail="覆盖恶意包、路径逃逸、Secret 泄露和回退" tone="ok" />
        </Panel>
        <Panel title="闭环进度" subtitle="M0 → M6 产品里程碑">
          {['契约冻结', 'Nexus Core', 'Memory + Task', '多设备', 'Skill MVP', 'Evolution', '产品完成'].map((name, index) => <ProgressRow key={name} name={name} value={index < 2 ? 100 : index < 6 ? 55 : 30} />)}
        </Panel>
      </section>
    </>
  );
}

function InboxPage({ refreshToken }: { refreshToken: number }) {
  const resource = useResource<Task[]>(['/api/v1/tasks', '/api/tasks'], [], refreshToken);
  const tasks = Array.isArray(resource.data) ? resource.data : [];
  return (
    <CollectionPage title="Agent Inbox" description="统一处理 needs_agent、needs_user、review 与 automatic 任务。" live={resource.live} loading={resource.loading} error={resource.error} count={tasks.length} empty="暂无任务。设备告警、Skill 失败、Memory 冲突和 Evolution Proposal 会自动进入这里。">
      {tasks.map((task) => <ListCard key={task.id} title={task.title} meta={`${task.type} · ${task.source || 'unknown source'}`} trailing={<><StatusBadge tone={toneForStatus(task.status)}>{task.status}</StatusBadge><small>{formatTime(task.updated_at)}</small></>} />)}
    </CollectionPage>
  );
}

function DevicesPage({ refreshToken }: { refreshToken: number }) {
  return <DevicesManagementPage refreshToken={refreshToken} />;
}

function SkillsPage({ refreshToken }: { refreshToken: number }) {
  const resource = useResource<Skill[]>(['/api/v1/skills', '/api/skills'], [], refreshToken);
  const skills = Array.isArray(resource.data) ? resource.data : [];
  return (
    <CollectionPage title="Skill Catalog" description="查看规范、Operations、安装设备、Runs、Evolution 与版本。" live={resource.live} loading={resource.loading} error={resource.error} count={skills.length} empty="Catalog 为空。导入外部 Skill 后将显示 provenance、trust、maturity 和 release。">
      <div className="card-grid">{skills.map((skill) => <EntityCard key={skill.id} icon={<Boxes size={20} />} title={skill.name} status={skill.maturity || 'draft'} detail={`v${skill.version || '0.0.0'} · trust: ${skill.trust || 'unverified'}`} leftLabel="安装设备" leftValue={String(skill.installations ?? 0)} rightLabel="Release" rightValue={skill.version || '无'} />)}</div>
    </CollectionPage>
  );
}

function RunsPage({ refreshToken }: { refreshToken: number }) {
  const resource = useResource<Run[]>(['/api/v1/runs', '/api/runs'], [], refreshToken);
  const runs = Array.isArray(resource.data) ? resource.data : [];
  return (
    <CollectionPage title="Runs & Evidence" description="统一查看步骤、证据、验证结果和失败层级。" live={resource.live} loading={resource.loading} error={resource.error} count={runs.length} empty="暂无 Run。Skill 执行或设备命令完成后会记录到统一 Run Registry。">
      {runs.map((run) => <ListCard key={run.id} title={run.title || run.skill || run.id} meta={`${run.device || '未知设备'} · ${formatTime(run.started_at)}`} trailing={<StatusBadge tone={toneForStatus(run.status)}>{run.status}</StatusBadge>} />)}
    </CollectionPage>
  );
}

function formatBytes(value?: number): string {
  if (!value || value < 0) return '暂无';
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
  let size = value;
  let unit = 0;
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024;
    unit += 1;
  }
  return `${size.toFixed(unit === 0 ? 0 : 2)} ${units[unit]}`;
}

function SchedulesPage({ refreshToken }: { refreshToken: number }) {
  const resource = useResource<Schedule[]>(['/api/v1/schedules', '/v1/schedules'], [], refreshToken);
  const schedules = Array.isArray(resource.data) ? resource.data : [];
  return (
    <CollectionPage title="计划任务" description="查看 DockMini 的真实定时计划、最近执行证据与云端归档历史。" live={resource.live} loading={resource.loading} error={resource.error} count={schedules.length} empty="暂无计划任务。">
      {schedules.map((schedule) => (
        <article className="schedule-card" key={schedule.id}>
          <header className="schedule-card-head">
            <div>
              <div className="schedule-title-row"><h3>{schedule.title}</h3><StatusBadge tone={schedule.enabled ? 'ok' : 'muted'}>{schedule.enabled ? '已启用' : '已停用'}</StatusBadge></div>
              <p>{schedule.description}</p>
            </div>
            <StatusBadge tone={toneForStatus(schedule.state)}>{schedule.state || 'unknown'}</StatusBadge>
          </header>
          <dl className="schedule-detail-grid">
            <div><dt>执行计划</dt><dd>{schedule.schedule}</dd></div>
            <div><dt>设备 / Provider</dt><dd>{schedule.device} · {schedule.provider}</dd></div>
            <div><dt>最近开始</dt><dd>{formatTime(schedule.last_started_at)}</dd></div>
            <div><dt>最近完成</dt><dd>{formatTime(schedule.last_completed_at)}</dd></div>
            <div><dt>下次运行</dt><dd>{formatTime(schedule.next_run_at)}</dd></div>
            <div><dt>归档大小</dt><dd>{formatBytes(schedule.archive_size)}</dd></div>
          </dl>
          <div className="schedule-evidence">
            <div><span>归档名称</span><code>{schedule.archive || '暂无'}</code></div>
            <div><span>SHA256</span><code>{schedule.sha256 || '暂无'}</code></div>
            <div><span>远端路径</span><code>{schedule.remote_path || '暂无'}</code></div>
            {schedule.message && <div><span>状态消息</span><p>{schedule.message}</p></div>}
          </div>
          <section className="schedule-history">
            <h4>最近历史记录</h4>
            {(schedule.history || []).length > 0 ? (schedule.history || []).slice().reverse().slice(0, 10).map((entry, index) => (
              <div className="schedule-history-row" key={`${entry.completed_at || entry.started_at || index}-${index}`}>
                <StatusBadge tone={toneForStatus(entry.state)}>{entry.state || 'unknown'}</StatusBadge>
                <div><strong>{formatTime(entry.completed_at || entry.started_at)}</strong><small>{entry.archive || entry.message || '无归档信息'}</small></div>
                <span>{formatBytes(entry.archive_size)}</span>
              </div>
            )) : <p className="schedule-history-empty">暂无历史记录</p>}
          </section>
        </article>
      ))}
    </CollectionPage>
  );
}

function SettingsPage() {
  return (
    <>
      <EnvManagerPage />
      <section className="settings-grid">
        <Panel title="认证与访问" subtitle="用户、Agent 与设备身份"><SettingRow label="User Session" detail="浏览器登录与会话管理" /><SettingRow label="Agent Token" detail="Scope 限制与撤销" /><SettingRow label="Device Token" detail="Enrollment 后独立轮换" /></Panel>
        <Panel title="发布策略" subtitle="Skill 与设备控制"><SettingRow label="默认 Channel" detail="stable" /><SettingRow label="Canary 验证" detail="发布前必须有 Verification Result" /><SettingRow label="自动回退" detail="验证失败时保持旧版本" /></Panel>
        <Panel title="审计与保留" subtitle="所有写操作均可追踪"><SettingRow label="Audit Event" detail="actor / action / object / result / risk" /><SettingRow label="Evidence" detail="保留脱敏后的运行证据" /><SettingRow label="Export" detail="禁止携带私有路径和 Secret" /></Panel>
      </section>
    </>
  );
}

function CollectionPage({ title, description, live, loading, error, count, empty, children }: { title: string; description: string; live: boolean; loading: boolean; error?: string; count: number; empty: string; children: ReactNode }) {
  return <section><div className="section-heading"><div><h2>{title}</h2><p>{description}</p></div><StatusBadge tone={error ? 'danger' : live ? 'ok' : 'warn'}>{error ? 'API 访问受限' : live ? 'Live API' : 'Compatibility mode'}</StatusBadge></div>{loading ? <EmptyState text="正在读取 Nexus 数据…" /> : error ? <ErrorState text={error} /> : count > 0 && children ? <div className="collection-stack">{children}</div> : <EmptyState text={empty} />}</section>;
}

function Panel({ title, subtitle, children }: { title: string; subtitle: string; children: ReactNode }) { return <article className="nexus-panel"><header><div><h3>{title}</h3><p>{subtitle}</p></div></header><div className="panel-body">{children}</div></article>; }
function StatusBadge({ tone, children }: { tone: Tone; children: ReactNode }) { return <span className={`status-badge tone-${tone}`}><span />{children}</span>; }
function TimelineItem({ icon, title, detail, tone }: { icon: ReactNode; title: string; detail: string; tone: Tone }) { return <div className="timeline-item"><span className={`timeline-icon tone-${tone}`}>{icon}</span><div><strong>{title}</strong><p>{detail}</p></div></div>; }
function ProgressRow({ name, value }: { name: string; value: number }) { return <div className="progress-row"><div><span>{name}</span><strong>{value}%</strong></div><div className="progress-track"><span style={{ width: `${value}%` }} /></div></div>; }
function ListCard({ title, meta, trailing }: { title: string; meta: string; trailing: ReactNode }) { return <article className="list-card"><div><h3>{title}</h3><p>{meta}</p></div><div className="list-trailing">{trailing}</div></article>; }
function InlineAlert({ tone, title, message }: { tone: Tone; title: string; message: string }) { return <div className={`nexus-inline-alert tone-${tone}`}><strong>{title}</strong><span>{message}</span></div>; }
function EmptyState({ text }: { text: string }) { return <div className="empty-state"><span><Activity size={24} /></span><h3>等待数据</h3><p>{text}</p></div>; }
function ErrorState({ text }: { text: string }) { return <div className="empty-state error-state"><span><CircleAlert size={24} /></span><h3>数据读取失败</h3><p>{text}</p></div>; }
function SettingRow({ label, detail }: { label: string; detail: string }) { return <div className="setting-row"><div><strong>{label}</strong><p>{detail}</p></div><ChevronRight size={17} /></div>; }
function EntityCard({ icon, title, status, detail, leftLabel, leftValue, rightLabel, rightValue }: { icon: ReactNode; title: string; status: string; detail: string; leftLabel: string; leftValue: string; rightLabel: string; rightValue: string }) { return <article className="entity-card"><div className="entity-head"><span className="entity-avatar">{icon}</span><StatusBadge tone={toneForStatus(status)}>{status}</StatusBadge></div><h3>{title}</h3><p>{detail}</p><dl><div><dt>{leftLabel}</dt><dd>{leftValue}</dd></div><div><dt>{rightLabel}</dt><dd>{rightValue}</dd></div></dl></article>; }
