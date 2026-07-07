import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { Check, Copy, FileJson, RefreshCw, Search } from 'lucide-react';
import { api } from '../../api/client';
import { formatTime, timeZoneLabel } from '../../lib/time';

type WorkflowLocation = 'drafts' | 'published' | 'retired';
type Tone = 'ok' | 'warn' | 'danger' | 'muted';

type WorkflowTemplateSummary = {
  id: string;
  version: string;
  title: string;
  description?: string;
  status: string;
  location: WorkflowLocation;
  file_name: string;
  path: string;
  size_bytes: number;
  updated_at: string;
  step_count: number;
  keywords?: string[];
  current?: boolean;
  version_count?: number;
  active_count?: number;
  draft_count?: number;
  retired_count?: number;
  has_conflict?: boolean;
};

type WorkflowTemplateDetail = WorkflowTemplateSummary & {
  content: string;
  json?: Record<string, unknown>;
};

type ListResponse = { ok: boolean; items: WorkflowTemplateSummary[]; count: number; total_count?: number; root: string; mode?: 'current' | 'history'; conflict_count?: number };
type DetailResponse = { ok: boolean; template: WorkflowTemplateDetail };
type Notice = { tone: Tone; text: string };
type StepView = { id: string; title: string; phase: string; required: boolean; depends: string[]; substitution: string };
type StepGroup = { phase: string; steps: StepView[] };
type MatchView = { label: string; values: string[] };

const LOCATIONS: Array<{ value: WorkflowLocation | 'all'; label: string }> = [
  { value: 'all', label: '全部' },
  { value: 'drafts', label: '草稿' },
  { value: 'published', label: '已发布' },
  { value: 'retired', label: '已退役' },
];

function locationLabel(value: WorkflowLocation): string {
  return value === 'drafts' ? '草稿' : value === 'published' ? '已发布' : '已退役';
}

function statusTone(template?: Pick<WorkflowTemplateSummary, 'location' | 'status'>): Tone {
  if (!template) return 'muted';
  if (template.location === 'published' && template.status === 'active') return 'ok';
  if (template.location === 'published') return 'warn';
  if (template.location === 'drafts') return 'warn';
  return 'muted';
}

function parseTemplate(content: string): { body: Record<string, unknown>; id: string; version: string; title: string; description: string; stepCount: number; error?: string } {
  try {
    const body = JSON.parse(content || '{}') as Record<string, unknown>;
    const id = text(body.id);
    const version = text(body.version);
    const title = text(body.title);
    const description = text(body.description);
    const steps = array(body.steps).length;
    if (!id || !version) return { body, id, version, title, description, stepCount: steps, error: 'JSON 需要包含 id 和 version。' };
    return { body, id, version, title, description, stepCount: steps };
  } catch (error) {
    return { body: {}, id: '', version: '', title: '', description: '', stepCount: 0, error: error instanceof Error ? error.message : 'JSON 解析失败' };
  }
}

