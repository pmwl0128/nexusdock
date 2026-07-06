import { useEffect, useMemo, useState, type ReactNode } from 'react';
import { CheckCircle2, FileText, Layers, RefreshCw, Search, ShieldAlert, Trash2 } from 'lucide-react';
import { ApiError, api } from '../../api/client';

type Tone = 'ok' | 'warn' | 'danger' | 'muted';
type TaskStatus = 'all' | 'active' | 'completed' | 'blocked';

type OpsTask = { id: string; title: string; goal: string; status: string; phase: string; review_status: string; blocker?: string; updated_at: string; created_at: string; template_id?: string; template_version?: string; condition_count: number; step_count: number; attempt_count: number; event_count: number; cleanable: boolean; file_name: string };
type OpsSkill = { id: string; title: string; source: string; path: string; description?: string; updated_at: string; file_count: number; status: string };
type OpsLog = { name: string; path: string; size_bytes: number; updated_at: string; tail: string };
type OpsPaths = { agentdock?: string; workspace?: string; workflows?: string; deploy?: string; source?: string };
type TaskListResponse = { ok: boolean; items: OpsTask[]; count: number; total: number; root: string };
type CleanupResponse = { ok: boolean; dry_run: boolean; changed: OpsTask[]; count: number };
type SkillsResponse = { ok: boolean; items: OpsSkill[]; count: number; root: string };
type CapabilitiesResponse = { ok: boolean; tools: Array<{ name: string; category: string; status: string; description: string }>; counts: Record<string, unknown>; paths: OpsPaths };
type LogsResponse = { ok: boolean; items: OpsLog[]; count: number; roots: string[] };
type DeploymentResponse = { ok: boolean; service: string; health: { ok: boolean; addr: string }; paths: OpsPaths; compose: string; source: { dir: string; commit: string }; image?: string; updated_at: string };

const emptyTasks: TaskListResponse = { ok: false, items: [], count: 0, total: 0, root: '' };
function formatTime(value?: string): string { if (!value) return '暂无'; const date = new Date(value); return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat('zh-CN', { dateStyle: 'short', timeStyle: 'short' }).format(date); }
function formatBytes(value?: number): string { if (value === undefined) return '暂无'; const units = ['B', 'KiB', 'MiB', 'GiB']; let size = value; let unit = 0; while (size >= 1024 && unit < units.length - 1) { size /= 1024; unit += 1; } return `${size.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`; }
function apiMessage(error: unknown): string { if (error instanceof ApiError) return `${error.code || error.status}：${error.message}`; return error instanceof Error ? error.message : '请求失败'; }
function toneForTask(task: Pick<OpsTask, 'status' | 'phase' | 'review_status'>): Tone { if (task.status === 'completed') return 'ok'; if (task.status === 'blocked') return 'danger'; if (task.phase === 'closeout' && task.review_status === 'pass') return 'warn'; if (task.status === 'active') return 'warn'; return 'muted'; }
function shortHash(value?: string): string { return value ? value.slice(0, 8) : 'unknown'; }

