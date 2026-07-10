import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { CheckCircle2, Layers, RefreshCw, Search, ShieldAlert, Trash2 } from 'lucide-react';
import { ApiError, api } from '../../api/client';
import { formatTime, timeZoneLabel } from '../../lib/time';

type Tone = 'ok' | 'warn' | 'danger' | 'muted';
type TaskStatus = 'all' | 'active' | 'completed' | 'blocked';

type OpsTask = { id: string; title: string; goal: string; status: string; phase: string; review_status: string; blocker?: string; updated_at: string; created_at: string; template_id?: string; template_version?: string; condition_count: number; step_count: number; attempt_count: number; event_count: number; cleanable: boolean; file_name: string };
type OpsTaskDetail = OpsTask & { path?: string; content?: string; json?: Record<string, unknown>; conditions?: unknown[]; steps?: unknown[]; attempts?: unknown[]; events?: unknown[]; final_review?: Record<string, unknown> };
type OpsSkill = { id: string; title: string; source: string; path: string; description?: string; updated_at: string; file_count: number; status: string; active_version?: string; versions?: string[]; channels?: Record<string, string>; runtime_state_path?: string; doc_root?: string };
type OpsSkillFile = { path: string; kind: string; size_bytes: number; updated_at: string };
type OpsSkillDetail = OpsSkill & { root?: string; skill_doc?: string; files?: OpsSkillFile[]; runtime_state?: Record<string, unknown> };
type OpsPaths = { agentdock?: string; workspace?: string; workflows?: string };
type OpsTool = { name: string; category: string; status: string; description: string; source?: string; version?: string; metadata?: Record<string, unknown> };
type TaskCounts = { active: number; blocked: number; completed: number; cleanable: number };
type TaskListResponse = { ok: boolean; items: OpsTask[]; count: number; total: number; root?: string; source?: string };
type TaskDetailResponse = { ok: boolean; task: OpsTaskDetail; source?: string };
type CleanupResponse = { ok: boolean; dry_run: boolean; changed: OpsTask[]; count: number };
type SkillsResponse = { ok: boolean; items: OpsSkill[]; count: number; root?: string; source?: string };
type SkillDetailResponse = { ok: boolean; skill: OpsSkillDetail; source?: string };
type CapabilitiesResponse = { ok: boolean; tools: OpsTool[]; counts: Record<string, unknown>; paths: OpsPaths; source?: string; runtime?: Record<string, unknown> };

const emptyTasks: TaskListResponse = { ok: false, items: [], count: 0, total: 0, root: '' };
function formatBytes(value?: number): string { if (value === undefined) return '暂无'; const units = ['B', 'KiB', 'MiB', 'GiB']; let size = value; let unit = 0; while (size >= 1024 && unit < units.length - 1) { size /= 1024; unit += 1; } return `${size.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`; }
function apiMessage(error: unknown): string { if (error instanceof ApiError) return `${error.code || error.status}：${error.message}`; return error instanceof Error ? error.message : '请求失败'; }
function toneForTask(task: Pick<OpsTask, 'status' | 'phase' | 'review_status'>): Tone { if (task.status === 'completed') return 'ok'; if (task.status === 'blocked') return 'danger'; if (task.phase === 'closeout' && task.review_status === 'pass') return 'warn'; if (task.status === 'active') return 'warn'; return 'muted'; }
function countTasks(tasks: OpsTask[]): TaskCounts { return { active: tasks.filter((item) => item.status === 'active').length, blocked: tasks.filter((item) => item.status === 'blocked').length, completed: tasks.filter((item) => item.status === 'completed').length, cleanable: tasks.filter((item) => item.cleanable).length }; }

