import { useEffect, useState, type ReactNode } from 'react';
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

function formatTime(value?: string): string { if (!value) return '暂无'; const date = new Date(value); return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat('zh-CN', { dateStyle: 'short', timeStyle: 'short' }).format(date); }
function formatBytes(value?: number): string { if (value === undefined) return '暂无'; const units = ['B', 'KiB', 'MiB', 'GiB']; let size = value; let unit = 0; while (size >= 1024 && unit < units.length - 1) { size /= 1024; unit += 1; } return `${size.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`; }
function apiMessage(error: unknown): string { if (error instanceof ApiError) return `${error.code || error.status}：${error.message}`; return error instanceof Error ? error.message : '请求失败'; }
function toneForTask(task: Pick<OpsTask, 'status' | 'phase' | 'review_status'>): Tone { if (task.status === 'completed') return 'ok'; if (task.status === 'blocked') return 'danger'; if (task.phase === 'closeout' && task.review_status === 'pass') return 'warn'; if (task.status === 'active') return 'warn'; return 'muted'; }

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
  const resource = useOpsResource<TaskListResponse>(path, { ok: false, items: [], count: 0, total: 0, root: '' }, refreshToken);
  const cleanable = resource.data.items.filter((item) => item.cleanable).length;
  return <OpsShell title="任务中心" subtitle="查看 AgentDock 真实任务状态。Active 是任务记录状态，不等于后台进程正在运行。" loading={resource.loading} error={resource.error} onReload={resource.reload}>
    <div className="ops-toolbar"><label><span>状态</span><select value={status} onChange={(event) => setStatus(event.target.value as TaskStatus)}><option value="active">Active</option><option value="completed">Completed</option><option value="blocked">Blocked</option><option value="all">全部</option></select></label><label className="ops-search"><Search size={15} /><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索标题、目标、模板、阻塞原因" /></label><span className="ops-count">{resource.data.count} / {resource.data.total} 条，{cleanable} 条可清理</span></div>
    <div className="ops-task-list">{resource.data.items.length === 0 ? <EmptyOps text="没有匹配任务。" /> : resource.data.items.map((task) => <TaskCard key={task.id} task={task} />)}</div>
  </OpsShell>;
}

export function TaskCleanupPage({ refreshToken }: { refreshToken: number }) {
  const resource = useOpsResource<TaskListResponse>('/v1/ops/tasks?status=active&limit=500', { ok: false, items: [], count: 0, total: 0, root: '' }, refreshToken);
  const candidates = resource.data.items.filter((item) => item.cleanable);
  const [busy, setBusy] = useState(false); const [result, setResult] = useState<CleanupResponse | null>(null); const [error, setError] = useState('');
  async function run(dryRun: boolean) { setBusy(true); setError(''); setResult(null); try { const response = await api<CleanupResponse>('/v1/ops/tasks/cleanup', { method: 'POST', body: JSON.stringify({ dry_run: dryRun, limit: 200 }) }); setResult(response); resource.reload(); } catch (err) { setError(apiMessage(err)); } finally { setBusy(false); } }
  return <OpsShell title="任务清理" subtitle="只处理 active + closeout + final_review pass 的任务，避免误动未验证任务。" loading={resource.loading || busy} error={resource.error || error} onReload={resource.reload}>
    <section className="ops-cleanup-hero"><div><strong>{candidates.length}</strong><span>可清理任务</span><p>这些任务已经 review pass，但状态仍停在 active。</p></div><div className="ops-actions"><button className="nx-button is-secondary" onClick={() => void run(true)} disabled={busy}><CheckCircle2 size={15} />预览</button><button className="nx-button is-danger" onClick={() => void run(false)} disabled={busy || candidates.length === 0}><Trash2 size={15} />清理</button></div></section>
    {result && <div className={`nx-alert is-${result.dry_run ? 'warning' : 'success'}`}>{result.dry_run ? '预览' : '已清理'} {result.count} 条任务。</div>}
    <div className="ops-task-list compact">{candidates.length === 0 ? <EmptyOps text="当前没有可清理任务。" /> : candidates.map((task) => <TaskCard key={task.id} task={task} />)}</div>
  </OpsShell>;
}
export function SkillsPage({ refreshToken }: { refreshToken: number }) {
  const resource = useOpsResource<SkillsResponse>('/v1/ops/skills', { ok: false, items: [], count: 0, root: '' }, refreshToken);
  return <OpsShell title="Skill 管理" subtitle="读取本机 AgentDock skills 目录，展示已安装 Skill 和来源。" loading={resource.loading} error={resource.error} onReload={resource.reload}>
    <div className="ops-grid cards">{resource.data.items.length === 0 ? <EmptyOps text="没有读取到 Skill。" /> : resource.data.items.map((skill) => <article className="ops-card" key={`${skill.source}:${skill.id}`}><header><Layers size={18} /><StatusBadge tone="ok">{skill.status}</StatusBadge></header><h3>{skill.title}</h3><p>{skill.description || '暂无说明。'}</p><dl><div><dt>ID</dt><dd>{skill.id}</dd></div><div><dt>来源</dt><dd>{skill.source}</dd></div><div><dt>文件</dt><dd>{skill.file_count}</dd></div><div><dt>更新</dt><dd>{formatTime(skill.updated_at)}</dd></div></dl><code>{skill.path}</code></article>)}</div>
  </OpsShell>;
}

