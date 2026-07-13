import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { CheckCircle2, Circle, FileText, Layers, LoaderCircle, Search, ShieldAlert, Trash2 } from 'lucide-react';
import { ApiError, api } from '../../api/client';
import { formatTime } from '../../lib/time';
import Dialog from '../Dialog';
import MobileDrilldownBar from '../MobileDrilldownBar';

type Tone = 'ok' | 'warn' | 'danger' | 'muted';
type TaskStatus = 'all' | 'active' | 'completed' | 'blocked';

const taskStatusLabels: Record<TaskStatus, string> = { all: '全部', active: '进行中', completed: '已完成', blocked: '阻塞' };
const runtimeTaskListLimit = 200;
function taskStatusLabel(status?: string): string { return taskStatusLabels[status as TaskStatus] || status || '未知'; }

type TaskStep = { id: string; title: string; status: string };
type OpsTask = { id: string; title: string; goal: string; status: string; summary?: string; blocker?: string; current_step?: TaskStep; completed_step_count: number; step_count: number; updated_at: string; file_name: string };
type OpsTaskDetail = OpsTask & { steps?: unknown[] };
type OpsSkill = { id: string; title: string; source: string; path: string; description?: string; updated_at: string; file_count: number; status: string; active_version?: string; versions?: string[]; channels?: Record<string, string>; runtime_state_path?: string; doc_root?: string };
type OpsSkillFile = { path: string; kind: string; size_bytes: number; updated_at: string };
type OpsSkillDetail = OpsSkill & { root?: string; skill_doc?: string; files?: OpsSkillFile[]; runtime_state?: Record<string, unknown> };
type TaskCounts = { active: number; blocked: number; completed: number };
type TaskListResponse = { ok: boolean; items: OpsTask[]; count: number; total: number; root?: string; source?: string };
type TaskDetailResponse = { ok: boolean; task: OpsTaskDetail; source?: string };
type DeleteTaskResponse = { ok: boolean; task_id: string; deleted_task?: OpsTask; source?: string };
type SkillsResponse = { ok: boolean; items: OpsSkill[]; count: number; root?: string; source?: string };
type SkillDetailResponse = { ok: boolean; skill: OpsSkillDetail; source?: string };
type SkillFileContent = OpsSkillFile & { content: string; truncated: boolean };
type SkillFileResponse = { ok: boolean; file?: SkillFileContent };

const emptyTasks: TaskListResponse = { ok: false, items: [], count: 0, total: 0, root: '' };
function formatBytes(value?: number): string { if (value === undefined) return '暂无'; const units = ['B', 'KiB', 'MiB', 'GiB']; let size = value; let unit = 0; while (size >= 1024 && unit < units.length - 1) { size /= 1024; unit += 1; } return `${size.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`; }
function apiMessage(error: unknown): string { if (error instanceof ApiError) return `${error.code || error.status}：${error.message}`; return error instanceof Error ? error.message : '请求失败'; }
function toneForTask(task: Pick<OpsTask, 'status'>): Tone { if (task.status === 'completed') return 'ok'; if (task.status === 'blocked') return 'danger'; if (task.status === 'active') return 'warn'; return 'muted'; }
function countTasks(tasks: OpsTask[]): TaskCounts { return { active: tasks.filter((item) => item.status === 'active').length, blocked: tasks.filter((item) => item.status === 'blocked').length, completed: tasks.filter((item) => item.status === 'completed').length }; }

function taskDisplayTitle(task?: Pick<OpsTask, 'title' | 'goal' | 'id'>): string {
  const title = task?.title?.trim();
  if (title) return title;
  const goal = task?.goal?.trim();
  if (goal) return goal.split(/[。.!?！？\n]/)[0] || goal;
  return task?.id || '未命名任务';
}

