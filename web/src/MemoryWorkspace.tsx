import { useEffect, useMemo, useState, type FormEvent } from 'react';
import {
  Archive, Check, Clock3, FileText, Folder, GitBranch, Pencil, Plus,
  RefreshCw, Save, Search, Trash2, UploadCloud,
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

const NEW_MEMORY_TEMPLATE = `---
type: note
scope: inbox
source: user-confirmed
confidence: medium
---

# 新记忆

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

export default function MemoryWorkspace() {
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
      await Promise.all([loadList(), loadSyncState(), loadHistory()]);
      if (path) await openMemory(path);
    } catch (reason) {
      setNotice({ text: messageOf(reason), danger: true });
    } finally {
      setLoading(false);
    }
  }

  async function loadList() {
    const response = await api<{ entries: MemoryEntry[] }>('/v1/memories?max_entries=500');
    setEntries(response.entries || []);
  }

  async function searchMemories(event?: FormEvent) {
    event?.preventDefault();
    if (!query.trim()) {
      await loadList();
      updateRoute(current?.path || '', '');
      return;
    }
    const response = await api<{ results: Array<{ path: string; size_bytes?: number }> }>('/v1/memories/search', {
      method: 'POST',
      body: JSON.stringify({ query: query.trim(), prefix: '', max_results: 100 }),
    });
    setEntries((response.results || []).map((item) => ({ path: item.path, name: nameOf(item.path), type: 'file', size_bytes: item.size_bytes })));
    updateRoute(current?.path || '', query.trim());
  }

  async function openMemory(path: string) {
    const response = await api<{ memory: Memory }>(`/v1/memories/${encodeURIComponent(path)}`);
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
      setNotice({ text: '记忆文件必须使用 .md、.markdown 或 .txt 扩展名', danger: true });
      return;
    }
    setBusy(true);
    try {
      const existing = Boolean(current?.path) && !creating;
      const target = existing ? `/v1/memories/${encodeURIComponent(current!.path)}` : '/v1/memories';
      const response = await api<{ memory: Memory }>(target, {
        method: existing ? 'PATCH' : 'POST',
        body: JSON.stringify({ path: existing ? current!.path : path, content: draftContent, confirmed: true, overwrite: true }),
      });
      clearMemoryDraft();
      setDraftAvailable(false);
      await Promise.all([loadList(), loadSyncState(), loadHistory()]);
      await openMemory(response.memory.path);
      setNotice({ text: '记忆已保存' });
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
      const response = await api<{ memory: Memory }>('/v1/memories/move', {
        method: 'POST',
        body: JSON.stringify({ from_path: pendingAction.path, to_path: normalized, confirmed: true, overwrite: false }),
      });
      setPendingAction(null);
      await loadList();
      await openMemory(response.memory.path);
      setNotice({ text: '记忆已移动' });
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
      await api(`/v1/memories/${encodeURIComponent(pendingAction.path)}?confirmed=true`, { method: 'DELETE' });
      setPendingAction(null);
      setCurrent(null);
      setDraftPath('');
      setDraftContent('');
      setEditing(false);
      updateRoute('', query.trim());
      await Promise.all([loadList(), loadSyncState(), loadHistory()]);
      setNotice({ text: '记忆已删除' });
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

  async function syncNow(action: 'pull' | 'push' | 'now' = 'now') {
    setBusy(true);
    try {
      const response = await api<SyncStatus>(`/v1/sync/${action}`, { method: 'POST', body: '{}' });
      setSyncStatus(response);
      await Promise.all([loadList(), loadSyncState(), loadHistory()]);
      if (current?.path) await openMemory(current.path).catch(() => setCurrent(null));
      setNotice({ text: action === 'pull' ? '已从远端更新' : action === 'push' ? '已保存到远端' : '记忆已同步' });
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
          <span className="mem-lite-kicker">NEXUS MEMORY</span>
          <h1>记忆库</h1>
          <p>浏览、编辑和同步 Markdown 记忆；复杂 Git 操作继续交给 Agent。</p>
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
          title={pendingAction.kind === 'move' ? '移动记忆' : '删除记忆'}
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

      <section className="mem-lite-grid">
        <aside className="mem-lite-browser">
          <div className="mem-lite-panel-head">
            <div><h2>文件</h2><p>{query ? '搜索结果' : '当前记忆仓库'}</p></div>
            <button className="icon" onClick={startNew} title="新建记忆"><Plus size={17} /></button>
          </div>
          <form className="mem-lite-search" onSubmit={(event) => void searchMemories(event)}>
            <Search size={15} />
            <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索记忆内容" />
            <button type="submit">搜索</button>
          </form>
          <div className="mem-lite-files">
            {loading ? <p className="mem-lite-empty">正在读取记忆…</p> : fileEntries.length === 0 ? <p className="mem-lite-empty">没有匹配的记忆文件。</p> : fileEntries.map((entry) => (
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
            <div><h2>{editing ? creating ? '新建记忆' : '编辑记忆' : current ? nameOf(current.path) : '选择一条记忆'}</h2><p>{editing ? draftPath : current?.path || '从左侧文件列表打开，或新建一条记忆。'}</p></div>
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
            <div className="mem-lite-empty large"><FileText size={28} /><strong>没有打开的记忆</strong><span>选择文件或创建一条新记忆。</span><button className="primary" onClick={startNew}><Plus size={15} />新建记忆</button></div>
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