export function CapabilitiesPage({ refreshToken }: { refreshToken: number }) {
  const resource = useOpsResource<CapabilitiesResponse>('/v1/ops/capabilities', { ok: false, tools: [], counts: {}, paths: {} }, refreshToken);
  const counts = resource.data.counts;
  return <OpsShell title="Capability" subtitle="展示 Nexus 当前可见的工具、任务、模板、Skill 和数据目录。" loading={resource.loading} error={resource.error} onReload={resource.reload}>
    <section className="ops-metrics"><Metric label="任务" value={String(counts.tasks ?? 0)} /><Metric label="Skill" value={String(counts.skills ?? 0)} /><Metric label="模板" value={JSON.stringify(counts.workflows ?? {})} wide /></section>
    <div className="ops-grid cards">{resource.data.tools.map((tool) => <article className="ops-card" key={tool.name}><header><Layers size={18} /><StatusBadge tone={tool.status === 'available' ? 'ok' : 'warn'}>{tool.status}</StatusBadge></header><h3>{tool.name}</h3><p>{tool.description}</p><code>{tool.category}</code></article>)}</div>
    <PathPanel paths={resource.data.paths} />
  </OpsShell>;
}

export function LogsPage({ refreshToken }: { refreshToken: number }) {
  const resource = useOpsResource<LogsResponse>('/v1/ops/logs', { ok: false, items: [], count: 0, roots: [] }, refreshToken);
  return <OpsShell title="运行日志" subtitle="聚合可读取的 AgentDock / workspace 日志文件，按更新时间排序并显示尾部内容。" loading={resource.loading} error={resource.error} onReload={resource.reload}>
    <div className="ops-log-list">{resource.data.items.length === 0 ? <EmptyOps text="当前挂载目录下没有可展示日志。" /> : resource.data.items.map((item) => <article className="ops-log" key={`${item.path}:${item.updated_at}`}><header><div><strong>{item.name}</strong><small>{item.path} · {formatBytes(item.size_bytes)} · {formatTime(item.updated_at)}</small></div><FileText size={17} /></header><pre>{item.tail || '空日志'}</pre></article>)}</div>
  </OpsShell>;
}

export function DeploymentPage({ refreshToken }: { refreshToken: number }) {
  const resource = useOpsResource<DeploymentResponse>('/v1/ops/deployment', { ok: false, service: 'recalldock', health: { ok: false, addr: '' }, paths: {}, compose: '', source: { dir: '', commit: '' }, updated_at: '' }, refreshToken);
  const commit = resource.data.source?.commit || '';
  return <OpsShell title="部署中心" subtitle="展示当前 Nexus 生产容器看到的部署目录、源码提交和 Compose 配置。" loading={resource.loading} error={resource.error} onReload={resource.reload}>
    <section className="ops-metrics"><Metric label="服务" value={resource.data.service} /><Metric label="健康" value={resource.data.health?.ok ? 'ok' : 'unknown'} /><Metric label="提交" value={commit ? commit.slice(0, 8) : 'unknown'} /></section>
    <PathPanel paths={resource.data.paths} />
    <article className="ops-log"><header><div><strong>docker-compose.yml</strong><small>{resource.data.paths?.deploy || '未配置'}</small></div><button className="nx-button is-secondary is-small" onClick={() => copyText(resource.data.compose)}>复制</button></header><pre>{resource.data.compose || '未读取到 compose。'}</pre></article>
  </OpsShell>;
}

function TaskCard({ task }: { task: OpsTask }) {
  return <article className="ops-task-card"><header><div><strong>{task.title || task.id}</strong><small>{task.id} · {formatTime(task.updated_at)}</small></div><StatusBadge tone={toneForTask(task)}>{task.status}</StatusBadge></header><p>{task.goal}</p>{task.blocker && <div className="ops-blocker"><ShieldAlert size={15} />{task.blocker}</div>}<footer><span>{task.phase || 'no phase'}</span><span>review: {task.review_status}</span><span>{task.condition_count} 条件 / {task.step_count} 步骤</span>{task.template_id && <span>{task.template_id}@{task.template_version}</span>}{task.cleanable && <strong>可清理</strong>}</footer></article>;
}

function OpsShell({ title, subtitle, loading, error, onReload, children }: { title: string; subtitle: string; loading: boolean; error?: string; onReload: () => void; children: ReactNode }) {
  return <section className="ops-page"><div className="section-heading ops-heading"><div><h2>{title}</h2><p>{subtitle}</p></div><button className="nx-button is-secondary" onClick={onReload} disabled={loading}><RefreshCw size={15} className={loading ? 'nx-spin' : ''} />刷新</button></div>{error && <div className="nx-alert is-error">{error}</div>}{children}</section>;
}

function Metric({ label, value }: { label: string; value: string; wide?: boolean }) { return <article><strong>{value}</strong><span>{label}</span></article>; }
function StatusBadge({ tone, children }: { tone: Tone; children: ReactNode }) { return <span className={`status-badge tone-${tone}`}><span />{children}</span>; }
function EmptyOps({ text }: { text: string }) { return <p className="empty-mini">{text}</p>; }
function PathPanel({ paths }: { paths: OpsPaths }) { return <article className="ops-paths"><h3>路径</h3>{Object.entries(paths).map(([key, value]) => <div key={key}><span>{key}</span><code>{value || '未配置'}</code></div>)}</article>; }
function copyText(value: string) { void navigator.clipboard?.writeText(value || ''); }