type TaskProgressState = { completed: number; total: number; percent: number; determinate: boolean; label: string };
function taskProgress(task: Pick<OpsTask, 'status' | 'completed_step_count' | 'step_count'>): TaskProgressState {
  const total = Math.max(0, Number(task.step_count) || 0);
  if (total === 0) {
    return { completed: 0, total: 0, percent: 0, determinate: false, label: task.status === 'completed' ? '已完成' : '未拆分步骤' };
  }
  const reported = Math.max(0, Number(task.completed_step_count) || 0);
  const completed = task.status === 'completed' ? total : Math.min(reported, total);
  return { completed, total, percent: Math.round((completed / total) * 100), determinate: true, label: `${completed} / ${total}` };
}

function taskCurrentText(task: OpsTask): string {
  if (task.status === 'blocked' && task.blocker) return `阻塞：${task.blocker}`;
  if (task.current_step?.title) return `当前：${task.current_step.title}`;
  if (task.summary) return task.summary;
  if (task.status === 'completed') return '任务已完成';
  return task.step_count > 0 ? '等待下一步' : '未拆分执行步骤';
}
function toneForStatus(status?: string): Tone { if (!status) return 'muted'; if (['ok', 'healthy', 'available', 'installed', 'active', 'success', 'completed'].includes(status)) return 'ok'; if (['failed', 'blocked', 'offline', 'unknown'].includes(status)) return 'danger'; if (['pending', 'draft', 'running', 'degraded'].includes(status)) return 'warn'; return 'muted'; }

function useOpsResource<T>(path: string, fallback: T, refreshToken: number) {
  const fallbackRef = useRef(fallback);
  fallbackRef.current = fallback;
  const [localToken, setLocalToken] = useState(0);
  const [state, setState] = useState<{ data: T; loading: boolean; error?: string }>({ data: fallback, loading: true });
  useEffect(() => {
    let cancelled = false;
    setState((current) => ({ ...current, loading: true, error: undefined }));
    api<T>(path).then((data) => { if (!cancelled) setState({ data, loading: false }); }).catch((error) => { if (!cancelled) setState({ data: fallbackRef.current, loading: false, error: apiMessage(error) }); });
    return () => { cancelled = true; };
  }, [path, refreshToken, localToken]);
  return { ...state, reload: () => setLocalToken((value) => value + 1) };
}

function useOptionalOpsResource<T>(path: string, fallback: T, refreshToken: number) {
  const fallbackRef = useRef(fallback);
  fallbackRef.current = fallback;
  const [state, setState] = useState<{ data: T; loading: boolean; error?: string }>({ data: fallback, loading: false });
  useEffect(() => {
    if (!path) {
      setState({ data: fallbackRef.current, loading: false });
      return undefined;
    }
    let cancelled = false;
    setState((current) => ({ ...current, loading: true, error: undefined }));
    api<T>(path).then((data) => { if (!cancelled) setState({ data, loading: false }); }).catch((error) => { if (!cancelled) setState({ data: fallbackRef.current, loading: false, error: apiMessage(error) }); });
    return () => { cancelled = true; };
  }, [path, refreshToken]);
  return state;
}