export default function WorkflowTemplatesPage({ refreshToken }: { refreshToken: number }) {
  const [items, setItems] = useState<WorkflowTemplateSummary[]>([]);
  const [root, setRoot] = useState('');
  const [location, setLocation] = useState<WorkflowLocation | 'all'>('all');
  const [query, setQuery] = useState('');
  const [selected, setSelected] = useState<WorkflowTemplateDetail | null>(null);
  const [content, setContent] = useState('');
  const [loading, setLoading] = useState(true);
  const [notice, setNotice] = useState<Notice | null>(null);

  const loadListRef = useRef(loadList);
  loadListRef.current = loadList;

  useEffect(() => { void loadListRef.current(); }, [refreshToken, location]);

  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) return items;
    return items.filter((item) => [item.id, item.version, item.title, item.description, item.status, item.location, item.file_name, ...(item.keywords || [])].flatMap((value) => value ? [value] : []).join(' ').toLowerCase().includes(needle));
  }, [items, query]);

  const parsed = useMemo(() => parseTemplate(content), [content]);
  const activeCount = items.filter((item) => item.location === 'published' && item.status === 'active').length;
  const draftCount = items.reduce((sum, item) => sum + (item.draft_count ?? (item.location === 'drafts' ? 1 : 0)), 0);
  const retiredCount = items.reduce((sum, item) => sum + (item.retired_count ?? (item.location === 'retired' ? 1 : 0)), 0);
  const conflictCount = items.filter((item) => item.has_conflict || (item.active_count ?? 0) > 1).length;
  const visibleMode = location === 'all' ? '当前版本' : locationLabel(location);

  async function loadList() {
    setLoading(true);
    setNotice(null);
    try {
      const params = new URLSearchParams();
      if (location !== 'all') params.set('location', location);
      const result = await api<ListResponse>(`/v1/workflow-templates${params.size ? `?${params.toString()}` : ''}`);
      setItems(result.items || []);
      setRoot(result.root || '');
      if (!selected && result.items?.[0]) await openTemplate(result.items[0]);
    } catch (error) {
      setNotice({ tone: 'danger', text: error instanceof Error ? error.message : '任务模板读取失败' });
    } finally {
      setLoading(false);
    }
  }

  async function openTemplate(template: WorkflowTemplateSummary) {
    setNotice(null);
    try {
      const result = await api<DetailResponse>(`/v1/workflow-templates/${template.location}/${template.file_name}`);
      setSelected(result.template);
      setContent(result.template.content);
    } catch (error) {
      setNotice({ tone: 'danger', text: error instanceof Error ? error.message : '模板详情读取失败' });
    }
  }

  function copyPath() {
    if (!selected) return;
    void navigator.clipboard?.writeText(selected.path);
    setNotice({ tone: 'ok', text: '模板路径已复制。' });
  }

  return <section className="workflow-page workflow-runtime-page">
    <div className="section-heading workflow-heading">
      <div>
        <h2>任务模板</h2>
        <p>只读查看 AgentDock Runtime API 暴露的工作流模板；生命周期写操作由 AgentDock 受控接口负责。</p>
      </div>
      <div className="workflow-heading-actions">
        <button type="button" className="nx-button is-secondary" onClick={() => void loadList()} disabled={loading}><RefreshCw size={15} />刷新</button>
      </div>
    </div>

    {notice && <div className={`nx-alert is-${notice.tone === 'danger' ? 'error' : notice.tone === 'ok' ? 'success' : 'warning'}`}>{notice.text}</div>}

    <section className="workflow-runtime-banner">
      <div><span>RUNTIME VIEWER</span><strong>只读模式</strong><p>Nexus 不再直接写 workflows 目录；发布、退役、保存将在 AgentDock 写接口完成后再出现。时间按 {timeZoneLabel()} 显示。</p></div>
      <StatusPill tone="ok">AgentDock API</StatusPill>
    </section>

    <section className="workflow-metrics workflow-runtime-metrics">
      <MetricCard value={String(items.length)} label={visibleMode} />
      <MetricCard value={String(activeCount)} label="Active 当前版" />
      <MetricCard value={String(draftCount)} label="草稿版本" />
      <MetricCard value={String(retiredCount)} label="退役历史" />
      <MetricCard value={String(conflictCount)} label="多 Active 异常" tone={conflictCount ? 'danger' : 'ok'} />
      <MetricCard value={root || 'agentdock-runtime-api'} label="数据源" wide />
    </section>

    <section className="workflow-layout workflow-runtime-layout">
      <aside className="workflow-list-panel workflow-runtime-list-panel">
        <div className="workflow-toolbar">
          <label><span>状态</span><select value={location} onChange={(event) => setLocation(event.target.value as WorkflowLocation | 'all')}>{LOCATIONS.map((item) => <option key={item.value} value={item.value}>{item.label}</option>)}</select></label>
          <label className="workflow-search"><Search size={15} /><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索 id、标题、关键词" /></label>
        </div>
        <div className="workflow-list-summary"><strong>{filtered.length}</strong><span>当前列表</span><em>{location === 'all' ? '全部状态' : locationLabel(location)}</em></div>
        <div className="workflow-list workflow-runtime-list">
          {loading ? <p className="empty-mini">正在读取任务模板…</p> : filtered.length === 0 ? <p className="empty-mini">没有匹配的模板。</p> : filtered.map((item) => <button type="button" key={item.path} className={selected?.path === item.path ? 'is-active' : ''} onClick={() => void openTemplate(item)}>
            <span className="workflow-file-icon"><FileJson size={16} /></span>
            <span><strong>{item.id || item.file_name}</strong><small>{item.title || '无标题'} · {item.version || 'no version'} · {item.version_count ?? 1} 个版本</small></span>
            <StatusPill tone={item.has_conflict ? 'danger' : statusTone(item)}>{item.has_conflict ? `Active×${item.active_count}` : item.status || locationLabel(item.location)}</StatusPill>
          </button>)}
        </div>
      </aside>

      <main className="workflow-runtime-viewer">
        {!selected ? <div className="empty-state"><span><FileJson size={24} /></span><h3>选择模板</h3><p>从左侧选择一个模板查看 Runtime API 内容。</p></div> : <RuntimeTemplateViewer selected={selected} parsed={parsed} onCopy={copyPath} />}
      </main>
    </section>
  </section>;
}