function useOpsResource<T>(path: string, fallback: T, refreshToken: number) {
  const [localToken, setLocalToken] = useState(0);
  const [state, setState] = useState<{ data: T; loading: boolean; error?: string }>({ data: fallback, loading: true });
  useEffect(() => {
    let cancelled = false;
    setState((current) => ({ ...current, loading: true, error: undefined }));
    api<T>(path).then((data) => { if (!cancelled) setState({ data, loading: false }); }).catch((error) => { if (!cancelled) setState({ data: fallback, loading: false, error: apiMessage(error) }); });
    return () => { cancelled = true; };
  }, [path, refreshToken, localToken]);
  return { ...state, reload: () => setLocalToken((value) => value + 1) };
}
export function TaskCenterPage({ refreshToken }: { refreshToken: number }) {
  const [status, setStatus] = useState<TaskStatus>('active');
  const [query, setQuery] = useState('');
  const path = `/v1/ops/tasks?status=${status}&limit=300${query.trim() ? `&q=${encodeURIComponent(query.trim())}` : ''}`;
  const resource = useOpsResource<TaskListResponse>(path, emptyTasks, refreshToken);
  const tasks = resource.data.items;
  const [selectedId, setSelectedId] = useState('');
  const selected = tasks.find((item) => item.id === selectedId) || tasks[0];
  const stats = useMemo(() => ({
    active: tasks.filter((item) => item.status === 'active').length,
    blocked: tasks.filter((item) => item.status === 'blocked').length,
    completed: tasks.filter((item) => item.status === 'completed').length,
    cleanable: tasks.filter((item) => item.cleanable).length,
  }), [tasks]);

  return <OpsShell title="任务中心" subtitle="按真实任务 JSON 展示状态、阶段、review 和清理信号。Active 是任务记录状态，不等于后台进程。" loading={resource.loading} error={resource.error} onReload={resource.reload}>
    <section className="ops-command-hero"><div><span>AGENTDOCK TASKS</span><h3>{resource.data.total || tasks.length} 条任务记录</h3><p>{resource.data.root || '任务目录未配置'} · 当前筛选 {resource.data.count} 条</p></div><StatusBadge tone={stats.blocked ? 'danger' : stats.cleanable ? 'warn' : 'ok'}>{stats.blocked ? `${stats.blocked} blocked` : stats.cleanable ? `${stats.cleanable} cleanable` : 'healthy'}</StatusBadge></section>
    <section className="ops-metrics is-dashboard"><Metric label="Active" value={String(stats.active)} tone="warn" /><Metric label="Blocked" value={String(stats.blocked)} tone={stats.blocked ? 'danger' : 'muted'} /><Metric label="Completed" value={String(stats.completed)} tone="ok" /><Metric label="可清理" value={String(stats.cleanable)} tone={stats.cleanable ? 'warn' : 'muted'} /></section>
    <div className="ops-toolbar is-console"><div className="ops-segmented">{(['active', 'blocked', 'completed', 'all'] as TaskStatus[]).map((item) => <button key={item} className={status === item ? 'is-active' : ''} onClick={() => setStatus(item)}>{item}</button>)}</div><label className="ops-search"><Search size={15} /><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索标题、目标、模板、阻塞原因" /></label><span className="ops-count">{resource.data.count} / {resource.data.total}</span></div>
    <section className="ops-master-detail"><div className="ops-task-rail">{tasks.length === 0 ? <EmptyOps text="没有匹配任务。" /> : tasks.map((task) => <button key={task.id} className={`ops-task-line ${selected?.id === task.id ? 'is-selected' : ''}`} onClick={() => setSelectedId(task.id)}><StatusDot tone={toneForTask(task)} /><span><strong>{task.title || task.id}</strong><small>{task.phase || 'no phase'} · {formatTime(task.updated_at)}</small></span>{task.cleanable && <em>清理</em>}</button>)}</div><TaskDetail task={selected} /></section>
  </OpsShell>;
}