export function TaskCenterPage({ refreshToken }: { refreshToken: number }) {
  const [status, setStatus] = useState<TaskStatus>('active');
  const [query, setQuery] = useState('');
  const [selectedId, setSelectedId] = useState('');
  const [mobileDetailOpen, setMobileDetailOpen] = useState(false);
  const [pendingDelete, setPendingDelete] = useState<OpsTask | null>(null);
  const [deletingId, setDeletingId] = useState('');
  const [deleteError, setDeleteError] = useState('');
  const [notice, setNotice] = useState('');

  const path = `/v1/runtime/tasks?status=${status}&limit=${runtimeTaskListLimit}${query.trim() ? `&q=${encodeURIComponent(query.trim())}` : ''}`;
  const resource = useOpsResource<TaskListResponse>(path, emptyTasks, refreshToken);
  const allResource = useOpsResource<TaskListResponse>(`/v1/runtime/tasks?status=all&limit=${runtimeTaskListLimit}`, emptyTasks, refreshToken);
  const tasks = resource.data.items;
  const selected = tasks.find((item) => item.id === selectedId) || tasks[0];
  const detail = useOptionalOpsResource<TaskDetailResponse>(selected?.file_name ? `/v1/runtime/tasks/${encodeURIComponent(selected.file_name)}` : '', { ok: false, task: selected as OpsTaskDetail }, refreshToken);
  const totalStats = useMemo(() => countTasks(allResource.data.items), [allResource.data.items]);
  const totalCount = allResource.data.total || allResource.data.count || resource.data.total || tasks.length;
  const statusCounts: Record<TaskStatus, number> = { ...totalStats, all: totalCount };

  function reloadTasks() {
    resource.reload();
    allResource.reload();
  }

  async function confirmDelete() {
    if (!pendingDelete || deletingId) return;
    const task = pendingDelete;
    setDeletingId(task.id);
    setDeleteError('');
    try {
      await api<DeleteTaskResponse>(`/v1/runtime/tasks/${encodeURIComponent(task.id)}`, { method: 'DELETE' });
      setPendingDelete(null);
      setSelectedId('');
      setMobileDetailOpen(false);
      setNotice(`任务「${taskDisplayTitle(task)}」已删除。`);
      reloadTasks();
    } catch (error) {
      setDeleteError(apiMessage(error));
    } finally {
      setDeletingId('');
    }
  }

  return <>
    <OpsShell error={resource.error || allResource.error}>
      {notice && <div className="nx-alert is-success" role="status">{notice}<button type="button" onClick={() => setNotice('')}>关闭</button></div>}
      <div className={`ops-toolbar is-console ops-task-toolbar mobile-list-toolbar ${mobileDetailOpen ? 'is-detail-open' : ''}`}><div className="ops-segmented">{(['active', 'blocked', 'completed', 'all'] as TaskStatus[]).map((item) => <button type="button" key={item} className={status === item ? 'is-active' : ''} aria-pressed={status === item} onClick={() => { setStatus(item); setMobileDetailOpen(false); }}><span>{taskStatusLabels[item]}</span><em>{statusCounts[item]}</em></button>)}</div><label className="ops-search"><Search size={15} /><input aria-label="搜索任务" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索任务或当前步骤" /></label><span className="ops-count">显示 {resource.data.count} 条</span></div>
      <section className={`ops-master-detail ops-task-master-detail mobile-drilldown ${mobileDetailOpen ? 'is-detail-open' : 'is-list-open'}`}>
        <div className="ops-task-rail mobile-drilldown-list">
          {tasks.length === 0 ? <EmptyOps text="没有匹配任务。" /> : tasks.map((task) => <button type="button" key={task.id} className={`ops-task-line ${selected?.id === task.id ? 'is-selected' : ''}`} aria-pressed={selected?.id === task.id} onClick={() => { setSelectedId(task.id); setMobileDetailOpen(true); }}><span className="ops-task-line-title"><strong>{taskDisplayTitle(task)}</strong><span className={`ops-task-state tone-${toneForTask(task)}`}>{taskStatusLabel(task.status)}</span></span><TaskProgress task={task} compact /><small>{taskCurrentText(task)}</small></button>)}
        </div>
        <div className="mobile-drilldown-detail">
          {selected && <MobileDrilldownBar label="任务详情" title={taskDisplayTitle(selected)} meta={taskStatusLabel(selected.status)} backLabel="返回任务列表" onBack={() => setMobileDetailOpen(false)} />}
          <TaskDetail task={selected} detail={detail.data.task} loading={detail.loading} error={detail.error} deleting={deletingId === selected?.id} onDelete={(task) => { setDeleteError(''); setPendingDelete(task); }} />
        </div>
      </section>
    </OpsShell>
    {pendingDelete && <Dialog title="删除任务" description="任务记录和步骤将被永久删除，此操作不可恢复。" onClose={() => { if (!deletingId) setPendingDelete(null); }}>
      <div className="ops-delete-dialog">
        <p>确定删除任务「{taskDisplayTitle(pendingDelete)}」？</p>
        <code>{pendingDelete.id}</code>
        {deleteError && <div className="nx-alert is-error" role="alert">{deleteError}</div>}
        <div className="nx-dialog-actions">
          <button type="button" className="nx-button is-secondary" data-dialog-initial-focus onClick={() => setPendingDelete(null)} disabled={Boolean(deletingId)}>取消</button>
          <button type="button" className="nx-button is-danger" aria-busy={Boolean(deletingId)} onClick={() => { void confirmDelete(); }} disabled={Boolean(deletingId)}><Trash2 size={15} />{deletingId ? '正在删除…' : '确认删除'}</button>
        </div>
      </div>
    </Dialog>}
  </>;
}