function RuntimeTemplateViewer({ selected, parsed, onCopy }: { selected: WorkflowTemplateDetail; parsed: ReturnType<typeof parseTemplate>; onCopy: () => void }) {
  const match = record(parsed.body.match);
  const steps = stepViews(parsed.body.steps);
  const conditions = stringValues(parsed.body.completion_conditions);
  const keywords = [...stringValues(match.keywords), ...(selected.keywords || [])].filter((value, index, list) => value && list.indexOf(value) === index);
  const matchRows = matchViews(match);
  const stepGroups = groupSteps(steps);
  const phases = stepGroups.flatMap((group) => group.phase ? [group.phase] : []);
  const raw = selected.json || parsed.body;
  return <article className="workflow-runtime-card">
    <header className="workflow-runtime-head">
      <div>
        <span className="nexus-eyebrow">{selected.path}</span>
        <h3>{parsed.title || selected.title || selected.file_name}</h3>
        <p>{parsed.description || selected.description || '暂无模板说明。'}</p>
      </div>
      <div className="workflow-runtime-actions">
        <div className="workflow-runtime-head-stat"><strong>{steps.length || selected.step_count || 0}</strong><span>steps</span></div>
        <StatusPill tone={selected.has_conflict ? 'danger' : statusTone(selected)}>{selected.has_conflict ? `Active×${selected.active_count}` : selected.status || selected.location}</StatusPill>
        <button type="button" className="nx-button is-secondary" onClick={onCopy}><Copy size={15} />复制路径</button>
      </div>
    </header>

    <div className="workflow-runtime-meta">
      <StatusPill tone={parsed.error ? 'danger' : 'ok'}>{parsed.error || <><Check size={13} /> JSON 可解析</>}</StatusPill>
      <span>{selected.id}@{selected.version}</span>
      <span>{selected.version_count ?? 1} 个版本</span>
      <span>{formatTime(selected.updated_at)}</span>
    </div>

    <section className="workflow-runtime-grid">
      <InfoTile label="模板 ID" value={parsed.id || selected.id} />
      <InfoTile label="版本" value={parsed.version || selected.version} />
      <InfoTile label="步骤" value={`${steps.length || selected.step_count || 0} 个`} />
      <InfoTile label="阶段" value={phases.join(' / ') || '未声明'} />
      <InfoTile label="草稿 / 历史" value={`${selected.draft_count ?? 0} / ${selected.retired_count ?? 0}`} />
      <InfoTile label="文件名" value={selected.file_name} />
    </section>

    <section className="workflow-runtime-section workflow-runtime-section-soft">
      <SectionTitle title="匹配规则" subtitle="模型用这些信号判断是否应该使用该模板。" />
      {keywords.length > 0 && <ChipRow values={keywords} />}
      {matchRows.length === 0 ? <EmptyMini>没有匹配规则。</EmptyMini> : <div className="workflow-match-grid">{matchRows.map((row) => <div key={row.label}><span>{row.label}</span><p>{row.values.join(' · ')}</p></div>)}</div>}
    </section>

    <section className="workflow-runtime-section">
      <SectionTitle title="完成条件" subtitle="任务结束前必须满足的可验证条件。" />
      {conditions.length === 0 ? <EmptyMini>没有完成条件。</EmptyMini> : <div className="workflow-condition-list">{conditions.map((condition, index) => <div key={`${condition}:${index}`}><span>{index + 1}</span><p>{condition}</p></div>)}</div>}
    </section>

    <section className="workflow-runtime-section">
      <SectionTitle title="执行步骤" subtitle="按阶段拆分的运行步骤，只读展示。" />
      {steps.length === 0 ? <EmptyMini>没有步骤。</EmptyMini> : <div className="workflow-phase-list">{stepGroups.map((group) => <div className="workflow-phase-block" key={group.phase}><header><span>{group.phase}</span><strong>{group.steps.length} steps</strong></header><div className="workflow-step-list">{group.steps.map((step, index) => <StepCard key={`${group.phase}:${step.id}:${index}`} step={step} index={steps.indexOf(step) + 1} />)}</div></div>)}</div>}
    </section>

    <details className="workflow-runtime-json"><summary>查看 Runtime 原始 JSON</summary><pre>{JSON.stringify(raw, null, 2)}</pre></details>
  </article>;
}

