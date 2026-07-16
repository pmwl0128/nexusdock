import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { Check, Copy, FileJson, Search } from 'lucide-react';
import { api } from '../../api/client';
import { formatTime } from '../../lib/time';
import MobileDrilldownBar from '../MobileDrilldownBar';

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

type ListResponse = { ok: boolean; items: WorkflowTemplateSummary[]; count: number };
type DetailResponse = { ok: boolean; template: Record<string, unknown>; template_summary: WorkflowTemplateSummary };
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
  if (template.location === 'published' || template.location === 'drafts') return 'warn';
  return 'muted';
}

function templateDisplayTitle(template?: Pick<WorkflowTemplateSummary, 'title' | 'id' | 'file_name'>): string {
  return template?.title?.trim() || template?.id || template?.file_name || '未命名模板';
}

function templateListMeta(template: WorkflowTemplateSummary): string {
  return [template.version || '未标记版本', `${template.step_count || 0} 步`, `${template.version_count ?? 1} 个版本`].join(' · ');
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
  const [location, setLocation] = useState<WorkflowLocation | 'all'>('all');
  const [query, setQuery] = useState('');
  const [selected, setSelected] = useState<WorkflowTemplateDetail | null>(null);
  const [mobileDetailOpen, setMobileDetailOpen] = useState(false);
  const [content, setContent] = useState('');
  const [loading, setLoading] = useState(true);
  const [notice, setNotice] = useState<Notice | null>(null);

  const loadListRef = useRef(loadList);
  loadListRef.current = loadList;
  useEffect(() => { void loadListRef.current(); }, [refreshToken, location]);

  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase();
    const scoped = location === 'all' ? items : items.filter((item) => item.location === location);
    if (!needle) return scoped;
    return scoped.filter((item) => [item.id, item.version, item.title, item.description, ...(item.keywords || [])].filter(Boolean).join(' ').toLowerCase().includes(needle));
  }, [items, location, query]);

  const parsed = useMemo(() => parseTemplate(content), [content]);

  async function loadList() {
    setLoading(true);
    setNotice(null);
    try {
      const result = await api<ListResponse>('/v1/workflow-templates?include_history=true');
      const nextItems = result.items || [];
      const visibleItems = location === 'all' ? nextItems : nextItems.filter((item) => item.location === location);
      setItems(nextItems);
      const selectedStillVisible = visibleItems.find((item) => item.path === selected?.path);
      if (selectedStillVisible) await openTemplate(selectedStillVisible);
      else if (visibleItems[0]) await openTemplate(visibleItems[0]);
      else {
        setSelected(null);
        setContent('');
      }
    } catch (error) {
      setNotice({ tone: 'danger', text: error instanceof Error ? error.message : 'Workflow 读取失败' });
    } finally {
      setLoading(false);
    }
  }

  async function openTemplate(template: WorkflowTemplateSummary, revealOnMobile = false) {
    setNotice(null);
    try {
      const result = await api<DetailResponse>(`/v1/workflow-templates/${encodeURIComponent(template.id)}/${encodeURIComponent(template.version)}`);
      const detail: WorkflowTemplateDetail = {
        ...result.template_summary,
        content: JSON.stringify(result.template, null, 2),
        json: result.template,
      };
      setSelected(detail);
      setContent(detail.content);
      if (revealOnMobile) setMobileDetailOpen(true);
    } catch (error) {
      setNotice({ tone: 'danger', text: error instanceof Error ? error.message : '模板详情读取失败' });
    }
  }

  function copyPath() {
    if (!selected) return;
    void navigator.clipboard?.writeText(selected.path);
    setNotice({ tone: 'ok', text: '模板路径已复制。' });
  }

  return <section className="workflow-page workflow-runtime-page workflow-focus-page">
    {notice && <div className={`nx-alert is-${notice.tone === 'danger' ? 'error' : notice.tone === 'ok' ? 'success' : 'warning'}`}>{notice.text}</div>}

    <section className={`workflow-layout workflow-runtime-layout mobile-drilldown ${mobileDetailOpen ? 'is-detail-open' : 'is-list-open'}`}>
      <aside className="workflow-list-panel workflow-runtime-list-panel mobile-drilldown-list">
        <div className="workflow-toolbar">
          <label><span>状态</span><select aria-label="筛选模板状态" value={location} onChange={(event) => { setLocation(event.target.value as WorkflowLocation | 'all'); setMobileDetailOpen(false); }}>{LOCATIONS.map((item) => <option key={item.value} value={item.value}>{item.label}</option>)}</select></label>
          <label className="workflow-search"><Search size={15} /><input aria-label="搜索 Workflow 模板" value={query} onChange={(event) => { setQuery(event.target.value); setMobileDetailOpen(false); }} placeholder="搜索标题或关键词" /></label>
        </div>
        <div className="workflow-list-summary"><strong>{filtered.length}</strong><span>个模板</span><em>{location === 'all' ? '全部' : locationLabel(location)}</em></div>
        <div className="workflow-list workflow-runtime-list">
          {loading ? <p className="empty-mini">正在读取 Workflow…</p> : filtered.length === 0 ? <p className="empty-mini">没有匹配的模板。</p> : filtered.map((item) => <button type="button" key={item.path} className={selected?.path === item.path ? 'is-active' : ''} aria-pressed={selected?.path === item.path} onClick={() => void openTemplate(item, true)}>
            <span className="workflow-file-icon"><FileJson size={16} /></span>
            <span><strong>{templateDisplayTitle(item)}</strong><small>{templateListMeta(item)}</small></span>
            <StatusPill tone={item.has_conflict ? 'danger' : statusTone(item)}>{item.has_conflict ? `Active×${item.active_count}` : item.status || locationLabel(item.location)}</StatusPill>
          </button>)}
        </div>
      </aside>

      <main className="workflow-runtime-viewer mobile-drilldown-detail">
        {selected && <MobileDrilldownBar label="模板详情" title={templateDisplayTitle(selected)} meta={selected.version || selected.status} backLabel="返回模板列表" onBack={() => setMobileDetailOpen(false)} />}
        {!selected ? <div className="empty-state"><span><FileJson size={24} /></span><h3>选择模板</h3><p>从左侧选择一个模板查看执行步骤。</p></div> : <RuntimeTemplateViewer selected={selected} parsed={parsed} onCopy={copyPath} />}
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

  return <article className="workflow-runtime-card workflow-focus-card">
    <header className="workflow-runtime-head">
      <div><span className="nexus-eyebrow">{selected.id}</span><h3>{parsed.title || selected.title || selected.file_name}</h3><p>{parsed.description || selected.description || '暂无模板说明。'}</p></div>
      <div className="workflow-runtime-actions"><StatusPill tone={selected.has_conflict ? 'danger' : statusTone(selected)}>{selected.has_conflict ? `Active×${selected.active_count}` : selected.status || selected.location}</StatusPill><span className="workflow-step-count">{steps.length || selected.step_count || 0} 步</span></div>
    </header>

    <div className="workflow-runtime-meta"><span>版本 {selected.version}</span><span>{phases.length || 1} 个阶段</span><span>更新于 {formatTime(selected.updated_at)}</span></div>

    <section className="workflow-runtime-section workflow-primary-section">
      <SectionTitle title="执行步骤" subtitle="按阶段查看任务主流程。" />
      {steps.length === 0 ? <EmptyMini>没有步骤。</EmptyMini> : <div className="workflow-phase-list">{stepGroups.map((group) => <div className="workflow-phase-block" key={group.phase}><header><span>{group.phase}</span><strong>{group.steps.length} 步</strong></header><div className="workflow-step-list">{group.steps.map((step, index) => <StepCard key={`${group.phase}:${step.id}:${index}`} step={step} index={steps.indexOf(step) + 1} />)}</div></div>)}</div>}
    </section>

    <section className="workflow-runtime-section">
      <SectionTitle title="完成条件" subtitle="任务结束前必须满足的结果。" />
      {conditions.length === 0 ? <EmptyMini>没有完成条件。</EmptyMini> : <div className="workflow-condition-list">{conditions.map((condition, index) => <div key={`${condition}:${index}`}><span>{index + 1}</span><p>{condition}</p></div>)}</div>}
    </section>

    <details className="workflow-secondary-details">
      <summary>匹配与技术信息</summary>
      <div className="workflow-secondary-body">
        <section>
          <SectionTitle title="匹配规则" subtitle="模型用这些信号判断是否使用该模板。" />
          {keywords.length > 0 && <ChipRow values={keywords} />}
          {matchRows.length === 0 ? <EmptyMini>没有匹配规则。</EmptyMini> : <div className="workflow-match-grid">{matchRows.map((row) => <div key={row.label}><span>{row.label}</span><p>{row.values.join(' · ')}</p></div>)}</div>}
        </section>
        <section className="workflow-runtime-grid is-compact">
          <InfoTile label="模板 ID" value={parsed.id || selected.id} />
          <InfoTile label="文件名" value={selected.file_name} />
          <InfoTile label="版本数" value={String(selected.version_count ?? 1)} />
          <InfoTile label="草稿 / 历史" value={`${selected.draft_count ?? 0} / ${selected.retired_count ?? 0}`} />
          <InfoTile label="JSON" value={parsed.error || '可解析'} />
        </section>
        <div className="workflow-technical-actions"><button type="button" className="nx-button is-secondary" onClick={onCopy}><Copy size={15} />复制模板路径</button></div>
        <details className="workflow-runtime-json"><summary><Check size={13} />查看 Runtime 原始 JSON</summary><pre>{JSON.stringify(raw, null, 2)}</pre></details>
      </div>
    </details>
  </article>;
}

function StepCard({ step, index }: { step: StepView; index: number }) {
  return <div className="workflow-step-card"><div><span>{index}</span><strong>{step.title || step.id || `步骤 ${index}`}</strong></div><footer><em>{step.phase || '未分阶段'}</em>{step.required && <em>必需</em>}{step.depends.length > 0 && <em>依赖 {step.depends.join(', ')}</em>}{step.substitution && <em>{step.substitution}</em>}</footer></div>;
}

function StatusPill({ tone, children }: { tone: Tone; children: ReactNode }) {
  return <span className={`status-badge tone-${tone}`}><span />{children}</span>;
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
  return Array.from(groups.entries()).map(([phase, groupedSteps]) => ({ phase, steps: groupedSteps }));
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
  const labels: Record<string, string> = { keywords: '关键词', devices: '运行端', task_types: '任务类型', projects: '项目', tools: '工具', skills: 'Skill', priority: '优先级' };
  return Object.entries(match).reduce<MatchView[]>((rows, [key, value]) => {
    const values = stringValues(value);
    if (values.length > 0) rows.push({ label: labels[key] || key, values });
    return rows;
  }, []);
}