export function TaskCleanupPage({ refreshToken }: { refreshToken: number }) {
  const resource = useOpsResource<TaskListResponse>('/v1/ops/tasks?status=active&limit=500', emptyTasks, refreshToken);
  const candidates = resource.data.items.filter((item) => item.cleanable);
  const [busy, setBusy] = useState(false); const [result, setResult] = useState<CleanupResponse | null>(null); const [error, setError] = useState('');
  async function run(dryRun: boolean) { setBusy(true); setError(''); setResult(null); try { const response = await api<CleanupResponse>('/v1/ops/tasks/cleanup', { method: 'POST', body: JSON.stringify({ dry_run: dryRun, limit: 200 }) }); setResult(response); resource.reload(); } catch (err) { setError(apiMessage(err)); } finally { setBusy(false); } }
  return <OpsShell title="任务清理" subtitle="只处理 active + closeout + final_review pass 的任务，避免误动未验证任务。" loading={resource.loading || busy} error={resource.error || error} onReload={resource.reload}>
    <section className="ops-cleanup-hero is-large"><div><span>SAFE CLEANUP</span><strong>{candidates.length}</strong><p>这些任务已经 final_review pass，但状态仍停在 active。先预览，再执行。</p></div><div className="ops-actions"><button className="nx-button is-secondary" onClick={() => void run(true)} disabled={busy}><CheckCircle2 size={15} />预览变更</button><button className="nx-button is-danger" onClick={() => void run(false)} disabled={busy || candidates.length === 0}><Trash2 size={15} />标记完成</button></div></section>
    {result && <div className={`nx-alert is-${result.dry_run ? 'warning' : 'success'}`}>{result.dry_run ? '预览' : '已清理'} {result.count} 条任务。</div>}
    <section className="ops-grid cards">{candidates.length === 0 ? <EmptyOps text="当前没有可清理任务。" /> : candidates.map((task) => <TaskCard key={task.id} task={task} />)}</section>
  </OpsShell>;
}
export function SkillsPage({ refreshToken }: { refreshToken: number }) {
  const resource = useOpsResource<SkillsResponse>('/v1/ops/skills', { ok: false, items: [], count: 0, root: '' }, refreshToken);
  const runtime = resource.data.items.filter((item) => item.source === 'runtime').length;
  return <OpsShell title="Skill 管理" subtitle="本机已安装 Skill、来源、说明和文件规模。" loading={resource.loading} error={resource.error} onReload={resource.reload}>
    <section className="ops-command-hero is-soft"><div><span>SKILL RUNTIME</span><h3>{resource.data.count} 个 Skill</h3><p>{runtime} 个 runtime Skill · {resource.data.root || '未挂载 skills 目录'}</p></div><StatusBadge tone={resource.data.count ? 'ok' : 'warn'}>{resource.data.count ? 'installed' : 'empty'}</StatusBadge></section>
    <div className="ops-grid cards is-rich">{resource.data.items.length === 0 ? <EmptyOps text="没有读取到 Skill。" /> : resource.data.items.map((skill) => <article className="ops-card skill-card" key={`${skill.source}:${skill.id}`}><header><span className="ops-card-icon"><Layers size={18} /></span><StatusBadge tone="ok">{skill.status}</StatusBadge></header><h3>{skill.title}</h3><p>{skill.description || '暂无说明。'}</p><dl><div><dt>ID</dt><dd>{skill.id}</dd></div><div><dt>来源</dt><dd>{skill.source}</dd></div><div><dt>文件</dt><dd>{skill.file_count}</dd></div><div><dt>更新</dt><dd>{formatTime(skill.updated_at)}</dd></div></dl><code>{skill.path}</code></article>)}</div>
  </OpsShell>;
}

export function CapabilitiesPage({ refreshToken }: { refreshToken: number }) {
  const resource = useOpsResource<CapabilitiesResponse>('/v1/ops/capabilities', { ok: false, tools: [], counts: {}, paths: {} }, refreshToken);
  const counts = resource.data.counts;
  const workflowCounts = (counts.workflows && typeof counts.workflows === 'object' ? counts.workflows : {}) as Record<string, unknown>;
  const groups = resource.data.tools.reduce<Record<string, CapabilitiesResponse['tools']>>((acc, tool) => {
    const key = tool.category || 'other';
    acc[key] = [...(acc[key] || []), tool];
    return acc;
  }, {});
  const availableTools = resource.data.tools.filter((tool) => tool.status === 'available').length;
  const workflowTotal = Number(workflowCounts.published || 0) + Number(workflowCounts.drafts || 0) + Number(workflowCounts.retired || 0);
  return <OpsShell title="Capability" subtitle="把当前可用工具、任务、Skill、模板和路径整理成能力矩阵，而不是原始接口字段。" loading={resource.loading} error={resource.error} onReload={resource.reload}>
    <section className="cap-hero"><div><span>CAPABILITY MATRIX</span><h3>{availableTools} / {resource.data.tools.length} 个工具可用</h3><p>能力页用于判断当前 Nexus 能看见什么、能操作什么、数据从哪里来。</p></div><StatusBadge tone={availableTools ? 'ok' : 'warn'}>{availableTools ? 'available' : 'empty'}</StatusBadge></section>
    <section className="cap-summary-grid">
      <CapabilitySummary title="任务系统" value={String(counts.tasks ?? 0)} detail="持久化任务记录" tone="warn" />
      <CapabilitySummary title="Skill Runtime" value={String(counts.skills ?? 0)} detail="本机可见 Skill" tone="ok" />
      <CapabilitySummary title="Workflow 模板" value={String(workflowTotal || workflowCounts.published || 0)} detail={`${workflowCounts.published ?? 0} published · ${workflowCounts.drafts ?? 0} drafts`} tone="muted" />
      <CapabilitySummary title="工具能力" value={String(resource.data.tools.length)} detail={`${availableTools} available`} tone={availableTools ? 'ok' : 'warn'} />
    </section>
    <section className="cap-layout">
      <article className="cap-tool-panel"><header><div><h3>工具分组</h3><p>按能力域展示，不再平铺成无意义卡片。</p></div><StatusBadge tone="muted">{Object.keys(groups).length} groups</StatusBadge></header>{Object.entries(groups).length === 0 ? <EmptyOps text="没有可展示工具。" /> : Object.entries(groups).map(([category, tools]) => <div className="cap-group" key={category}><div className="cap-group-head"><strong>{category}</strong><span>{tools.length} tools</span></div>{tools.map((tool) => <div className="cap-tool-row" key={tool.name}><StatusDot tone={tool.status === 'available' ? 'ok' : 'warn'} /><div><strong>{tool.name}</strong><small>{tool.description}</small></div><em>{tool.status}</em></div>)}</div>)}</article>
      <aside className="cap-side"><article className="cap-mini-panel"><h3>模板状态</h3><div className="cap-kv"><span>Published</span><strong>{String(workflowCounts.published ?? 0)}</strong></div><div className="cap-kv"><span>Drafts</span><strong>{String(workflowCounts.drafts ?? 0)}</strong></div><div className="cap-kv"><span>Retired</span><strong>{String(workflowCounts.retired ?? 0)}</strong></div></article><PathPanel paths={resource.data.paths} /></aside>
    </section>
  </OpsShell>;
}