function taskDisplayTitle(task?: Pick<OpsTask, 'title' | 'goal' | 'id'>): string {
  const title = task?.title?.trim();
  if (title) return title;
  const goal = task?.goal?.trim();
  if (goal) return goal.split(/[。.!?！？\n]/)[0] || goal;
  return task?.id || '未命名任务';
}
function taskListMeta(task: OpsTask): string {
  return [task.phase || 'no phase', task.template_id || '', formatTime(task.updated_at)].filter(Boolean).join(' · ');
}
function toneForStatus(status?: string): Tone { if (!status) return 'muted'; if (['ok', 'healthy', 'available', 'installed', 'active', 'success', 'completed'].includes(status)) return 'ok'; if (['failed', 'blocked', 'offline', 'unknown'].includes(status)) return 'danger'; if (['pending', 'draft', 'running', 'degraded'].includes(status)) return 'warn'; return 'muted'; }
function shortHash(value?: string): string { return value ? value.slice(0, 8) : 'unknown'; }

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
  const path = `/v1/runtime/tasks?status=${status}&limit=300${query.trim() ? `&q=${encodeURIComponent(query.trim())}` : ''}`;
  const resource = useOpsResource<TaskListResponse>(path, emptyTasks, refreshToken);
  const allResource = useOpsResource<TaskListResponse>('/v1/runtime/tasks?status=all&limit=300', emptyTasks, refreshToken);
  const tasks = resource.data.items;
  const [selectedId, setSelectedId] = useState('');
  const selected = tasks.find((item) => item.id === selectedId) || tasks[0];
  const detail = useOptionalOpsResource<TaskDetailResponse>(selected?.file_name ? `/v1/runtime/tasks/${encodeURIComponent(selected.file_name)}` : '', { ok: false, task: selected as OpsTaskDetail }, refreshToken);
  const visibleStats = useMemo(() => countTasks(tasks), [tasks]);
  const totalStats = useMemo(() => countTasks(allResource.data.items), [allResource.data.items]);
  const totalCount = allResource.data.total || allResource.data.count || resource.data.total || tasks.length;
  const healthStats = totalCount ? totalStats : visibleStats;
  const healthTone: Tone = healthStats.blocked ? 'danger' : healthStats.cleanable ? 'warn' : 'ok';
  const healthText = healthStats.blocked ? `${healthStats.blocked} blocked` : healthStats.cleanable ? `${healthStats.cleanable} cleanable` : 'healthy';

  return <OpsShell title="任务中心" subtitle={`通过 AgentDock Runtime API 展示任务状态、阶段、review、条件、步骤和事件。时间按 ${timeZoneLabel()} 显示。`} loading={resource.loading || allResource.loading} error={resource.error || allResource.error} onReload={() => { resource.reload(); allResource.reload(); }}>
    <section className="ops-command-hero"><div><span>AGENTDOCK TASKS</span><h3>{totalCount} 条任务记录</h3><p>{resource.data.source || resource.data.root || 'AgentDock Runtime API'} · 当前筛选 {resource.data.count} 条</p></div><StatusBadge tone={healthTone}>{healthText}</StatusBadge></section>
    <section className="ops-metrics is-dashboard"><Metric label="Active" value={String(totalStats.active)} tone={totalStats.active ? 'warn' : 'muted'} /><Metric label="Blocked" value={String(totalStats.blocked)} tone={totalStats.blocked ? 'danger' : 'muted'} /><Metric label="Completed" value={String(totalStats.completed)} tone="ok" /><Metric label="可清理" value={String(totalStats.cleanable)} tone={totalStats.cleanable ? 'warn' : 'muted'} /></section>
    <div className="ops-toolbar is-console"><div className="ops-segmented">{(['active', 'blocked', 'completed', 'all'] as TaskStatus[]).map((item) => <button type="button" key={item} className={status === item ? 'is-active' : ''} onClick={() => setStatus(item)}>{item}</button>)}</div><label className="ops-search"><Search size={15} /><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索标题、目标、模板、阻塞原因" /></label><span className="ops-count">当前 {resource.data.count} / 全部 {totalCount}</span></div>
    <section className="ops-master-detail"><div className="ops-task-rail">{tasks.length === 0 ? <EmptyOps text="没有匹配任务。" /> : tasks.map((task) => <button type="button" key={task.id} className={`ops-task-line ${selected?.id === task.id ? 'is-selected' : ''}`} onClick={() => setSelectedId(task.id)}><StatusDot tone={toneForTask(task)} /><span><strong>{taskDisplayTitle(task)}</strong><small>{taskListMeta(task)}</small></span>{task.cleanable && <em>清理</em>}</button>)}</div><TaskDetail task={selected} detail={detail.data.task} loading={detail.loading} error={detail.error} /></section>
  </OpsShell>;
}