export function SkillsPage({ refreshToken }: { refreshToken: number }) {
  const resource = useOpsResource<SkillsResponse>('/v1/runtime/skills', { ok: false, items: [], count: 0, root: '' }, refreshToken);
  const [query, setQuery] = useState('');
  const [selectedKey, setSelectedKey] = useState('');
  const [mobileDetailOpen, setMobileDetailOpen] = useState(false);
  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) return resource.data.items;
    return resource.data.items.filter((item) => [item.id, item.title, item.description, item.active_version].filter(Boolean).join(' ').toLowerCase().includes(needle));
  }, [query, resource.data.items]);
  const selected = filtered.find((item) => `${item.source}:${item.id}` === selectedKey) || filtered[0];
  const detail = useOptionalOpsResource<SkillDetailResponse>(selected ? `/v1/runtime/skills/${encodeURIComponent(selected.source)}/${encodeURIComponent(selected.id)}` : '', { ok: false, skill: selected as OpsSkillDetail }, refreshToken);
  return <OpsShell error={resource.error}>
    <div className={`ops-toolbar skill-toolbar mobile-list-toolbar ${mobileDetailOpen ? 'is-detail-open' : ''}`}>
      <label className="ops-search"><Search size={15} /><input aria-label="搜索 Skill" value={query} onChange={(event) => { setQuery(event.target.value); setMobileDetailOpen(false); }} placeholder="搜索名称或说明" /></label>
      <span className="ops-count">{filtered.length} / {resource.data.count}</span>
    </div>
    <section className={`ops-master-detail skills-layout mobile-drilldown ${mobileDetailOpen ? 'is-detail-open' : 'is-list-open'}`}>
      <div className="ops-task-rail ops-skill-rail mobile-drilldown-list">
        {filtered.length === 0 ? <EmptyOps text="没有匹配的 Skill。" /> : filtered.map((skill) => <button type="button" key={`${skill.source}:${skill.id}`} className={`ops-task-line ${selected?.source === skill.source && selected?.id === skill.id ? 'is-selected' : ''}`} aria-pressed={selected?.source === skill.source && selected?.id === skill.id} onClick={() => { setSelectedKey(`${skill.source}:${skill.id}`); setMobileDetailOpen(true); }}><span className="ops-card-icon"><Layers size={16} /></span><span><strong>{skill.title || skill.id}</strong><small>{skill.active_version || '未标记版本'} · {skill.file_count > 0 ? `${skill.file_count} 个文件` : '文件按需读取'}</small></span></button>)}
      </div>
      <div className="mobile-drilldown-detail">
        {selected && <MobileDrilldownBar label="Skill" title={selected.title || selected.id} meta={selected.active_version || selected.status} backLabel="返回 Skill 列表" onBack={() => setMobileDetailOpen(false)} />}
        <SkillDetail skill={selected} detail={detail.data.skill} loading={detail.loading} error={detail.error} refreshToken={refreshToken} />
      </div>
    </section>
  </OpsShell>;
}

