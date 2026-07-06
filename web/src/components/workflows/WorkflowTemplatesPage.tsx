import { useEffect, useMemo, useState } from 'react';
import { Check, Copy, FileJson, RefreshCw, Search } from 'lucide-react';
import { api } from '../../api/client';

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

function formatTime(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'short', timeStyle: 'short' }).format(date);
}

function fileNameFor(id: string, version: string): string {
  return `${id.trim()}@${version.trim()}.json`;
}

function starterTemplate(): WorkflowTemplateDetail {
  const content = JSON.stringify({
    id: 'workflow.new-template',
    version: '1.0.0',
    title: '新任务模板',
    description: '说明这个模板适合什么任务，避免泛化。',
    status: 'draft',
    match: {
      keywords: ['任务模板'],
      devices: ['DockMini'],
      task_types: ['template-maintenance'],
      priority: 50,
    },
    completion_conditions: ['已完成真实检查、执行、验证和收尾'],
    steps: [
      { id: 'check_context', title: '检查上下文和真实环境', phase: 'check', required: true, substitution: 'forbidden' },
      { id: 'execute_work', title: '执行模板定义的真实工作', phase: 'execute', required: true, depends_on: ['check_context'], substitution: 'forbidden' },
      { id: 'verify_result', title: '验证结果并记录证据', phase: 'verify', required: true, depends_on: ['execute_work'], substitution: 'forbidden' },
    ],
  }, null, 2);
  return {
    id: 'workflow.new-template',
    version: '1.0.0',
    title: '新任务模板',
    description: '说明这个模板适合什么任务，避免泛化。',
    status: 'draft',
    location: 'drafts',
    file_name: 'workflow.new-template@1.0.0.json',
    path: 'drafts/workflow.new-template@1.0.0.json',
    size_bytes: content.length,
    updated_at: new Date().toISOString(),
    step_count: 3,
    keywords: ['任务模板'],
    content,
    json: JSON.parse(content) as Record<string, unknown>,
  };
}