function TaskCleanupPage({ refreshToken }: { refreshToken: number }) {
  const resource = useOpsResource<TaskListResponse>('/v1/runtime/tasks?status=active&limit=500', emptyTasks, refreshToken);
  const candidates = resource.data.items.filter((item) => item.cleanable);
  return <OpsShell title="任务清理" subtitle="AgentDock Runtime API 当前只开放只读接口，Nexus 不再直接修改 AgentDock Runtime 状态。" loading={resource.loading} error={resource.error} onReload={resource.reload}>
    <section className="ops-cleanup-hero is-large"><div><span>READ ONLY</span><strong>{candidates.length}</strong><p>这些任务已经 final_review pass，但写入动作需要 AgentDock 暴露受控写接口后再启用。</p></div><div className="ops-actions"><button type="button" className="nx-button is-secondary" disabled title="Runtime 写接口未启用"><CheckCircle2 size={15} />预览已禁用</button><button type="button" className="nx-button is-danger" disabled title="Runtime 写接口未启用"><Trash2 size={15} />清理已禁用</button></div></section>
    <div className="nx-alert is-warning">任务清理已切换为只读模式：Nexus 不再直接改 AgentDock 内部文件，避免绕过 Runtime 状态机。</div>
    <section className="ops-grid cards">{candidates.length === 0 ? <EmptyOps text="当前没有可清理任务。" /> : candidates.map((task) => <TaskCard key={task.id} task={task} />)}</section>
  </OpsShell>;
}

export function SkillsPage({ refreshToken }: { refreshToken: number }) {
  const resource = useOpsResource<SkillsResponse>('/v1/runtime/skills', { ok: false, items: [], count: 0, root: '' }, refreshToken);
  const runtime = resource.data.items.filter((item) => item.source === 'agentdock-api' || item.source === 'runtime').length;
  const [selectedKey, setSelectedKey] = useState('');
  const selected = resource.data.items.find((item) => `${item.source}:${item.id}` === selectedKey) || resource.data.items[0];
  const detail = useOptionalOpsResource<SkillDetailResponse>(selected ? `/v1/runtime/skills/${encodeURIComponent(selected.source)}/${encodeURIComponent(selected.id)}` : '', { ok: false, skill: selected as OpsSkillDetail }, refreshToken);
  return <OpsShell title="Skill" subtitle="通过 AgentDock Runtime API 展示已安装 Skill、版本、channel、manifest 和 selection。" loading={resource.loading} error={resource.error} onReload={resource.reload}>
    <section className="ops-command-hero is-soft"><div><span>SKILL RUNTIME</span><h3>{resource.data.count} 个 Skill</h3><p>{runtime} 个 Runtime Skill · {resource.data.source || resource.data.root || 'AgentDock Runtime API'}</p></div><StatusBadge tone={resource.data.count ? 'ok' : 'warn'}>{resource.data.count ? 'installed' : 'empty'}</StatusBadge></section>
    <section className="ops-master-detail skills-layout"><div className="ops-task-rail ops-skill-rail">{resource.data.items.length === 0 ? <EmptyOps text="没有读取到 Skill。" /> : resource.data.items.map((skill) => <button type="button" key={`${skill.source}:${skill.id}`} className={`ops-task-line ${selected?.source === skill.source && selected?.id === skill.id ? 'is-selected' : ''}`} onClick={() => setSelectedKey(`${skill.source}:${skill.id}`)}><span className="ops-card-icon"><Layers size={16} /></span><span><strong>{skill.title || skill.id}</strong><small>{skill.source} · {skill.active_version || 'no version'} · {skill.file_count} files · {formatTime(skill.updated_at)}</small></span><StatusBadge tone={toneForStatus(skill.status)}>{skill.status}</StatusBadge></button>)}</div><SkillDetail skill={selected} detail={detail.data.skill} loading={detail.loading} error={detail.error} /></section>
  </OpsShell>;
}