function TaskDetail({ task, detail, loading, error, deleting, onDelete }: { task?: OpsTask; detail?: OpsTaskDetail; loading: boolean; error?: string; deleting: boolean; onDelete: (task: OpsTask) => void }) {
  if (!task) return <article className="ops-detail-empty"><EmptyOps text="请选择一个任务。" /></article>;
  const full = detail?.id ? detail : task;
  const steps = taskSteps(detail?.steps);
  const currentStep = full.current_step || steps.find((step) => step.status === 'in_progress') || steps.find((step) => step.status === 'pending');
  const currentTitle = currentStep?.title || (full.status === 'completed' ? '任务已完成' : full.status === 'blocked' ? '任务已阻塞' : '等待下一步');
  return <article className="ops-task-detail ops-task-detail-simple">
    <header>
      <div><span>任务</span><h3>{taskDisplayTitle(full)}</h3>{full.goal && <p>{full.goal}</p>}</div>
      <div className="ops-task-detail-actions">
        <StatusBadge tone={toneForTask(full)}>{taskStatusLabel(full.status)}</StatusBadge>
        <button type="button" className="nx-button is-danger is-small" aria-label={`删除任务 ${taskDisplayTitle(full)}`} onClick={() => onDelete(full)} disabled={deleting}><Trash2 size={15} />{deleting ? '删除中…' : '删除'}</button>
      </div>
    </header>
    {loading && <div className="nx-alert is-info">正在读取任务详情…</div>}
    {error && <div className="nx-alert is-error">{error}</div>}
    {full.blocker && <div className="ops-blocker"><ShieldAlert size={15} />{full.blocker}</div>}
    <TaskProgress task={full} />
    <section className="ops-current-step" aria-label="当前进展">
      <span>{full.status === 'completed' ? '结果' : full.status === 'blocked' ? '当前状态' : '当前步骤'}</span>
      <strong>{currentTitle}</strong>
      {full.summary && full.summary !== currentTitle && <p>{full.summary}</p>}
    </section>
    <TaskStepList steps={steps} status={full.status} />
    <footer className="ops-task-updated">更新于 {formatTime(full.updated_at)}</footer>
  </article>;
}

function SkillDetail({ skill, detail, loading, error, refreshToken }: { skill?: OpsSkill; detail?: OpsSkillDetail; loading: boolean; error?: string; refreshToken: number }) {
  if (!skill) return <article className="ops-detail-empty"><EmptyOps text="请选择一个 Skill。" /></article>;
  return <SkillDetailContent skill={skill} detail={detail} loading={loading} error={error} refreshToken={refreshToken} />;
}