function StepCard({ step, index }: { step: StepView; index: number }) {
  return <div className="workflow-step-card"><div><span>{index}</span><strong>{step.title || step.id || `步骤 ${index}`}</strong></div><p>{step.id || '未声明 step id'}</p><footer><em>{step.phase || 'phase unknown'}</em>{step.required && <em>required</em>}{step.substitution && <em>{step.substitution}</em>}{step.depends.length > 0 && <em>depends: {step.depends.join(', ')}</em>}</footer></div>;
}

function StatusPill({ tone, children }: { tone: Tone; children: ReactNode }) {
  return <span className={`status-badge tone-${tone}`}><span />{children}</span>;
}

function MetricCard({ value, label, tone = 'muted', wide = false }: { value: string; label: string; tone?: Tone; wide?: boolean }) {
  return <article className={wide ? 'is-wide' : ''}><strong className={`tone-${tone}`}>{value}</strong><span>{label}</span></article>;
}

function InfoTile({ label, value }: { label: string; value: string }) {
  return <div className="workflow-info-tile"><span>{label}</span><strong>{value || '暂无'}</strong></div>;
}

function SectionTitle({ title, subtitle }: { title: string; subtitle: string }) {
  return <header className="workflow-section-title"><div><h4>{title}</h4><p>{subtitle}</p></div></header>;
}

function ChipRow({ values }: { values: string[] }) {
  return <div className="workflow-chip-row">{values.slice(0, 18).map((value) => <span key={value}>{value}</span>)}</div>;
}

function EmptyMini({ children }: { children: ReactNode }) {
  return <p className="empty-mini">{children}</p>;
}

function record(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {};
}

function array(value: unknown): unknown[] {
  return Array.isArray(value) ? value : [];
}

function text(value: unknown): string {
  if (typeof value === 'string') return value;
  if (typeof value === 'number' || typeof value === 'boolean') return String(value);
  return '';
}

function stringValues(value: unknown): string[] {
  if (Array.isArray(value)) return value.flatMap((item) => { const current = text(item); return current ? [current] : []; });
  const single = text(value);
  return single ? [single] : [];
}

function groupSteps(steps: StepView[]): StepGroup[] {
  const groups = new Map<string, StepView[]>();
  for (const step of steps) {
    const phase = step.phase || '未分阶段';
    groups.set(phase, [...(groups.get(phase) || []), step]);
  }
  return Array.from(groups.entries()).map(([phase, groupSteps]) => ({ phase, steps: groupSteps }));
}

function stepViews(value: unknown): StepView[] {
  return array(value).map((item) => {
    const body = record(item);
    return {
      id: text(body.id),
      title: text(body.title) || text(body.name),
      phase: text(body.phase),
      required: body.required === true,
      depends: stringValues(body.depends_on || body.depends),
      substitution: text(body.substitution),
    };
  });
}

function matchViews(match: Record<string, unknown>): MatchView[] {
  const labels: Record<string, string> = { keywords: '关键词', devices: '设备', task_types: '任务类型', projects: '项目', tools: '工具', skills: 'Skill', priority: '优先级' };
  return Object.entries(match).reduce<MatchView[]>((rows, [key, value]) => {
    const values = stringValues(value);
    if (values.length > 0) rows.push({ label: labels[key] || key, values });
    return rows;
  }, []);
}