export function CapabilitiesPage({ refreshToken }: { refreshToken: number }) {
  const resource = useOpsResource<CapabilitiesResponse>('/v1/runtime/capabilities', { ok: false, tools: [], counts: {}, paths: {} }, refreshToken);
  const counts = resource.data.counts;
  const workflowCounts = (counts.workflows && typeof counts.workflows === 'object' ? counts.workflows : {}) as Record<string, unknown>;
  const [selectedKey, setSelectedKey] = useState('');
  const groups = resource.data.tools.reduce<Record<string, OpsTool[]>>((acc, tool) => { const key = tool.category || 'other'; acc[key] = [...(acc[key] || []), tool]; return acc; }, {});
  const selected = resource.data.tools.find((tool) => toolKey(tool) === selectedKey) || resource.data.tools[0];
  const availableTools = resource.data.tools.filter((tool) => tool.status === 'available').length;
  const workflowTotal = Number(workflowCounts.published || 0) + Number(workflowCounts.drafts || 0) + Number(workflowCounts.retired || 0);
  return <OpsShell title="Capability" subtitle="把当前可用工具、任务、Skill、模板和路径整理成能力矩阵，并可查看选中能力的来源信息。" loading={resource.loading} error={resource.error} onReload={resource.reload}>
    <section className="cap-hero"><div><span>CAPABILITY MATRIX</span><h3>{availableTools} / {resource.data.tools.length} 个工具可用</h3><p>能力页用于判断当前 Nexus 从 AgentDock Runtime API 看见什么、能操作什么。</p></div><StatusBadge tone={availableTools ? 'ok' : 'warn'}>{availableTools ? 'available' : 'empty'}</StatusBadge></section>
    <section className="cap-summary-grid">
      <CapabilitySummary title="Runtime 任务" value={String(counts.tasks ?? 0)} detail="持久化任务记录" tone="warn" />
      <CapabilitySummary title="Skill Runtime" value={String(counts.skills ?? 0)} detail="本机可见 Skill" tone="ok" />
      <CapabilitySummary title="Workflow 模板" value={String(workflowTotal || workflowCounts.published || 0)} detail={`${workflowCounts.published ?? 0} published · ${workflowCounts.drafts ?? 0} drafts`} tone="muted" />
      <CapabilitySummary title="工具能力" value={String(resource.data.tools.length)} detail={`${availableTools} available`} tone={availableTools ? 'ok' : 'warn'} />
    </section>
    <section className="cap-layout">
      <article className="cap-tool-panel"><header><div><h3>工具分组</h3><p>选中一项后右侧展示用途、状态、统计和路径。</p></div><StatusBadge tone="muted">{Object.keys(groups).length} groups</StatusBadge></header>{Object.entries(groups).length === 0 ? <EmptyOps text="没有可展示工具。" /> : Object.entries(groups).map(([category, tools]) => <div className="cap-group" key={category}><div className="cap-group-head"><strong>{category}</strong><span>{tools.length} tools</span></div>{tools.map((tool) => <button type="button" className={`cap-tool-row ${selected && toolKey(selected) === toolKey(tool) ? 'is-selected' : ''}`} key={toolKey(tool)} onClick={() => setSelectedKey(toolKey(tool))}><StatusDot tone={tool.status === 'available' ? 'ok' : 'warn'} /><div><strong>{tool.name}</strong><small>{tool.source || 'unknown'} · {tool.description}</small></div><em>{tool.status}</em></button>)}</div>)}</article>
      <aside className="cap-side"><CapabilityDetail tool={selected} counts={counts} paths={resource.data.paths} workflowCounts={workflowCounts} /><PathPanel paths={resource.data.paths} /></aside>
    </section>
  </OpsShell>;
}