function SkillDetailContent({ skill, detail, loading, error, refreshToken }: { skill: OpsSkill; detail?: OpsSkillDetail; loading: boolean; error?: string; refreshToken: number }) {
  const [selectedPath, setSelectedPath] = useState('');
  const full = detail?.id ? detail : skill;
  const files = detail?.files || [];
  const preferredPath = files.find((file) => file.path.toLowerCase() === 'skill.md')?.path || files[0]?.path || '';
  const activePath = files.some((file) => file.path === selectedPath) ? selectedPath : preferredPath;
  const fileURL = activePath ? `/v1/runtime/skills/${encodeURIComponent(full.source)}/${encodeURIComponent(full.id)}/files/${encodePathSegments(activePath)}` : '';
  const preview = useOptionalOpsResource<SkillFileResponse>(fileURL, { ok: false }, refreshToken);
  const raw = detail?.runtime_state;
  const manifest = asRecord(raw?.manifest);
  const selection = asRecord(raw?.selection);

  return <article className="ops-task-detail ops-skill-detail">
    <header className="ops-skill-head">
      <div><span>Skill</span><h3>{full.title || full.id}</h3><p>{full.description || '暂无用途说明。'}</p></div>
      <StatusBadge tone={toneForStatus(full.status)}>{full.active_version || full.status}</StatusBadge>
    </header>
    {loading && <div className="nx-alert is-info">正在读取 Skill 详情…</div>}
    {error && <div className="nx-alert is-error">{error}</div>}

    <div className="ops-skill-summary" aria-label="Skill 摘要">
      <div><span>当前版本</span><strong>{full.active_version || '未标记'}</strong></div>
      <div><span>文件</span><strong>{files.length}</strong></div>
      <div><span>最近更新</span><strong>{formatTime(full.updated_at)}</strong></div>
    </div>

    <section className="ops-skill-files">
      <div className="ops-skill-file-browser">
        <header><h4>文件</h4><span>{files.length}</span></header>
        {files.length === 0 ? <EmptyOps text="当前安装包没有可展示的文件。" /> : <>
          <div className="ops-mobile-file-tabs" role="tablist" aria-label="选择文件">{files.map((file) => <button type="button" role="tab" aria-selected={activePath === file.path} key={file.path} className={activePath === file.path ? 'is-active' : ''} onClick={() => setSelectedPath(file.path)}><FileText size={13} /><span>{file.path}</span></button>)}</div>
          <div className="ops-file-list">{files.map((file) => <button type="button" key={file.path} className={`ops-file-row ${activePath === file.path ? 'is-active' : ''}`} onClick={() => setSelectedPath(file.path)}><FileText size={15} /><span><strong>{file.path}</strong><small>{fileKindLabel(file.kind)} · {formatBytes(file.size_bytes)}</small></span></button>)}</div>
        </>}
      </div>
      <div className="ops-skill-file-preview">
        {!activePath ? <EmptyOps text="选择文件后在这里查看内容。" /> : preview.loading ? <EmptyOps text="正在读取文件…" /> : preview.error ? <div className="nx-alert is-error">{preview.error}</div> : preview.data.file ? <>
          <header><div><strong>{preview.data.file.path}</strong><span>{fileKindLabel(preview.data.file.kind)} · {formatBytes(preview.data.file.size_bytes)}</span></div>{preview.data.file.truncated && <em>仅显示前 256 KiB</em>}</header>
          <pre>{preview.data.file.content}</pre>
        </> : <EmptyOps text="文件内容不可用。" />}
      </div>
    </section>

    <details className="ops-secondary-details">
      <summary>版本与技术信息</summary>
      <div className="ops-detail-grid">
        <Info label="ID" value={full.id} />
        <Info label="来源" value={full.source || 'agentdock-api'} />
        <Info label="版本历史" value={(full.versions || []).join(' → ') || '暂无'} />
        <Info label="安装目录" value={detail?.root || '不可用'} />
      </div>
      <ChannelChips channels={full.channels} />
      {manifest && <div className="ops-key-values is-compact"><Info label="metadata" value={Object.keys(asRecord(manifest.metadata) || {}).join(', ') || '无'} /><Info label="operations" value={String((manifest.operations as unknown[] | undefined)?.length || 0)} /><Info label="permissions" value={Object.keys(asRecord(manifest.permissions) || {}).join(', ') || '无'} /><Info label="env" value={String((manifest.env as unknown[] | undefined)?.length || 0)} /><Info label="Active" value={full.active_version || pickText(selection || {}, ['active_version']) || 'unknown'} /></div>}
      {raw && <RawJsonPanel title="Runtime 原始响应" value={raw} />}
    </details>
  </article>;
}

function encodePathSegments(value: string): string {
  return value.split('/').map((segment) => encodeURIComponent(segment)).join('/');
}

function fileKindLabel(kind: string): string {
  if (kind === 'doc') return '文档';
  if (kind === 'code') return '代码';
  if (kind === 'config') return '配置';
  if (kind === 'manifest') return '清单';
  return '文件';
}

