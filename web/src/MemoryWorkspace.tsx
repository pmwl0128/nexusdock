import { useEffect, useMemo, useState, type FormEvent } from 'react';
import {
  Archive, Check, Clock3, Cpu, FileText, Folder, GitBranch, Pencil, Plus,
  RefreshCw, Save, Search, Sparkles, Trash2, UploadCloud,
} from 'lucide-react';
import { api } from './api/client';
import Dialog from './components/Dialog';
import { clearMemoryDraft, loadMemoryDraft, saveMemoryDraft } from './lib/drafts';

type EntryType = 'file' | 'directory';
type MemoryEntry = { path: string; name?: string; type: EntryType; size_bytes?: number };
type Memory = { path: string; content: string };
type GitCommit = { hash: string; short_hash: string; date: string; author: string; subject: string };
type ChangedFile = { status: string; path: string };
type GitDiff = { ok?: boolean; git_repo?: boolean; dirty?: boolean; status?: string; stat?: string; files?: ChangedFile[] };
type SyncStatus = Record<string, unknown> & {
  dirty?: boolean; ahead?: string | number; behind?: string | number;
  pending_push?: boolean; branch?: string; remote?: string;
};
type Notice = { text: string; danger?: boolean } | null;
type PendingMemoryAction =
  | { kind: 'move'; path: string; nextPath: string; error?: string }
  | { kind: 'delete'; path: string; error?: string }
  | null;

type MemoryCard = {
  title: string;
  content: string;
  type: string;
  project: string;
  status: string;
  confidence: string;
  tags?: string[];
  source: string;
  evidence?: string;
  path: string;
};

type CardSearchResult = { path: string; title?: string; score?: number; size_bytes?: number };
type CardCaptureResult = {
  ok: boolean;
  card: MemoryCard;
  warnings?: string[];
  capture_plan?: Record<string, unknown>;
  similar_results?: CardSearchResult[];
  similar_count?: number;
};
type CardWriteResult = { ok: boolean; card: MemoryCard; memory: Memory; warnings?: string[]; index_policy?: string };
type EmbeddingStatus = {
  enabled?: boolean;
  configured?: boolean;
  reachable?: boolean;
  model?: string;
  endpoint?: string;
  index_path?: string;
  count?: number;
  dimension?: number;
  error?: string;
  reason?: string;
};
type EmbeddingSearchResult = { path: string; title?: string; score: number };
type EmbeddingSearchResponse = { ok?: boolean; count?: number; model?: string; index?: { count?: number; dimension?: number }; results?: EmbeddingSearchResult[] };

const NEW_MEMORY_TEMPLATE = `---
type: note
scope: inbox
source: user-confirmed
confidence: medium
---

# 新召回条目

`;

function normalizePath(value: string): string {
  return String(value || '').replace(/^\/+|\/+$/g, '').replace(/\/+/g, '/');
}

function nameOf(path: string): string {
  const parts = normalizePath(path).split('/').filter(Boolean);
  return parts.at(-1) || path;
}

function formatBytes(value?: number): string {
  if (!value) return '—';
  if (value < 1024) return `${value} B`;
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KiB`;
  return `${(value / 1024 / 1024).toFixed(1)} MiB`;
}

function formatTime(value?: string): string {
  if (!value) return '未知';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString('zh-CN', { hour12: false });
}

function initialPath(): string {
  return normalizePath(new URLSearchParams(window.location.search).get('path') || '');
}

function updateRoute(path = '', query = '') {
  const params = new URLSearchParams(window.location.search);
  params.delete('tab');
  params.delete('prefix');
  if (path) params.set('path', path); else params.delete('path');
  if (query) params.set('q', query); else params.delete('q');
  const next = `${window.location.pathname}${params.size ? `?${params.toString()}` : ''}#memory`;
  window.history.replaceState(null, '', next);
}

function messageOf(reason: unknown): string {
  return reason instanceof Error ? reason.message : '操作失败';
}