function TaskDetail({ task, detail, loading, error }: { task?: OpsTask; detail?: OpsTaskDetail; loading: boolean; error?: string }) {
  if (!task) return <article className="ops-detail-empty"><EmptyOps text="请选择一个任务。" /></article>;
  const full = detail?.id ? detail : task;
  const conditionItems = readableItems(detail?.conditions, '条件');
  const stepItems = readableItems(detail?.steps, '步骤');
  const eventItems = readableItems(detail?.events, '事件');
  return <article className="ops-task-detail"><header><div><span>任务详情</span><h3>{taskDisplayTitle(full)}</h3><p>{full.goal || '暂无目标描述。'}</p></div><StatusBadge tone={toneForTask(full)}>{full.status}</StatusBadge></header>{loading && <div className="nx-alert is-info">正在读取任务详情…</div>}{error && <div className="nx-alert is-error">{error}</div>}{full.blocker && <div className="ops-blocker"><ShieldAlert size={15} />{full.blocker}</div>}<div className="ops-detail-grid"><Info label="阶段" value={full.phase || 'none'} /><Info label="Review" value={full.review_status || 'not_started'} /><Info label="条件" value={String(full.condition_count)} /><Info label="步骤" value={String(full.step_count)} /><Info label="事件" value={String(full.event_count)} /><Info label="更新" value={formatTime(full.updated_at)} /></div><section className="ops-detail-section"><h4>Runtime 任务记录</h4><div className="ops-key-values"><Info label="ID" value={full.id} /><Info label="记录 ID" value={full.file_name} /><Info label="来源" value={detail?.path || 'AgentDock Runtime API'} /><Info label="创建" value={formatTime(full.created_at)} />{full.template_id && <Info label="模板" value={`${full.template_id}@${full.template_version || 'unknown'}`} />}</div></section><ReviewSummary review={detail?.final_review} /><ReadableList title="完成条件" items={conditionItems} empty="没有条件详情。" /><ReadableList title="执行步骤" items={stepItems} empty="没有步骤详情。" /><ReadableList title="最近事件" items={eventItems.slice(0, 8)} empty="没有事件记录。" /><footer><code>{full.id}</code>{full.template_id && <code>{full.template_id}@{full.template_version}</code>}{full.cleanable && <StatusBadge tone="warn">可清理</StatusBadge>}</footer></article>;
}

function SkillDetail({ skill, detail, loading, error }: { skill?: OpsSkill; detail?: OpsSkillDetail; loading: boolean; error?: string }) {
  if (!skill) return <article className="ops-detail-empty"><EmptyOps text="请选择一个 Skill。" /></article>;
  const full = detail?.id ? detail : skill;
  const files = detail?.files || [];
  const raw = detail?.runtime_state;
  const manifest = asRecord(raw?.manifest);
  const selection = asRecord(raw?.selection);
  return <article className="ops-task-detail ops-skill-detail"><header><div><span>Runtime Skill</span><h3>{full.title || full.id}</h3><p>{full.description || '来自 AgentDock Runtime API。'}</p></div><StatusBadge tone={toneForStatus(full.status)}>{full.status}</StatusBadge></header>{loading && <div className="nx-alert is-info">正在读取 Skill Runtime 详情…</div>}{error && <div className="nx-alert is-error">{error}</div>}<div className="ops-detail-grid"><Info label="ID" value={full.id} /><Info label="来源" value={full.source || 'agentdock-api'} /><Info label="当前版本" value={full.active_version || 'unknown'} /><Info label="版本数" value={String((full.versions || []).length)} /><Info label="更新" value={formatTime(full.updated_at)} /><Info label="逻辑路径" value={full.path || 'agentdock-api'} /></div><section className="ops-detail-section"><h4>Runtime Selection</h4><div className="ops-key-values"><Info label="Active" value={full.active_version || pickText(selection || {}, ['active_version']) || 'unknown'} /><Info label="版本历史" value={(full.versions || []).slice(0, 6).join(' → ') || '暂无'} /><Info label="Binding" value={String(raw?.binding_configured ?? 'unknown')} /><Info label="Root" value={detail?.root || 'agentdock-runtime-api'} /></div><ChannelChips channels={full.channels} /></section>{manifest && <section className="ops-detail-section"><h4>Manifest 摘要</h4><div className="ops-key-values"><Info label="metadata" value={Object.keys(asRecord(manifest.metadata) || {}).join(', ') || '无'} /><Info label="operations" value={String((manifest.operations as unknown[] | undefined)?.length || 0)} /><Info label="permissions" value={Object.keys(asRecord(manifest.permissions) || {}).join(', ') || '无'} /><Info label="env" value={String((manifest.env as unknown[] | undefined)?.length || 0)} /></div></section>}<section className="ops-detail-section"><h4>文件清单</h4>{files.length === 0 ? <EmptyOps text="Runtime API 当前不返回安装包文件清单。" /> : <div className="ops-file-list">{files.map((file) => <div key={file.path} className="ops-file-row"><span><strong>{file.path}</strong><small>{file.kind} · {formatTime(file.updated_at)}</small></span><em>{formatBytes(file.size_bytes)}</em></div>)}</div>}</section>{raw && <RawJsonPanel title="Runtime 原始响应" value={raw} />}</article>;
}