export function LogsPage({ refreshToken }: { refreshToken: number }) {
  const resource = useOpsResource<LogsResponse>('/v1/ops/logs', { ok: false, items: [], count: 0, roots: [] }, refreshToken);
  const [selectedPath, setSelectedPath] = useState('');
  const selected = resource.data.items.find((item) => item.path === selectedPath) || resource.data.items[0];
  return <OpsShell title="运行日志" subtitle="按更新时间聚合可读取日志，左侧选择文件，右侧查看尾部内容。" loading={resource.loading} error={resource.error} onReload={resource.reload}>
    <section className="ops-master-detail logs-layout"><div className="ops-task-rail">{resource.data.items.length === 0 ? <EmptyOps text="当前没有可展示日志。" /> : resource.data.items.map((item) => <button key={`${item.path}:${item.updated_at}`} className={`ops-task-line ${selected?.path === item.path ? 'is-selected' : ''}`} onClick={() => setSelectedPath(item.path)}><StatusDot tone="muted" /><span><strong>{item.name}</strong><small>{formatBytes(item.size_bytes)} · {formatTime(item.updated_at)}</small></span></button>)}</div><article className="ops-log is-detail"><header><div><strong>{selected?.name || '暂无日志'}</strong><small>{selected?.path || resource.data.roots.join(', ')}</small></div><FileText size={17} /></header><pre>{selected?.tail || '空日志'}</pre></article></section>
  </OpsShell>;
}