export default function RecallWorkspace() {
  const [entries, setEntries] = useState<MemoryEntry[]>([]);
  const [current, setCurrent] = useState<Memory | null>(null);
  const [draftPath, setDraftPath] = useState('');
  const [draftContent, setDraftContent] = useState('');
  const [editing, setEditing] = useState(false);
  const [creating, setCreating] = useState(false);
  const [query, setQuery] = useState(() => new URLSearchParams(window.location.search).get('q') || '');
  const [syncStatus, setSyncStatus] = useState<SyncStatus | null>(null);
  const [gitDiff, setGitDiff] = useState<GitDiff | null>(null);
  const [commits, setCommits] = useState<GitCommit[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState<Notice>(null);
  const [draftAvailable, setDraftAvailable] = useState(false);
  const [pendingAction, setPendingAction] = useState<PendingMemoryAction>(null);
  const [cardTitle, setCardTitle] = useState('');
  const [cardContent, setCardContent] = useState('');
  const [cardProject, setCardProject] = useState('agentdock');
  const [cardType, setCardType] = useState('runbook');
  const [cardTags, setCardTags] = useState('');
  const [cardSource, setCardSource] = useState('nexus-recall-ui');
  const [cardEvidence, setCardEvidence] = useState('');
  const [cardPath, setCardPath] = useState('');
  const [allowCardWarnings, setAllowCardWarnings] = useState(false);
  const [cardCapture, setCardCapture] = useState<CardCaptureResult | null>(null);
  const [embeddingStatus, setEmbeddingStatus] = useState<EmbeddingStatus | null>(null);
  const [embeddingQuery, setEmbeddingQuery] = useState('');
  const [embeddingResults, setEmbeddingResults] = useState<EmbeddingSearchResult[]>([]);

  const fileEntries = useMemo(
    () => entries.filter((entry) => entry.type === 'file').sort((a, b) => a.path.localeCompare(b.path, 'zh-CN')),
    [entries],
  );
  const directoryCount = useMemo(
    () => new Set(fileEntries.map((entry) => normalizePath(entry.path).split('/').slice(0, -1).join('/')).filter(Boolean)).size,
    [fileEntries],
  );
  const changedCount = gitDiff?.files?.length ?? 0;
  const dirty = Boolean(gitDiff?.dirty || syncStatus?.dirty || syncStatus?.pending_push);
  const hasUnsavedChanges = editing && (draftPath !== (current?.path || '') || draftContent !== (current?.content || ''));

  useEffect(() => {
    const saved = loadMemoryDraft();
    if (saved?.path || saved?.content) setDraftAvailable(true);
    void refreshAll(initialPath());
  }, []);

  useEffect(() => {
    if (!editing) return;
    const timer = window.setTimeout(() => saveMemoryDraft(draftPath, draftContent), 250);
    return () => window.clearTimeout(timer);
  }, [editing, draftPath, draftContent]);

  useEffect(() => {
    if (!notice) return;
    const timer = window.setTimeout(() => setNotice(null), 3500);
    return () => window.clearTimeout(timer);
  }, [notice]);

  async function refreshAll(path = current?.path || '') {
    setLoading(true);
    try {
      await Promise.all([loadList(), loadSyncState(), loadHistory(), loadEmbeddingStatus()]);
      if (path) await openMemory(path);
    } catch (reason) {
      setNotice({ text: messageOf(reason), danger: true });
    } finally {
      setLoading(false);
    }
  }

  async function loadList() {
    const response = await api<{ entries: MemoryEntry[] }>('/v1/recall?max_entries=500');
    setEntries(response.entries || []);
  }

  async function searchMemories(event?: FormEvent) {
    event?.preventDefault();
    if (!query.trim()) {
      await loadList();
      updateRoute(current?.path || '', '');
      return;
    }
    const response = await api<{ results: Array<{ path: string; size_bytes?: number }> }>('/v1/recall/search', {
      method: 'POST',
      body: JSON.stringify({ query: query.trim(), prefix: '', max_results: 100 }),
    });
    setEntries((response.results || []).map((item) => ({ path: item.path, name: nameOf(item.path), type: 'file', size_bytes: item.size_bytes })));
    updateRoute(current?.path || '', query.trim());
  }

  async function openMemory(path: string) {
    const response = await api<{ memory: Memory }>(`/v1/recall/${encodeURIComponent(path)}`);
    setCurrent(response.memory);
    setDraftPath(response.memory.path);
    setDraftContent(response.memory.content);
    setEditing(false);
    setCreating(false);
    clearMemoryDraft();
    setDraftAvailable(false);
    updateRoute(response.memory.path, query.trim());
  }

  function startNew() {
    setCurrent(null);
    setDraftPath('inbox/new-memory.md');
    setDraftContent(NEW_MEMORY_TEMPLATE);
    setEditing(true);
    setCreating(true);
  }

  function startEdit() {
    if (!current) return;
    setDraftPath(current.path);
    setDraftContent(current.content);
    setEditing(true);
    setCreating(false);
  }

  function cancelEdit() {
    setDraftPath(current?.path || '');
    setDraftContent(current?.content || '');
    setEditing(false);
    setCreating(false);
    clearMemoryDraft();
    setDraftAvailable(false);
  }

  function restoreDraft() {
    const saved = loadMemoryDraft();
    if (!saved) return;
    setCurrent(null);
    setDraftPath(saved.path || 'inbox/recovered-memory.md');
    setDraftContent(saved.content);
    setEditing(true);
    setCreating(true);
    setDraftAvailable(false);
  }

  async function saveMemory() {
    const path = normalizePath(draftPath);
    if (!path || !draftContent.trim()) {
      setNotice({ text: '路径和内容不能为空', danger: true });
      return;
    }
    if (!/\.(md|markdown|txt)$/i.test(path)) {
      setNotice({ text: '召回文件必须使用 .md、.markdown 或 .txt 扩展名', danger: true });
      return;
    }
    setBusy(true);
    try {
      const existing = Boolean(current?.path) && !creating;
      const target = existing ? `/v1/recall/${encodeURIComponent(current!.path)}` : '/v1/recall';
      const response = await api<{ memory: Memory }>(target, {
        method: existing ? 'PATCH' : 'POST',
        body: JSON.stringify({ path: existing ? current!.path : path, content: draftContent, confirmed: true, overwrite: true }),
      });
      clearMemoryDraft();
      setDraftAvailable(false);
      await Promise.all([loadList(), loadSyncState(), loadHistory()]);
      await openMemory(response.memory.path);
      setNotice({ text: '召回内容已保存' });
    } catch (reason) {
      setNotice({ text: messageOf(reason), danger: true });
    } finally {
      setBusy(false);
    }
  }

  function requestMove() {
    if (!current?.path) return;
    setPendingAction({ kind: 'move', path: current.path, nextPath: current.path });
  }

  async function confirmMove() {
    if (!pendingAction || pendingAction.kind !== 'move') return;
    const normalized = normalizePath(pendingAction.nextPath);
    if (!normalized || normalized === pendingAction.path) {
      setPendingAction({ ...pendingAction, error: '请输入新的记忆路径。' });
      return;
    }
    if (!/\.(md|markdown|txt)$/i.test(normalized)) {
      setPendingAction({ ...pendingAction, error: '目标路径必须是 Markdown 或文本文件。' });
      return;
    }
    setBusy(true);
    try {
      const response = await api<{ memory: Memory }>('/v1/recall/move', {
        method: 'POST',
        body: JSON.stringify({ from_path: pendingAction.path, to_path: normalized, confirmed: true, overwrite: false }),
      });
      setPendingAction(null);
      await loadList();
      await openMemory(response.memory.path);
      setNotice({ text: '召回内容已移动' });
    } catch (reason) {
      setPendingAction({ ...pendingAction, error: messageOf(reason) });
    } finally {
      setBusy(false);
    }
  }

  function requestDelete() {
    if (!current?.path) return;
    setPendingAction({ kind: 'delete', path: current.path });
  }

  async function confirmDelete() {
    if (!pendingAction || pendingAction.kind !== 'delete') return;
    setBusy(true);
    try {
      await api(`/v1/recall/${encodeURIComponent(pendingAction.path)}?confirmed=true`, { method: 'DELETE' });
      setPendingAction(null);
      setCurrent(null);
      setDraftPath('');
      setDraftContent('');
      setEditing(false);
      updateRoute('', query.trim());
      await Promise.all([loadList(), loadSyncState(), loadHistory()]);
      setNotice({ text: '召回内容已删除' });
    } catch (reason) {
      setPendingAction({ ...pendingAction, error: messageOf(reason) });
    } finally {
      setBusy(false);
    }
  }

  async function loadSyncState() {
    const [sync, diff] = await Promise.all([
      api<SyncStatus>('/v1/sync/status'),
      api<GitDiff>('/v1/git/diff'),
    ]);
    setSyncStatus(sync);
    setGitDiff(diff);
  }

  async function loadHistory() {
    const response = await api<{ commits: GitCommit[] }>('/v1/git/log?limit=12');
    setCommits(response.commits || []);
  }

  async function loadEmbeddingStatus() {
    const response = await api<EmbeddingStatus>('/v1/embeddings/status');
    setEmbeddingStatus(response);
  }

  function cardPayload(extra: Record<string, unknown> = {}) {
    return {
      title: cardTitle.trim(),
      content: cardContent.trim(),
      project: cardProject.trim() || 'agentdock',
      type: cardType,
      status: 'inbox',
      confidence: 'medium',
      source: cardSource.trim() || 'nexus-recall-ui',
      evidence: cardEvidence.trim(),
      path: cardPath.trim(),
      tags: cardTags.split(',').map((tag) => tag.trim()).filter(Boolean),
      ...extra,
    };
  }

  async function captureCard(event?: FormEvent) {
    event?.preventDefault();
    if (!cardTitle.trim() || !cardContent.trim()) {
      setNotice({ text: '卡片标题和内容不能为空。', danger: true });
      return;
    }
    setBusy(true);
    try {
      const result = await api<CardCaptureResult>('/v1/cards/capture', {
        method: 'POST',
        body: JSON.stringify(cardPayload({ max_results: 6 })),
      });
      setCardCapture(result);
      setCardPath(result.card.path);
      setAllowCardWarnings(false);
      setNotice({ text: result.similar_count ? `已生成候选，发现 ${result.similar_count} 条相似卡片。` : '已生成候选卡片。' });
    } catch (reason) {
      setNotice({ text: messageOf(reason), danger: true });
    } finally {
      setBusy(false);
    }
  }

  async function writeCard() {
    if (!cardCapture) {
      setNotice({ text: '请先生成候选卡片。', danger: true });
      return;
    }
    if ((cardCapture.warnings?.length || 0) > 0 && !allowCardWarnings) {
      setNotice({ text: '候选卡片有警告，请确认后再写入。', danger: true });
      return;
    }
    setBusy(true);
    try {
      const result = await api<CardWriteResult>('/v1/cards', {
        method: 'POST',
        body: JSON.stringify(cardPayload({ confirmed: true, overwrite: false, allow_warnings: allowCardWarnings })),
      });
      await Promise.all([loadList(), loadSyncState(), loadHistory(), loadEmbeddingStatus()]);
      await openMemory(result.memory.path);
      setCardCapture(null);
      setNotice({ text: `卡片已写入：${result.card.path}` });
    } catch (reason) {
      setNotice({ text: messageOf(reason), danger: true });
    } finally {
      setBusy(false);
    }
  }

  async function reindexCards() {
    setBusy(true);
    try {
      await api('/v1/embeddings/reindex', {
        method: 'POST',
        body: JSON.stringify({ prefix: 'cards', max_entries: 1000 }),
      });
      await loadEmbeddingStatus();
      setNotice({ text: 'cards 向量索引已重建。' });
    } catch (reason) {
      setNotice({ text: messageOf(reason), danger: true });
    } finally {
      setBusy(false);
    }
  }

  async function searchCardEmbeddings(event?: FormEvent) {
    event?.preventDefault();
    const text = embeddingQuery.trim() || query.trim();
    if (!text) {
      setNotice({ text: '请输入要搜索的经验问题。', danger: true });
      return;
    }
    setBusy(true);
    try {
      const response = await api<EmbeddingSearchResponse>('/v1/embeddings/search', {
        method: 'POST',
        body: JSON.stringify({ query: text, prefix: 'cards', max_results: 8 }),
      });
      setEmbeddingResults(response.results || []);
      setNotice({ text: `向量搜索返回 ${response.count ?? response.results?.length ?? 0} 条结果。` });
    } catch (reason) {
      setNotice({ text: messageOf(reason), danger: true });
    } finally {
      setBusy(false);
    }
  }

  async function syncNow(action: 'pull' | 'push' | 'now' = 'now') {
    setBusy(true);
    try {
      const response = await api<SyncStatus>(`/v1/sync/${action}`, { method: 'POST', body: '{}' });
      setSyncStatus(response);
      await Promise.all([loadList(), loadSyncState(), loadHistory()]);
      if (current?.path) await openMemory(current.path).catch(() => setCurrent(null));
      setNotice({ text: action === 'pull' ? '已从远端更新' : action === 'push' ? '已保存到远端' : '召回库已同步' });
    } catch (reason) {
      setNotice({ text: messageOf(reason), danger: true });
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="mem-lite">
      <header className="mem-lite-header">
        <div>
          <span className="mem-lite-kicker">NEXUS RECALL</span>
          <h1>召回库</h1>
          <p>浏览、编辑和同步 Markdown 召回内容；复杂 Git 操作继续交给 Agent。</p>
        </div>
        <div className="mem-lite-header-actions">
          <span className={`mem-lite-health ${dirty ? 'warn' : 'ok'}`}>{dirty ? `${changedCount} 项待同步` : '已同步'}</span>
          <button onClick={() => void refreshAll()} disabled={loading || busy}><RefreshCw size={15} />刷新</button>
          <button className="primary" onClick={() => void syncNow()} disabled={busy}><UploadCloud size={15} />立即同步</button>
        </div>
      </header>

      {notice && <div className={`mem-lite-notice ${notice.danger ? 'danger' : ''}`}>{notice.danger ? null : <Check size={16} />}{notice.text}</div>}
      {draftAvailable && !editing && <div className="mem-lite-notice"><span>检测到未提交草稿。</span><button onClick={restoreDraft}>恢复草稿</button><button onClick={() => { clearMemoryDraft(); setDraftAvailable(false); }}>丢弃</button></div>}
      {pendingAction && (
        <Dialog
          title={pendingAction.kind === 'move' ? '移动召回内容' : '删除召回内容'}
          description={pendingAction.kind === 'move' ? '修改路径后会保留文件内容，并刷新当前打开的记忆。' : '删除后会产生本地 Git 变更，需要同步后才会进入远端。'}
          onClose={() => { if (!busy) setPendingAction(null); }}
        >
          <div className="mem-lite-dialog-body">
            {pendingAction.kind === 'move' ? (
              <label>
                <span>新的记忆路径</span>
                <input
                  value={pendingAction.nextPath}
                  onChange={(event) => setPendingAction({ ...pendingAction, nextPath: event.target.value, error: undefined })}
                  autoFocus
                />
              </label>
            ) : (
              <div className="mem-lite-danger-box">
                <strong>确认删除这条记忆？</strong>
                <code>{pendingAction.path}</code>
              </div>
            )}
            {'error' in pendingAction && pendingAction.error && <p className="mem-lite-dialog-error">{pendingAction.error}</p>}
            <div className="mem-lite-dialog-actions">
              <button type="button" onClick={() => setPendingAction(null)} disabled={busy}>取消</button>
              {pendingAction.kind === 'move' ? (
                <button className="primary" type="button" onClick={() => void confirmMove()} disabled={busy}>确认移动</button>
              ) : (
                <button className="danger" type="button" onClick={() => void confirmDelete()} disabled={busy}>确认删除</button>
              )}
            </div>
          </div>
        </Dialog>
      )}

      <section className="mem-lite-stats">
        <div><Archive size={18} /><span>文件</span><strong>{fileEntries.length}</strong></div>
        <div><Folder size={18} /><span>目录</span><strong>{directoryCount}</strong></div>
        <div><GitBranch size={18} /><span>本地变更</span><strong>{changedCount}</strong></div>
        <div><Clock3 size={18} /><span>最近版本</span><strong>{commits.length}</strong></div>
      </section>

      <section className="mem-card-console">
        <article className="mem-card-panel">
          <div className="mem-lite-panel-head">
            <div><h2>经验卡片</h2><p>先生成候选和相似检查，再确认写入 cards/。</p></div>
            <span className={`mem-lite-health ${cardCapture?.warnings?.length ? 'warn' : 'ok'}`}>{cardCapture ? '候选已生成' : '待捕获'}</span>
          </div>
          <form className="mem-card-form" onSubmit={(event) => void captureCard(event)}>
            <label><span>标题</span><input value={cardTitle} onChange={(event) => setCardTitle(event.target.value)} placeholder="例如：MemoryDock BGE-M3 标准入口" /></label>
            <label><span>项目</span><input value={cardProject} onChange={(event) => setCardProject(event.target.value)} /></label>
            <label><span>类型</span><select value={cardType} onChange={(event) => setCardType(event.target.value)}><option value="runbook">runbook</option><option value="bug_pattern">bug_pattern</option><option value="deploy_note">deploy_note</option><option value="project_trap">project_trap</option><option value="architecture">architecture</option><option value="decision">decision</option><option value="anti_pattern">anti_pattern</option><option value="preference">preference</option></select></label>
            <label><span>来源</span><input value={cardSource} onChange={(event) => setCardSource(event.target.value)} /></label>
            <label className="wide"><span>内容</span><textarea value={cardContent} onChange={(event) => setCardContent(event.target.value)} placeholder="写可复用结论，不写临时日志。" /></label>
            <label><span>证据</span><input value={cardEvidence} onChange={(event) => setCardEvidence(event.target.value)} placeholder="命令、端点、commit 或验证结果" /></label>
            <label><span>标签</span><input value={cardTags} onChange={(event) => setCardTags(event.target.value)} placeholder="逗号分隔" /></label>
            <label className="wide"><span>目标路径</span><input value={cardPath} onChange={(event) => setCardPath(event.target.value)} placeholder="留空时自动生成 cards/<project>/inbox/<type>/..." /></label>
            <div className="mem-card-actions">
              <button type="submit" disabled={busy}><Sparkles size={15} />生成候选</button>
              <button type="button" className="primary" onClick={() => void writeCard()} disabled={busy || !cardCapture}>确认写入</button>
              {cardCapture?.warnings?.length ? <label className="mem-card-check"><input type="checkbox" checked={allowCardWarnings} onChange={(event) => setAllowCardWarnings(event.target.checked)} />已人工确认警告</label> : null}
            </div>
          </form>
          {cardCapture && <div className="mem-card-review"><strong>{cardCapture.card.path}</strong>{cardCapture.warnings?.map((warning) => <p key={warning} className="warn">{warning}</p>)}{(cardCapture.similar_results || []).map((item) => <button key={item.path} type="button" onClick={() => void openMemory(item.path)}><FileText size={14} /><span>{item.title || nameOf(item.path)}</span><em>{item.path}</em></button>)}</div>}
        </article>

        <article className="mem-card-panel">
          <div className="mem-lite-panel-head">
            <div><h2>向量记忆</h2><p>只搜索 cards/，用于高信噪比经验召回。</p></div>
            <span className={`mem-lite-health ${embeddingStatus?.reachable ? 'ok' : 'warn'}`}>{embeddingStatus?.reachable ? 'BGE-M3 可达' : '未就绪'}</span>
          </div>
          <div className="mem-card-embed-status">
            <div><span>模型</span><strong>{embeddingStatus?.model || '未知'}</strong></div>
            <div><span>维度</span><strong>{String(embeddingStatus?.dimension || '—')}</strong></div>
            <div><span>索引</span><strong>{String(embeddingStatus?.count || '—')}</strong></div>
          </div>
          <form className="mem-card-search" onSubmit={(event) => void searchCardEmbeddings(event)}>
            <Search size={15} />
            <input value={embeddingQuery} onChange={(event) => setEmbeddingQuery(event.target.value)} placeholder="搜索经验卡片" />
            <button type="submit" disabled={busy}>向量搜索</button>
            <button type="button" onClick={() => void reindexCards()} disabled={busy}><Cpu size={15} />重建索引</button>
          </form>
          <div className="mem-card-results">
            {embeddingResults.length === 0 ? <p className="mem-lite-empty">暂无向量搜索结果。</p> : embeddingResults.map((item) => <button key={item.path} type="button" onClick={() => void openMemory(item.path)}><strong>{item.title || nameOf(item.path)}</strong><small>{item.path}</small><em>{item.score.toFixed(4)}</em></button>)}
          </div>
        </article>
      </section>

      <section className="mem-lite-grid">
        <aside className="mem-lite-browser">
          <div className="mem-lite-panel-head">
            <div><h2>文件</h2><p>{query ? '搜索结果' : '当前召回仓库'}</p></div>
            <button className="icon" onClick={startNew} title="新建召回条目"><Plus size={17} /></button>
          </div>
          <form className="mem-lite-search" onSubmit={(event) => void searchMemories(event)}>
            <Search size={15} />
            <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索召回内容" />
            <button type="submit">搜索</button>
          </form>
          <div className="mem-lite-files">
            {loading ? <p className="mem-lite-empty">正在读取召回内容…</p> : fileEntries.length === 0 ? <p className="mem-lite-empty">没有匹配的召回文件。</p> : fileEntries.map((entry) => (
              <button key={entry.path} className={current?.path === entry.path ? 'active' : ''} onClick={() => void openMemory(entry.path)}>
                <FileText size={16} />
                <span><strong>{nameOf(entry.path)}</strong><small>{entry.path}</small></span>
                <em>{formatBytes(entry.size_bytes)}</em>
              </button>
            ))}
          </div>
        </aside>

        <article className="mem-lite-editor">
          <div className="mem-lite-panel-head">
            <div><h2>{editing ? creating ? '新建召回条目' : '编辑召回条目' : current ? nameOf(current.path) : '选择一条召回内容'}</h2><p>{editing ? draftPath : current?.path || '从左侧文件列表打开，或新建一条记忆。'}</p></div>
            <div className="mem-lite-editor-actions">
              {!editing && current && <button onClick={startEdit}><Pencil size={15} />编辑</button>}
              {!editing && current && <button onClick={requestMove}>移动</button>}
              {!editing && current && <button className="danger" onClick={requestDelete}><Trash2 size={15} />删除</button>}
              {editing && <button onClick={cancelEdit}>取消</button>}
              {editing && <button className="primary" onClick={() => void saveMemory()} disabled={busy || !hasUnsavedChanges}><Save size={15} />保存</button>}
            </div>
          </div>
          {editing ? (
            <div className="mem-lite-edit-body">
              <label><span>路径</span><input value={draftPath} onChange={(event) => setDraftPath(event.target.value)} disabled={!creating} /></label>
              <label className="content"><span>内容</span><textarea value={draftContent} onChange={(event) => setDraftContent(event.target.value)} spellCheck={false} /></label>
              <small>{draftContent.length.toLocaleString()} 字符 · 草稿自动保存在当前浏览器会话</small>
            </div>
          ) : current ? (
            <pre className="mem-lite-preview">{current.content}</pre>
          ) : (
            <div className="mem-lite-empty large"><FileText size={28} /><strong>没有打开的召回内容</strong><span>选择文件或创建一条新召回条目。</span><button className="primary" onClick={startNew}><Plus size={15} />新建召回条目</button></div>
          )}
        </article>
      </section>

      <section className="mem-lite-lower">
        <article className="mem-lite-sync">
          <div className="mem-lite-panel-head"><div><h2>同步</h2><p>只提供明确的更新与保存动作</p></div></div>
          <div className="mem-lite-sync-state">
            <div><span>分支</span><strong>{String(syncStatus?.branch || '默认')}</strong></div>
            <div><span>领先</span><strong>{String(syncStatus?.ahead ?? '0')}</strong></div>
            <div><span>落后</span><strong>{String(syncStatus?.behind ?? '0')}</strong></div>
            <div><span>状态</span><strong>{dirty ? '需要同步' : '健康'}</strong></div>
          </div>
          <div className="mem-lite-sync-actions">
            <button onClick={() => void syncNow('pull')} disabled={busy}>从远端更新</button>
            <button onClick={() => void syncNow('push')} disabled={busy}>保存到远端</button>
            <button className="primary" onClick={() => void syncNow('now')} disabled={busy}>更新并保存</button>
          </div>
        </article>

        <article className="mem-lite-history">
          <div className="mem-lite-panel-head"><div><h2>最近版本</h2><p>仅展示时间线；详细 Diff 交给 Git 工具或 Agent</p></div></div>
          <div className="mem-lite-commits">
            {commits.length === 0 ? <p className="mem-lite-empty">暂无版本记录。</p> : commits.map((commit) => (
              <div key={commit.hash}>
                <span />
                <div><strong>{commit.subject || '(无说明)'}</strong><small>{commit.short_hash} · {commit.author} · {formatTime(commit.date)}</small></div>
              </div>
            ))}
          </div>
        </article>
      </section>
    </main>
  );
}