function CapabilityDetail({ tool, counts, paths, workflowCounts }: { tool?: OpsTool; counts: Record<string, unknown>; paths: OpsPaths; workflowCounts: Record<string, unknown> }) {
  if (!tool) return <article className="cap-mini-panel"><h3>能力详情</h3><EmptyOps text="请选择一个工具能力。" /></article>;
  return <article className="cap-mini-panel cap-detail-panel"><h3>{tool.name}</h3><p>{tool.description}</p><div className="ops-key-values is-compact"><Info label="分类" value={tool.category || 'other'} /><Info label="状态" value={tool.status} /><Info label="来源" value={tool.source || 'unknown'} /><Info label="版本" value={tool.version || '—'} /><Info label="任务" value={String(counts.tasks ?? 0)} /><Info label="Skill" value={String(counts.skills ?? 0)} /><Info label="Published 模板" value={String(workflowCounts.published ?? 0)} /></div><MetadataChips metadata={tool.metadata} /><section className="ops-detail-section"><h4>相关路径</h4><div className="ops-key-values is-compact"><Info label="AgentDock" value={paths.agentdock || '未配置'} /><Info label="Workspace" value={paths.workspace || '未配置'} /><Info label="Workflows" value={paths.workflows || '未配置'} /></div></section></article>;
}