export function DeploymentPage({ refreshToken }: { refreshToken: number }) {
  const resource = useOpsResource<DeploymentResponse>('/v1/ops/deployment', { ok: false, service: 'recalldock', health: { ok: false, addr: '' }, paths: {}, compose: '', source: { dir: '', commit: '' }, updated_at: '' }, refreshToken);
  const commit = resource.data.source?.commit || '';
  return <OpsShell title="部署中心" subtitle="生产容器看到的健康、源码、镜像、目录和配置。" loading={resource.loading} error={resource.error} onReload={resource.reload}>
    <section className="ops-command-hero deploy-hero"><div><span>PRODUCTION</span><h3>{resource.data.service}</h3><p>{resource.data.health?.addr || 'addr unknown'} · {formatTime(resource.data.updated_at)}</p></div><StatusBadge tone={resource.data.health?.ok ? 'ok' : 'danger'}>{resource.data.health?.ok ? 'healthy' : 'unknown'}</StatusBadge></section>
    <section className="ops-metrics is-dashboard"><Metric label="服务" value={resource.data.service} /><Metric label="健康" value={resource.data.health?.ok ? 'ok' : 'unknown'} tone={resource.data.health?.ok ? 'ok' : 'warn'} /><Metric label="提交" value={shortHash(commit)} /><Metric label="镜像" value={resource.data.image || 'local'} /></section>
    <section className="deploy-grid"><PathPanel paths={resource.data.paths} /><article className="ops-card deploy-card"><header><span className="ops-card-icon"><FileText size={18} /></span><StatusBadge tone="muted">配置</StatusBadge></header><h3>Compose 配置</h3><p>{resource.data.paths?.deploy || '未配置部署目录'}</p><pre>{resource.data.compose || '未读取到配置。'}</pre></article></section>
  </OpsShell>;
}
function TaskDetail({ task }: { task?: OpsTask }) {
  if (!task) return <article className="ops-detail-empty"><EmptyOps text="请选择一个任务。" /></article>;
  const completion = Math.max(0, Math.min(100, Math.round((task.step_count ? task.condition_count / Math.max(task.condition_count, task.step_count) : 0) * 100)));
  return <article className="ops-task-detail"><header><div><span>任务详情</span><h3>{task.title || task.id}</h3><p>{task.goal}</p></div><StatusBadge tone={toneForTask(task)}>{task.status}</StatusBadge></header>{task.blocker && <div className="ops-blocker"><ShieldAlert size={15} />{task.blocker}</div>}<div className="ops-detail-grid"><Info label="阶段" value={task.phase || 'none'} /><Info label="Review" value={task.review_status || 'not_started'} /><Info label="条件" value={String(task.condition_count)} /><Info label="步骤" value={String(task.step_count)} /><Info label="事件" value={String(task.event_count)} /><Info label="更新" value={formatTime(task.updated_at)} /></div><div className="ops-progress"><div><span>完成信号</span><strong>{completion}%</strong></div><i style={{ width: `${completion}%` }} /></div><footer><code>{task.id}</code>{task.template_id && <code>{task.template_id}@{task.template_version}</code>}{task.cleanable && <StatusBadge tone="warn">可清理</StatusBadge>}</footer></article>;
}

function TaskCard({ task }: { task: OpsTask }) { return <article className="ops-task-card"><header><div><strong>{task.title || task.id}</strong><small>{task.id} · {formatTime(task.updated_at)}</small></div><StatusBadge tone={toneForTask(task)}>{task.status}</StatusBadge></header><p>{task.goal}</p>{task.blocker && <div className="ops-blocker"><ShieldAlert size={15} />{task.blocker}</div>}<footer><span>{task.phase || 'no phase'}</span><span>review: {task.review_status}</span><span>{task.condition_count} 条件 / {task.step_count} 步骤</span>{task.template_id && <span>{task.template_id}@{task.template_version}</span>}{task.cleanable && <strong>可清理</strong>}</footer></article>; }
function OpsShell({ title, subtitle, loading, error, onReload, children }: { title: string; subtitle: string; loading: boolean; error?: string; onReload: () => void; children: ReactNode }) { return <section className="ops-page ops-console"><div className="section-heading ops-heading"><div><h2>{title}</h2><p>{subtitle}</p></div><button className="nx-button is-secondary" onClick={onReload} disabled={loading}><RefreshCw size={15} className={loading ? 'nx-spin' : ''} />刷新</button></div>{error && <div className="nx-alert is-error">{error}</div>}{children}</section>; }
function Metric({ label, value, tone = 'muted' }: { label: string; value: string; tone?: Tone }) { return <article><span className={`metric-icon tone-${tone}`}>{label.slice(0, 1)}</span><strong>{value}</strong><small>{label}</small></article>; }
function CapabilitySummary({ title, value, detail, tone }: { title: string; value: string; detail: string; tone: Tone }) { return <article className="cap-summary-card"><header><span className={`metric-icon tone-${tone}`}>{title.slice(0, 1)}</span>{title}</header><strong>{value}</strong><p>{detail}</p></article>; }
function Info({ label, value }: { label: string; value: string }) { return <div><dt>{label}</dt><dd>{value}</dd></div>; }
function StatusBadge({ tone, children }: { tone: Tone; children: ReactNode }) { return <span className={`status-badge tone-${tone}`}><span />{children}</span>; }
function StatusDot({ tone }: { tone: Tone }) { return <i className={`ops-dot tone-${tone}`} />; }
function EmptyOps({ text }: { text: string }) { return <p className="empty-mini">{text}</p>; }
function PathPanel({ paths }: { paths: OpsPaths }) { return <article className="ops-paths is-rich"><h3>路径</h3>{Object.entries(paths).map(([key, value]) => <div key={key}><span>{key}</span><code>{value || '未配置'}</code></div>)}</article>; }