function parseTemplate(content: string): { id: string; version: string; title: string; stepCount: number; error?: string } {
  try {
    const body = JSON.parse(content) as Record<string, unknown>;
    const id = typeof body.id === 'string' ? body.id : '';
    const version = typeof body.version === 'string' ? body.version : '';
    const title = typeof body.title === 'string' ? body.title : '';
    const steps = Array.isArray(body.steps) ? body.steps.length : 0;
    if (!id || !version) return { id, version, title, stepCount: steps, error: 'JSON 需要包含 id 和 version。' };
    return { id, version, title, stepCount: steps };
  } catch (error) {
    return { id: '', version: '', title: '', stepCount: 0, error: error instanceof Error ? error.message : 'JSON 解析失败' };
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

  useEffect(() => { void loadList(); }, [refreshToken, location]);

  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) return items;
    return items.filter((item) => [item.id, item.version, item.title, item.description, item.status, item.location, item.file_name, ...(item.keywords || [])].filter(Boolean).join(' ').toLowerCase().includes(needle));
  }, [items, query]);

  const parsed = useMemo(() => parseTemplate(content), [content]);
  const dirty = false;
  const activeCount = items.filter((item) => item.location === 'published' && item.status === 'active').length;
  const draftCount = items.reduce((sum, item) => sum + (item.draft_count ?? (item.location === 'drafts' ? 1 : 0)), 0);
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

  return <section className="workflow-page">
    <div className="section-heading workflow-heading">
      <div>
        <h2>任务模板</h2>
        <p>通过 AgentDock Runtime API 只读查看任务模板。发布、退役、保存需要 AgentDock 暴露受控写接口后再启用。</p>
      </div>
      <div className="workflow-heading-actions">
        <button className="nx-button is-secondary" onClick={() => void loadList()} disabled={loading}><RefreshCw size={15} />刷新</button>
      </div>
    </div>

    {notice && <div className={`nx-alert is-${notice.tone === 'danger' ? 'error' : notice.tone === 'ok' ? 'success' : 'warning'}`}>{notice.text}</div>}
    <div className="nx-alert is-info">只读模式：当前页面不直接写 AgentDock workflows 目录，避免绕过 Runtime 生命周期。</div>

    <section className="workflow-metrics">
      <article><strong>{items.length}</strong><span>{visibleMode}</span></article>
      <article><strong>{activeCount}</strong><span>Active 当前版</span></article>
      <article><strong>{conflictCount}</strong><span>多 Active 异常</span></article>
      <article><strong>{root || '未配置'}</strong><span>workflow root</span></article>
    </section>

    <section className="workflow-layout">
      <aside className="workflow-list-panel">
        <div className="workflow-toolbar">
          <label><span>状态</span><select value={location} onChange={(event) => setLocation(event.target.value as WorkflowLocation | 'all')}>{LOCATIONS.map((item) => <option key={item.value} value={item.value}>{item.label}</option>)}</select></label>
          <label className="workflow-search"><Search size={15} /><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索 id、标题、关键词" /></label>
        </div>
        <div className="workflow-list">
          {loading ? <p className="empty-mini">正在读取任务模板…</p> : filtered.length === 0 ? <p className="empty-mini">没有匹配的模板。</p> : filtered.map((item) => <button key={item.path} className={selected?.path === item.path ? 'is-active' : ''} onClick={() => void openTemplate(item)}>
            <span className="workflow-file-icon"><FileJson size={16} /></span>
            <span><strong>{item.id || item.file_name}</strong><small>{item.title || '无标题'} · {item.version || 'no version'} · {item.version_count ?? 1} 个版本</small></span>
            <StatusPill tone={item.has_conflict ? 'danger' : statusTone(item)}>{item.has_conflict ? `Active×${item.active_count}` : item.status || locationLabel(item.location)}</StatusPill>
          </button>)}
        </div>
      </aside>

      <main className="workflow-editor-panel">
        {!selected ? <div className="empty-state"><span><FileJson size={24} /></span><h3>选择模板</h3><p>从左侧选择一个任务模板查看 Runtime API 内容。</p></div> : <>
          <header className="workflow-editor-head">
            <div>
              <span className="nexus-eyebrow">{selected.path}</span>
              <h3>{parsed.title || selected.title || selected.file_name}</h3>
              <p>{parsed.id || selected.id} · {parsed.version || selected.version} · {parsed.stepCount} 个步骤 · {selected.version_count ?? 1} 个版本 · 更新于 {formatTime(selected.updated_at)}</p>
            </div>
            <div className="workflow-editor-actions">
              <button className="nx-button is-secondary" onClick={copyPath}><Copy size={15} />复制路径</button>
              <button className="nx-button" disabled title="Runtime 写接口未启用">发布已禁用</button>
              <button className="nx-button is-secondary" disabled title="Runtime 写接口未启用">退役已禁用</button>
              <button className="nx-button" disabled title="Runtime 写接口未启用">保存已禁用</button>
            </div>
          </header>

          <div className="workflow-editor-meta">
            <StatusPill tone={selected.has_conflict ? 'danger' : statusTone(selected)}>{selected.has_conflict ? `Active×${selected.active_count}` : selected.status || selected.location}</StatusPill>
            <span>{selected.size_bytes} bytes</span>
            <span>草稿 {selected.draft_count ?? 0} / 历史 {selected.retired_count ?? 0}</span>
            {dirty && <span className="workflow-dirty">有未保存修改</span>}
            {parsed.error ? <span className="workflow-json-error">{parsed.error}</span> : <span className="workflow-json-ok"><Check size={13} /> JSON 可解析</span>}
          </div>

          <textarea className="workflow-json-editor" value={content} spellCheck={false} readOnly onChange={() => undefined} />
        </>}
      </main>
    </section>
  </section>;
}

function StatusPill({ tone, children }: { tone: Tone; children: React.ReactNode }) {
  return <span className={`status-badge tone-${tone}`}><span />{children}</span>;
}