type ReadableItem = { title: string; meta?: string; detail?: string };
function readableItems(values?: unknown[], fallback = '条目'): ReadableItem[] {
  if (!Array.isArray(values)) return [];
  return values.map((value, index) => {
    const record = asRecord(value);
    if (!record) return { title: `${fallback} ${index + 1}`, detail: String(value) };
    const title = pickText(record, ['text', 'title', 'name', 'id', 'type', 'summary']) || `${fallback} ${index + 1}`;
    const meta = [pickText(record, ['status', 'phase', 'outcome']), pickText(record, ['updated_at', 'created_at', 'occurred_at', 'time'])].filter(Boolean).join(' · ');
    const detail = pickText(record, ['summary', 'evidence', 'message', 'diagnosis', 'result', 'reason', 'description']);
    return { title, meta, detail };
  });
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
function ReadableList({ title, items, empty }: { title: string; items: ReadableItem[]; empty: string }) {
  return <section className="ops-detail-section"><h4>{title}</h4>{items.length === 0 ? <EmptyOps text={empty} /> : <div className="ops-readable-list">{items.map((item, index) => <div className="ops-readable-row" key={`${item.title}:${index}`}><strong>{item.title}</strong>{item.meta && <small>{item.meta}</small>}{item.detail && item.detail !== item.title && <p>{item.detail}</p>}</div>)}</div>}</section>;
}
function ReviewSummary({ review }: { review?: Record<string, unknown> }) {
  if (!review || Object.keys(review).length === 0) return null;
  const status = pickText(review, ['status', 'review_status']) || 'unknown';
  return <section className="ops-detail-section"><h4>Final Review</h4><div className="ops-key-values"><Info label="状态" value={status} /><Info label="摘要" value={pickText(review, ['summary']) || '暂无'} /><Info label="已验证" value={String(review.verified_fact_count ?? 0)} /><Info label="风险" value={String(review.open_risk_count ?? 0)} /></div></section>;
}
function ChannelChips({ channels }: { channels?: Record<string, string> }) {
  const entries = Object.entries(channels || {});
  if (entries.length === 0) return null;
  return <div className="ops-chip-row">{entries.map(([name, version]) => <span key={name}>{name}: {version}</span>)}</div>;
}
function MetadataChips({ metadata }: { metadata?: Record<string, unknown> }) {
  const entries = Object.entries(metadata || {}).filter(([, value]) => value !== undefined && value !== null && value !== '' && !(Array.isArray(value) && value.length === 0));
  if (entries.length === 0) return null;
  return <section className="ops-detail-section"><h4>补充信息</h4><div className="ops-chip-row">{entries.slice(0, 12).map(([key, value]) => <span key={key}>{key}: {Array.isArray(value) ? value.join(', ') : typeof value === 'object' ? '已配置' : String(value)}</span>)}</div></section>;
}

function toolKey(tool: Pick<OpsTool, 'name' | 'category' | 'source'>): string {
  return [tool.source || 'unknown', tool.category || 'other', tool.name].join(':');
}
function RawJsonPanel({ title, value }: { title: string; value: unknown }) {
  return <details className="ops-json-panel"><summary>{title}</summary><pre>{JSON.stringify(value, null, 2)}</pre></details>;
}

function TaskCard({ task }: { task: OpsTask }) { return <article className="ops-task-card"><header><div><strong>{taskDisplayTitle(task)}</strong><small>{task.id} · {formatTime(task.updated_at)}</small></div><StatusBadge tone={toneForTask(task)}>{task.status}</StatusBadge></header><p>{task.goal}</p>{task.blocker && <div className="ops-blocker"><ShieldAlert size={15} />{task.blocker}</div>}<footer><span>{task.phase || 'no phase'}</span><span>review: {task.review_status}</span><span>{task.condition_count} 条件 / {task.step_count} 步骤</span>{task.template_id && <span>{task.template_id}@{task.template_version}</span>}{task.cleanable && <strong>可清理</strong>}</footer></article>; }
function OpsShell({ title, subtitle, loading, error, onReload, children }: { title: string; subtitle: string; loading: boolean; error?: string; onReload: () => void; children: ReactNode }) { return <section className="ops-page ops-console"><div className="section-heading ops-heading"><div><h2>{title}</h2><p>{subtitle}</p></div><button type="button" className="nx-button is-secondary" onClick={onReload} disabled={loading}><RefreshCw size={15} className={loading ? 'nx-spin' : ''} />刷新</button></div>{error && <div className="nx-alert is-error">{error}</div>}{children}</section>; }
function Metric({ label, value, tone = 'muted' }: { label: string; value: string; tone?: Tone }) { return <article><span className={`metric-icon tone-${tone}`}>{label.slice(0, 1)}</span><strong>{value}</strong><small>{label}</small></article>; }
function CapabilitySummary({ title, value, detail, tone }: { title: string; value: string; detail: string; tone: Tone }) { return <article className="cap-summary-card"><header><span className={`metric-icon tone-${tone}`}>{title.slice(0, 1)}</span>{title}</header><strong>{value}</strong><p>{detail}</p></article>; }
function Info({ label, value }: { label: string; value: string }) { return <div><dt>{label}</dt><dd>{value}</dd></div>; }
function StatusBadge({ tone, children }: { tone: Tone; children: ReactNode }) { return <span className={`status-badge tone-${tone}`}><span />{children}</span>; }
function StatusDot({ tone }: { tone: Tone }) { return <i className={`ops-dot tone-${tone}`} />; }
function EmptyOps({ text }: { text: string }) { return <p className="empty-mini">{text}</p>; }
function PathPanel({ paths }: { paths: OpsPaths }) { return <article className="ops-paths is-rich"><h3>路径</h3>{Object.entries(paths).map(([key, value]) => <div key={key}><span>{key}</span><code>{value || '未配置'}</code></div>)}</article>; }