function taskSteps(values?: unknown[]): TaskStep[] {
  if (!Array.isArray(values)) return [];
  return values.flatMap((value, index) => {
    const record = asRecord(value);
    if (!record) return [];
    const title = pickText(record, ['title', 'name', 'text']) || `步骤 ${index + 1}`;
    return [{ id: pickText(record, ['id']) || `step-${index + 1}`, title, status: pickText(record, ['status']) || 'pending' }];
  });
}

function TaskProgress({ task, compact = false }: { task: OpsTask; compact?: boolean }) {
  const progress = taskProgress(task);
  const progressText = progress.determinate ? `${progress.label} · ${progress.percent}%` : progress.label;
  return <div className={`ops-task-progress ${compact ? 'is-compact' : ''}`}>
    {!compact && <div className="ops-task-progress-head"><strong>进度</strong><span>{progressText}</span></div>}
    <div className={`ops-progress-track tone-${toneForTask(task)} ${progress.determinate ? '' : 'is-undetermined'}`} role="progressbar" aria-label="任务进度" aria-valuemin={0} aria-valuemax={100} aria-valuenow={progress.determinate ? progress.percent : undefined} aria-valuetext={progressText}>
      <i style={{ width: `${progress.percent}%` }} />
    </div>
    {compact && <span className="ops-progress-count">{progressText}</span>}
  </div>;
}

function taskStepStatusLabel(status: string): string {
  if (status === 'completed') return '已完成';
  if (status === 'in_progress') return '进行中';
  return '待处理';
}

function TaskStepList({ steps, status }: { steps: TaskStep[]; status: string }) {
  return <section className="ops-task-steps">
    <header><h4>步骤</h4><span>{steps.length} 项</span></header>
    {steps.length === 0 ? <p className="ops-no-steps">该任务未拆分步骤，只能显示任务状态。</p> : <div className="ops-task-step-list">{steps.map((step) => {
      const stepStatus = status === 'completed' ? 'completed' : step.status;
      return <div className={`ops-task-step is-${stepStatus}`} key={step.id}>
        <span className="ops-task-step-icon">{stepStatus === 'completed' ? <CheckCircle2 size={17} /> : stepStatus === 'in_progress' ? <LoaderCircle size={17} /> : <Circle size={17} />}</span>
        <strong>{step.title}</strong>
        <small>{taskStepStatusLabel(stepStatus)}</small>
      </div>;
    })}</div>}
  </section>;
}

function asRecord(value: unknown): Record<string, unknown> | null { return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : null; }
function pickText(record: Record<string, unknown>, keys: string[]): string {
  for (const key of keys) {
    const value = record[key];
    if (typeof value === 'string' && value.trim()) return value;
    if (typeof value === 'number' || typeof value === 'boolean') return String(value);
  }
  return '';
}
function ChannelChips({ channels }: { channels?: Record<string, string> }) {
  const entries = Object.entries(channels || {});
  if (entries.length === 0) return null;
  return <div className="ops-chip-row">{entries.map(([name, version]) => <span key={name}>{name}: {version}</span>)}</div>;
}
function RawJsonPanel({ title, value }: { title: string; value: unknown }) {
  return <details className="ops-json-panel"><summary>{title}</summary><pre>{JSON.stringify(value, null, 2)}</pre></details>;
}

function OpsShell({ error, children }: { error?: string; children: ReactNode }) { return <section className="ops-page ops-console">{error && <div className="nx-alert is-error">{error}</div>}{children}</section>; }
function Info({ label, value }: { label: string; value: string }) { return <div><dt>{label}</dt><dd>{value}</dd></div>; }
function StatusBadge({ tone, children }: { tone: Tone; children: ReactNode }) { return <span className={`status-badge tone-${tone}`}><span />{children}</span>; }
function EmptyOps({ text }: { text: string }) { return <p className="empty-mini">{text}</p>; }
