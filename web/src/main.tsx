import React, { useEffect, useMemo, useState } from 'react';
import { createRoot } from 'react-dom/client';
import {
  Archive,
  Braces,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  ChevronUp,
  Clock3,
  Command,
  FileText,
  Home,
  Folder,
  FolderOpen,
  GitBranch,
  Loader2,
  PenLine,
  Plus,
  RefreshCw,
  Save,
  Search,
  Settings,
  Trash2,
  Undo2,
  X,
} from 'lucide-react';
import './styles.css';

type Tab = 'dashboard' | 'memories' | 'git' | 'sync';
type EntryType = 'file' | 'directory';

type MemoryEntry = {
  path: string;
  name: string;
  type: EntryType;
  size_bytes?: number;
};

type Memory = {
  path: string;
  content: string;
};

type TreeNode = {
  name: string;
  path: string;
  type: EntryType;
  entry?: MemoryEntry;
  children: Map<string, TreeNode>;
};

type ChangedFile = {
  status: string;
  path: string;
};

type GitDiff = {
  ok: boolean;
  git_repo: boolean;
  dirty: boolean;
  status: string;
  stat: string;
  diff: string;
  cached_diff: string;
  files?: ChangedFile[];
};

type GitCommit = {
  hash: string;
  short_hash: string;
  date: string;
  author: string;
  subject: string;
};

type CommitFile = {
  status: string;
  path: string;
};

type CommitDetail = {
  ok: boolean;
  git_repo: boolean;
  commit: GitCommit;
  files: CommitFile[];
  stat: string;
  diff: string;
};

type SyncStatus = Record<string, unknown> & {
  dirty?: boolean;
  ahead?: string;
  behind?: string;
  pending_push?: boolean;
};

type Toast = { message: string; danger?: boolean } | null;

const TEXT_EXTENSIONS = /\.(md|markdown|txt)$/i;
const MARKDOWN_EXTENSIONS = /\.(md|markdown)$/i;

async function api<T>(path: string, options: RequestInit = {}): Promise<T> {
  const res = await fetch(path, {
    headers: { 'Content-Type': 'application/json', ...(options.headers || {}) },
    ...options,
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok || data.ok === false) {
    throw new Error(data?.error?.message || res.statusText);
  }
  return data as T;
}

function normalizePath(path: string): string {
  return String(path || '').replace(/^\/+|\/+$/g, '').replace(/\/+/g, '/');
}

function fileName(path: string): string {
  const parts = normalizePath(path).split('/').filter(Boolean);
  return parts[parts.length - 1] || '';
}

function parentPath(path: string): string {
  const parts = normalizePath(path).split('/').filter(Boolean);
  parts.pop();
  return parts.join('/');
}

function joinPath(dir: string, name: string): string {
  dir = normalizePath(dir);
  name = fileName(name);
  return dir ? `${dir}/${name}` : name;
}

function formatBytes(bytes?: number): string {
  if (!bytes) return '';
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

function buildTree(entries: MemoryEntry[]): TreeNode {
  const root: TreeNode = { name: '', path: '', type: 'directory', children: new Map() };
  for (const entry of entries) {
    const parts = normalizePath(entry.path).split('/').filter(Boolean);
    let node = root;
    let cursor = '';
    parts.forEach((part, index) => {
      cursor = cursor ? `${cursor}/${part}` : part;
      const leaf = index === parts.length - 1;
      let child = node.children.get(part);
      if (!child) {
        child = { name: part, path: cursor, type: 'directory', children: new Map() };
        node.children.set(part, child);
      }
      if (leaf) {
        child.type = entry.type;
        child.entry = entry;
      }
      node = child;
    });
  }
  return root;
}

function sortedChildren(node: TreeNode): TreeNode[] {
  return [...node.children.values()].sort((a, b) => {
    if (a.type !== b.type) return a.type === 'directory' ? -1 : 1;
    return a.name.localeCompare(b.name, 'zh-Hans-CN', { numeric: true, sensitivity: 'base' });
  });
}

function countFiles(node: TreeNode): number {
  if (node.type === 'file') return 1;
  return [...node.children.values()].reduce((sum, child) => sum + countFiles(child), 0);
}

function isPathInside(path: string, dir: string): boolean {
  path = normalizePath(path);
  dir = normalizePath(dir);
  return Boolean(path && dir && (path === dir || path.startsWith(`${dir}/`)));
}

function renderInlineMarkdown(input: string): string {
  let html = escapeHtml(input);
  html = html.replace(/\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)/g, '<a href="$2" target="_blank" rel="noreferrer">$1</a>');
  html = html.replace(/`([^`]+)`/g, '<code>$1</code>');
  html = html.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
  html = html.replace(/\*([^*]+)\*/g, '<em>$1</em>');
  return html;
}

function escapeHtml(input: string): string {
  return String(input).replace(/[&<>"']/g, (ch) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[ch] || ch));
}

function splitFrontmatter(content: string): { meta: string; body: string } {
  if (!content.startsWith('---\n')) return { meta: '', body: content };
  const end = content.indexOf('\n---\n', 4);
  if (end < 0) return { meta: '', body: content };
  return { meta: content.slice(4, end), body: content.slice(end + 5) };
}

function markdownToHtml(content: string): string {
  const { meta, body } = splitFrontmatter(content);
  const lines = body.replace(/\r\n/g, '\n').split('\n');
  const out: string[] = [];
  let paragraph: string[] = [];
  let list: null | { type: 'ul' | 'ol'; items: string[] } = null;
  let code: string[] | null = null;
  let quote: string[] = [];

  const flushParagraph = () => {
    if (!paragraph.length) return;
    out.push(`<p>${renderInlineMarkdown(paragraph.join(' '))}</p>`);
    paragraph = [];
  };
  const flushList = () => {
    if (!list) return;
    out.push(`<${list.type}>${list.items.map((item) => `<li>${renderInlineMarkdown(item)}</li>`).join('')}</${list.type}>`);
    list = null;
  };
  const flushQuote = () => {
    if (!quote.length) return;
    out.push(`<blockquote>${quote.map((line) => `<p>${renderInlineMarkdown(line)}</p>`).join('')}</blockquote>`);
    quote = [];
  };
  const close = () => {
    flushParagraph();
    flushList();
    flushQuote();
  };

  for (const line of lines) {
    if (line.startsWith('```')) {
      if (code) {
        out.push(`<pre><code>${escapeHtml(code.join('\n'))}</code></pre>`);
        code = null;
      } else {
        close();
        code = [];
      }
      continue;
    }
    if (code) {
      code.push(line);
      continue;
    }
    if (!line.trim()) {
      close();
      continue;
    }
    const heading = /^(#{1,6})\s+(.+)$/.exec(line);
    if (heading) {
      close();
      const level = Math.min(6, heading[1].length);
      out.push(`<h${level}>${renderInlineMarkdown(heading[2].trim())}</h${level}>`);
      continue;
    }
    if (/^---+$/.test(line.trim())) {
      close();
      out.push('<hr />');
      continue;
    }
    if (line.startsWith('>')) {
      flushParagraph();
      flushList();
      quote.push(line.replace(/^>\s?/, ''));
      continue;
    }
    const unordered = /^\s*[-*+]\s+(.+)$/.exec(line);
    const ordered = /^\s*\d+[.)]\s+(.+)$/.exec(line);
    if (unordered || ordered) {
      flushParagraph();
      flushQuote();
      const type = unordered ? 'ul' : 'ol';
      if (!list || list.type !== type) flushList();
      if (!list) list = { type, items: [] };
      list.items.push((unordered || ordered)![1]);
      continue;
    }
    paragraph.push(line.trim());
  }
  if (code) out.push(`<pre><code>${escapeHtml(code.join('\n'))}</code></pre>`);
  close();
  const metaHtml = meta.trim()
    ? `<details class="frontmatter"><summary>Frontmatter</summary><pre><code>${escapeHtml(meta.trim())}</code></pre></details>`
    : '';
  return metaHtml + (out.join('\n') || '<p class="muted">空 Markdown 文件</p>');
}

function diffFileName(line: string): string {
  const parts = line.trim().split(/\s+/);
  const b = parts.find((part) => part.startsWith('b/'));
  const a = parts.find((part) => part.startsWith('a/'));
  return (b || a || parts[parts.length - 1] || 'diff').replace(/^[ab]\//, '');
}

function parseHunkHeader(line: string): { oldLine: number; newLine: number } {
  const match = /^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@/.exec(line);
  return { oldLine: match ? Number(match[1]) : 0, newLine: match ? Number(match[3]) : 0 };
}

type DiffRow = {
  kind: 'ctx' | 'add' | 'del' | 'change' | 'meta' | 'hunk' | 'note';
  oldNo?: number;
  newNo?: number;
  left?: string;
  right?: string;
};

type DiffFile = { name: string; rows: DiffRow[] };
type DiffSection = { title: string; files: DiffFile[] };

function parseSideBySideDiff(sections: { title: string; diff: string }[]): DiffSection[] {
  const parsed: DiffSection[] = [];
  for (const section of sections.filter((s) => s.diff?.trim())) {
    const result: DiffSection = { title: section.title, files: [] };
    let file: DiffFile | null = null;
    let oldLine = 0;
    let newLine = 0;
    let pendingDeletes: { no: number; text: string }[] = [];

    const ensureFile = (name = 'diff') => {
      if (!file) {
        file = { name, rows: [] };
        result.files.push(file);
      }
      return file;
    };
    const flushDeletes = () => {
      const current = ensureFile();
      for (const item of pendingDeletes) current.rows.push({ kind: 'del', oldNo: item.no, left: item.text, right: '' });
      pendingDeletes = [];
    };

    for (const line of section.diff.split('\n')) {
      if (line.startsWith('diff --git ')) {
        if (file) flushDeletes();
        file = { name: diffFileName(line), rows: [] };
        result.files.push(file);
        continue;
      }
      const current = ensureFile();
      if (line.startsWith('@@ ')) {
        flushDeletes();
        const parsedHeader = parseHunkHeader(line);
        oldLine = parsedHeader.oldLine;
        newLine = parsedHeader.newLine;
        current.rows.push({ kind: 'hunk', left: line, right: line });
        continue;
      }
      if (line.startsWith('index ') || line.startsWith('new file mode') || line.startsWith('deleted file mode') || line.startsWith('similarity index') || line.startsWith('rename from') || line.startsWith('rename to') || line.startsWith('--- ') || line.startsWith('+++ ')) {
        flushDeletes();
        current.rows.push({ kind: 'meta', left: line, right: line });
        continue;
      }
      if (line.startsWith('\\ No newline')) {
        flushDeletes();
        current.rows.push({ kind: 'note', left: line, right: line });
        continue;
      }
      if (line.startsWith('-')) {
        pendingDeletes.push({ no: oldLine++, text: line });
        continue;
      }
      if (line.startsWith('+')) {
        const deleted = pendingDeletes.shift();
        if (deleted) current.rows.push({ kind: 'change', oldNo: deleted.no, newNo: newLine++, left: deleted.text, right: line });
        else current.rows.push({ kind: 'add', newNo: newLine++, left: '', right: line });
        continue;
      }
      flushDeletes();
      current.rows.push({ kind: 'ctx', oldNo: oldLine || undefined, newNo: newLine || undefined, left: line || ' ', right: line || ' ' });
      if (oldLine) oldLine++;
      if (newLine) newLine++;
    }
    if (file) flushDeletes();
    if (result.files.length) parsed.push(result);
  }
  return parsed;
}

function useToast() {
  const [toast, setToast] = useState<Toast>(null);
  const show = (message: string, danger = false) => {
    setToast({ message, danger });
    window.setTimeout(() => setToast(null), 3200);
  };
  return { toast, show };
}

function App() {
  const { toast, show } = useToast();
  const [tab, setTab] = useState<Tab>('memories');
  const [sidebarCollapsed, setSidebarCollapsed] = useState(localStorage.getItem('memorydock.sidebarCollapsed') === '1');
  const [explorerCollapsed, setExplorerCollapsed] = useState(localStorage.getItem('memorydock.explorerCollapsed') === '1');
  const [entries, setEntries] = useState<MemoryEntry[]>([]);
  const [expanded, setExpanded] = useState<Set<string>>(new Set(['']));
  const [current, setCurrent] = useState<Memory | null>(null);
  const [editing, setEditing] = useState(false);
  const [draftPath, setDraftPath] = useState('');
  const [draftContent, setDraftContent] = useState('');
  const [search, setSearch] = useState('');
  const [prefix, setPrefix] = useState('');
  const [loading, setLoading] = useState(false);
  const [gitDiff, setGitDiff] = useState<GitDiff | null>(null);
  const [commits, setCommits] = useState<GitCommit[]>([]);
  const [selectedCommit, setSelectedCommit] = useState<CommitDetail | null>(null);
  const [syncStatus, setSyncStatus] = useState<SyncStatus | null>(null);
  const [draggingPath, setDraggingPath] = useState('');
  const [renamingPath, setRenamingPath] = useState('');
  const [renamingValue, setRenamingValue] = useState('');
  const [commandOpen, setCommandOpen] = useState(false);
  const [mobileNavCompact, setMobileNavCompact] = useState(() => window.matchMedia('(max-width: 900px)').matches);
  const [desktopContentHover, setDesktopContentHover] = useState(false);
  const [desktopReviewFocus, setDesktopReviewFocus] = useState(false);

  const tree = useMemo(() => buildTree(entries), [entries]);
  const fileCount = entries.filter((entry) => entry.type === 'file').length;
  const dirCount = entries.filter((entry) => entry.type === 'directory').length;

  useEffect(() => {
    void loadList();
  }, []);

  useEffect(() => {
    localStorage.setItem('memorydock.sidebarCollapsed', sidebarCollapsed ? '1' : '0');
  }, [sidebarCollapsed]);

  useEffect(() => {
    localStorage.setItem('memorydock.explorerCollapsed', explorerCollapsed ? '1' : '0');
  }, [explorerCollapsed]);

  useEffect(() => {
    if (tab === 'dashboard') {
      void loadGitDiff().catch(() => undefined);
      void loadGitLog().catch(() => undefined);
      void loadSyncStatus().catch(() => undefined);
    }
    if (tab === 'git') void loadGitPanel();
    if (tab === 'sync') void loadSyncStatus();
  }, [tab]);

  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
        event.preventDefault();
        setCommandOpen((open) => !open);
      }
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, []);

  useEffect(() => {
    const syncMobileDock = () => {
      const mobile = window.matchMedia('(max-width: 900px)').matches;
      if (mobile) setMobileNavCompact(true);
      else setMobileNavCompact(false);
    };
    syncMobileDock();
    window.addEventListener('resize', syncMobileDock);
    return () => window.removeEventListener('resize', syncMobileDock);
  }, []);

  function viewChange(update: () => void) {
    const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    const doc = document as Document & { startViewTransition?: (callback: () => void) => void };
    if (reduceMotion || typeof doc.startViewTransition !== 'function') {
      update();
      return;
    }
    doc.startViewTransition(() => update());
  }

  function selectTab(next: Tab) {
    viewChange(() => {
      setMobileNavCompact(window.matchMedia('(max-width: 900px)').matches);
      setDesktopContentHover(false);
      setDesktopReviewFocus(false);
      setTab(next);
    });
  }

  function setContentHover(next: boolean) {
    const desktopPointer = window.matchMedia('(min-width: 901px) and (pointer: fine)').matches;
    setDesktopContentHover(desktopPointer && next);
  }

  function setReviewFocus(next: boolean) {
    const desktopPointer = window.matchMedia('(min-width: 901px) and (pointer: fine)').matches;
    setDesktopReviewFocus(desktopPointer && next);
  }

  function expandPath(path: string) {
    const parts = normalizePath(path).split('/').filter(Boolean);
    setExpanded((prev) => {
      const next = new Set(prev);
      let cursor = '';
      for (let i = 0; i < parts.length - 1; i++) {
        cursor = cursor ? `${cursor}/${parts[i]}` : parts[i];
        next.add(cursor);
      }
      return next;
    });
  }

  async function loadList() {
    const qs = new URLSearchParams({ max_entries: '500' });
    if (prefix.trim()) qs.set('prefix', prefix.trim());
    const data = await api<{ entries: MemoryEntry[] }>(`/v1/memories?${qs}`);
    setEntries(data.entries || []);
    setExpanded((prev) => {
      const next = new Set(prev);
      const root = buildTree(data.entries || []);
      const walk = (node: TreeNode) => {
        if (node.type === 'directory') next.add(node.path);
        sortedChildren(node).forEach(walk);
      };
      sortedChildren(root).forEach(walk);
      return next;
    });
  }

  async function doSearch() {
    if (!search.trim()) return loadList();
    const data = await api<{ results: Array<{ path: string; size_bytes?: number }>; count: number }>('/v1/memories/search', {
      method: 'POST',
      body: JSON.stringify({ query: search.trim(), prefix: prefix.trim(), max_results: 100 }),
    });
    setEntries((data.results || []).map((result) => ({ path: result.path, name: fileName(result.path), type: 'file', size_bytes: result.size_bytes || 0 })));
  }

  async function loadMemory(path: string) {
    const data = await api<{ memory: Memory }>(`/v1/memories/${encodeURIComponent(path)}`);
    viewChange(() => {
      setCurrent(data.memory);
      setDraftPath(data.memory.path);
      setDraftContent(data.memory.content);
      setEditing(false);
      expandPath(data.memory.path);
    });
  }

  function newMemory() {
    const template = '---\ntype: note\nscope: inbox\nsource: user-confirmed\nconfidence: medium\n---\n\n# 新记忆\n\n';
    setCurrent(null);
    setDraftPath('inbox/new-memory.md');
    setDraftContent(template);
    setEditing(true);
  }

  async function saveMemory() {
    if (!draftPath.trim() || !draftContent.trim()) return show('path 和 content 不能为空', true);
    const existing = Boolean(current?.path);
    const target = existing ? `/v1/memories/${encodeURIComponent(current!.path)}` : '/v1/memories';
    const data = await api<{ memory: Memory }>(target, {
      method: existing ? 'PATCH' : 'POST',
      body: JSON.stringify({ path: existing ? current!.path : draftPath.trim(), content: draftContent, confirmed: true, overwrite: true }),
    });
    show('已保存');
    await loadList();
    await loadMemory(data.memory.path);
  }

  async function deleteCurrent() {
    if (!current?.path) return;
    if (!confirm(`确认删除：${current.path} ?`)) return;
    await api(`/v1/memories/${encodeURIComponent(current.path)}?confirmed=true`, { method: 'DELETE' });
    setCurrent(null);
    setEditing(false);
    setDraftPath('');
    setDraftContent('');
    show('已删除');
    await loadList();
  }

  async function moveToDirectory(fromPath: string, dirPath: string) {
    fromPath = normalizePath(fromPath);
    const toPath = joinPath(dirPath, fromPath);
    if (!fromPath || toPath === fromPath) return;
    if (!confirm(`移动文件？\n\n${fromPath}\n→ ${toPath}`)) return;
    const data = await api<{ memory: Memory }>('/v1/memories/move', {
      method: 'POST',
      body: JSON.stringify({ from_path: fromPath, to_path: toPath, confirmed: true, overwrite: false }),
    });
    show(`已移动到 ${toPath}`);
    await loadList();
    await loadMemory(data.memory.path);
  }

  function renameNode(node: TreeNode) {
    setRenamingPath(node.path);
    setRenamingValue(node.name);
  }

  function cancelRename() {
    setRenamingPath('');
    setRenamingValue('');
  }

  async function commitRename(node: TreeNode, nextName: string) {
    const oldPath = normalizePath(node.path);
    const trimmed = nextName.trim();
    if (trimmed === node.name) return cancelRename();
    if (!trimmed || trimmed.includes('/') || trimmed.includes('\\') || trimmed.startsWith('.')) {
      setRenamingValue(node.name);
      show('名称不能为空，且不能包含 /、\\ 或以 . 开头', true);
      return;
    }
    const newPath = joinPath(parentPath(oldPath), trimmed);
    if (node.type === 'file' && !TEXT_EXTENSIONS.test(newPath)) {
      setRenamingValue(node.name);
      show('文件名需要以 .md、.markdown 或 .txt 结尾', true);
      return;
    }
    await api('/v1/memories/move', { method: 'POST', body: JSON.stringify({ from_path: oldPath, to_path: newPath, confirmed: true, overwrite: false }) });
    cancelRename();
    show(`已重命名为 ${newPath}`);
    const nextCurrent = current?.path && isPathInside(current.path, oldPath) ? current.path.replace(oldPath, newPath) : current?.path;
    await loadList();
    if (nextCurrent) await loadMemory(nextCurrent).catch(() => setCurrent(null));
  }

  async function deleteNode(node: TreeNode) {
    const path = normalizePath(node.path);
    const message = node.type === 'directory' ? `确认递归删除整个文件夹？\n\n${path}\n\n其中的所有文件都会被删除。` : `确认删除文件？\n\n${path}`;
    if (!confirm(message)) return;
    await api(`/v1/memories/${encodeURIComponent(path)}?confirmed=true`, { method: 'DELETE' });
    if (current?.path && (node.type === 'directory' ? isPathInside(current.path, path) : current.path === path)) setCurrent(null);
    show(node.type === 'directory' ? `已删除文件夹 ${path}` : `已删除文件 ${path}`);
    await loadList();
  }

  async function loadGitPanel() {
    await Promise.all([loadGitDiff(), loadGitLog()]);
  }

  async function loadGitDiff() {
    const data = await api<GitDiff>('/v1/git/diff');
    setGitDiff(data);
  }

  async function loadGitLog() {
    const data = await api<{ commits: GitCommit[] }>('/v1/git/log?limit=50');
    setCommits(data.commits || []);
  }

  async function loadCommitDetail(hash: string) {
    const data = await api<CommitDetail>(`/v1/git/commit?hash=${encodeURIComponent(hash)}`);
    setSelectedCommit(data);
  }

  function openGitFile(path: string) {
    selectTab('memories');
    void loadMemory(path)
      .then(() => setEditing(true))
      .catch((e) => show(e.message, true));
  }

  async function discardGitChanges(path = '') {
    const target = path ? `文件：${path}` : '全部未提交变更';
    if (!confirm(`确认丢弃 ${target}？\n\n这个操作不可撤销。`)) return;
    await api('/v1/git/discard', { method: 'POST', body: JSON.stringify({ path, confirmed: true }) });
    show(path ? `已丢弃 ${path} 的变更` : '已丢弃全部未提交变更');
    await Promise.all([loadGitDiff(), loadList().catch(() => undefined), loadSyncStatus().catch(() => undefined)]);
    if (current?.path) await loadMemory(current.path).catch(() => setCurrent(null));
  }

  async function loadSyncStatus() {
    const data = await api<SyncStatus>('/v1/sync/status');
    setSyncStatus(data);
  }

  async function syncAction(action: 'pull' | 'push' | 'now') {
    const data = await api<SyncStatus>(`/v1/sync/${action}`, { method: 'POST' });
    setSyncStatus(data);
    show('同步操作完成');
  }

  return (
    <div className={`app ${sidebarCollapsed ? 'sidebar-collapsed' : ''} ${mobileNavCompact ? 'mobile-nav-orb' : ''} ${desktopContentHover ? 'desktop-content-orb' : ''} ${desktopReviewFocus ? 'desktop-review-focus' : ''}`}>
      <AppSidebar collapsed={sidebarCollapsed} tab={tab} setTab={selectTab} onToggle={() => setSidebarCollapsed((v) => !v)} mobileCompact={mobileNavCompact} onCompactOpen={() => { setMobileNavCompact(false); setDesktopContentHover(false); }} />
      <section className="workspace">
        <Topbar tab={tab} current={current} fileCount={fileCount} dirCount={dirCount} onCommand={() => setCommandOpen(true)} />
        {tab === 'dashboard' && (
          <Dashboard
            entries={entries}
            current={current}
            diff={gitDiff}
            commits={commits}
            syncStatus={syncStatus}
            onNew={() => { selectTab('memories'); newMemory(); }}
            onOpen={(path) => { selectTab('memories'); void loadMemory(path).catch((e) => show(e.message, true)); }}
            onReview={() => selectTab('git')}
            onSync={() => selectTab('sync')}
          />
        )}
        {tab === 'memories' && (
          <section className={`memory-layout ${explorerCollapsed ? 'explorer-collapsed' : ''}`}>
            <Explorer
              tree={tree}
              expanded={expanded}
              setExpanded={setExpanded}
              currentPath={current?.path || ''}
              fileCount={fileCount}
              dirCount={dirCount}
              search={search}
              prefix={prefix}
              setSearch={setSearch}
              setPrefix={setPrefix}
              onSearch={() => void doSearch().catch((e) => show(e.message, true))}
              onRefresh={() => void loadList().catch((e) => show(e.message, true))}
              onOpen={(path) => void loadMemory(path).catch((e) => show(e.message, true))}
              onRename={renameNode}
              onRenameCommit={(node, value) => void commitRename(node, value).catch((e) => show(e.message, true))}
              onRenameCancel={cancelRename}
              renamingPath={renamingPath}
              renamingValue={renamingValue}
              setRenamingValue={setRenamingValue}
              onDelete={(node) => void deleteNode(node).catch((e) => show(e.message, true))}
              draggingPath={draggingPath}
              setDraggingPath={setDraggingPath}
              onMove={(from, to) => void moveToDirectory(from, to).catch((e) => show(e.message, true))}
              collapsed={explorerCollapsed}
              onToggle={() => setExplorerCollapsed((v) => !v)}
            />
            <MemoryEditor
              current={current}
              editing={editing}
              draftPath={draftPath}
              draftContent={draftContent}
              setDraftPath={setDraftPath}
              setDraftContent={setDraftContent}
              onNew={newMemory}
              onEdit={() => viewChange(() => setEditing(true))}
              onCancel={() => viewChange(() => {
                setEditing(false);
                setDraftPath(current?.path || '');
                setDraftContent(current?.content || '');
              })}
              onSave={() => void saveMemory().catch((e) => show(e.message, true))}
              onDelete={() => void deleteCurrent().catch((e) => show(e.message, true))}
              onContentHover={setContentHover}
            />
          </section>
        )}
        {tab === 'git' && <GitView diff={gitDiff} commits={commits} selectedCommit={selectedCommit} onRefresh={loadGitPanel} onDiscard={discardGitChanges} onOpenFile={openGitFile} onSelectCommit={loadCommitDetail} onFocusDiff={setReviewFocus} />}
        {tab === 'sync' && <SyncView status={syncStatus} onRefresh={loadSyncStatus} onAction={syncAction} />}
      </section>
      {commandOpen && (
        <CommandPalette
          entries={entries}
          current={current}
          onClose={() => setCommandOpen(false)}
          onNew={() => { setCommandOpen(false); selectTab('memories'); newMemory(); }}
          onOpen={(path) => { setCommandOpen(false); selectTab('memories'); void loadMemory(path).catch((e) => show(e.message, true)); }}
          onTab={(next) => { setCommandOpen(false); selectTab(next); }}
          onSync={() => { setCommandOpen(false); selectTab('sync'); void syncAction('now').catch((e) => show(e.message, true)); }}
        />
      )}
      {toast && <div className={`toast ${toast.danger ? 'danger' : ''}`}>{toast.message}</div>}
    </div>
  );
}


function recentFiles(entries: MemoryEntry[], limit = 8): MemoryEntry[] {
  return entries.filter((entry) => entry.type === 'file').slice(0, limit);
}

function Dashboard({ entries, current, diff, commits, syncStatus, onNew, onOpen, onReview, onSync }: {
  entries: MemoryEntry[];
  current: Memory | null;
  diff: GitDiff | null;
  commits: GitCommit[];
  syncStatus: SyncStatus | null;
  onNew: () => void;
  onOpen: (path: string) => void;
  onReview: () => void;
  onSync: () => void;
}) {
  const files = entries.filter((entry) => entry.type === 'file');
  const dirs = entries.filter((entry) => entry.type === 'directory');
  const recent = recentFiles(entries);
  return (
    <section className="dashboard-grid">
      <div className="dashboard-hero panel-card">
        <div>
          <span className="eyebrow">MemoryDock</span>
          <h2>记忆、变更和同步，一屏掌控</h2>
          <p>先看状态，再决定是继续写、审阅本地变更，还是保存到远程。</p>
        </div>
        <div className="hero-actions">
          <button className="primary" onClick={onNew}><Plus size={16} />新建记忆</button>
          <button onClick={onReview}><GitBranch size={16} />审阅变更</button>
          <button onClick={onSync}><RefreshCw size={16} />同步中心</button>
        </div>
      </div>
      <div className="metric-card panel-card"><span>记忆文件</span><strong>{files.length}</strong><p>可搜索的知识资产</p></div>
      <div className="metric-card panel-card"><span>目录</span><strong>{dirs.length}</strong><p>按项目和主题整理</p></div>
      <div className="metric-card panel-card"><span>本地变更</span><strong>{diff?.dirty ? '待审阅' : '干净'}</strong><p>{diff?.dirty ? '先检查再保存' : '没有未保存更改'}</p></div>
      <div className="panel-card dashboard-section">
        <div className="card-head compact"><div><h3>最近记忆</h3><p>快速回到最近的文件</p></div><FileText size={18} /></div>
        <div className="recent-list">
          {recent.length ? recent.map((entry) => <button key={entry.path} onClick={() => onOpen(entry.path)}><FileText size={15} /><span>{entry.path}</span><small>{formatBytes(entry.size_bytes)}</small></button>) : <div className="empty-state">暂无文件</div>}
        </div>
      </div>
      <div className="panel-card dashboard-section">
        <div className="card-head compact"><div><h3>同步健康</h3><p>一眼判断是否安全</p></div><Settings size={18} /></div>
        <SyncHealth status={syncStatus} diff={diff} />
      </div>
      <div className="panel-card dashboard-section wide">
        <div className="card-head compact"><div><h3>版本历史</h3><p>最近保存到远程的记录</p></div><Clock3 size={18} /></div>
        <div className="commit-list compact-list">
          {commits.slice(0, 5).map((commit) => <div className="commit" key={commit.hash}><div><strong>{commit.subject || '(no subject)'}</strong><span>{commit.short_hash}</span></div><p>{[commit.author, commit.date].filter(Boolean).join(' · ')}</p></div>)}
          {!commits.length && <div className="empty-state">暂无版本历史</div>}
        </div>
      </div>
      {current && <div className="panel-card dashboard-section wide"><div className="card-head compact"><div><h3>当前打开</h3><p>{current.path}</p></div></div><article className="mini-preview" dangerouslySetInnerHTML={{ __html: MARKDOWN_EXTENSIONS.test(current.path) ? markdownToHtml(current.content) : escapeHtml(current.content) }} /></div>}
    </section>
  );
}

function SyncHealth({ status, diff }: { status: SyncStatus | null; diff: GitDiff | null }) {
  const items = [
    { label: '本地更改', value: diff?.dirty ? '有' : '无', tone: diff?.dirty ? 'warn' : 'ok' },
    { label: '待保存', value: status?.pending_push ? '是' : '否', tone: status?.pending_push ? 'warn' : 'ok' },
    { label: 'Ahead', value: String(status?.ahead ?? '0'), tone: String(status?.ahead ?? '0') !== '0' ? 'warn' : 'ok' },
    { label: 'Behind', value: String(status?.behind ?? '0'), tone: String(status?.behind ?? '0') !== '0' ? 'warn' : 'ok' },
  ];
  return <div className="health-grid">{items.map((item) => <div className={`health-item ${item.tone}`} key={item.label}><span>{item.label}</span><strong>{item.value}</strong></div>)}</div>;
}


function CommandPalette({ entries, current, onClose, onNew, onOpen, onTab, onSync }: {
  entries: MemoryEntry[];
  current: Memory | null;
  onClose: () => void;
  onNew: () => void;
  onOpen: (path: string) => void;
  onTab: (tab: Tab) => void;
  onSync: () => void;
}) {
  const [query, setQuery] = useState('');

  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [onClose]);

  const normalized = query.trim().toLowerCase();
  const files = entries
    .filter((entry) => entry.type === 'file')
    .filter((entry) => !normalized || entry.path.toLowerCase().includes(normalized))
    .slice(0, 8);
  const actions = [
    { label: '打开工作台', hint: 'Dashboard', icon: <Home size={16} />, run: () => onTab('dashboard') },
    { label: '新建记忆', hint: 'Create note', icon: <Plus size={16} />, run: onNew },
    { label: '打开记忆库', hint: 'Explorer', icon: <Archive size={16} />, run: () => onTab('memories') },
    { label: '打开变更审阅', hint: 'Review local changes', icon: <GitBranch size={16} />, run: () => onTab('git') },
    { label: '打开同步中心', hint: 'Sync status', icon: <Settings size={16} />, run: () => onTab('sync') },
    { label: '立即更新并保存', hint: 'Pull + push', icon: <RefreshCw size={16} />, run: onSync },
  ].filter((item) => !normalized || item.label.toLowerCase().includes(normalized) || item.hint.toLowerCase().includes(normalized));

  return (
    <div className="command-overlay" onMouseDown={onClose}>
      <div className="command-panel" onMouseDown={(event) => event.stopPropagation()}>
        <div className="command-search">
          <Command size={17} />
          <input autoFocus value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索命令或文件…" />
        </div>
        <div className="command-group">
          <span>命令</span>
          {actions.map((item) => <button key={item.label} onClick={item.run}>{item.icon}<strong>{item.label}</strong><small>{item.hint}</small></button>)}
        </div>
        <div className="command-group">
          <span>文件</span>
          {files.map((entry) => <button key={entry.path} onClick={() => onOpen(entry.path)}><FileText size={16} /><strong>{entry.path}</strong><small>{formatBytes(entry.size_bytes)}</small></button>)}
          {!files.length && <p className="muted command-empty">没有匹配文件{current ? ` · 当前：${current.path}` : ''}</p>}
        </div>
      </div>
    </div>
  );
}

function AppSidebar({ collapsed, tab, setTab, onToggle, mobileCompact = false, onCompactOpen }: { collapsed: boolean; tab: Tab; setTab: (tab: Tab) => void; onToggle: () => void; mobileCompact?: boolean; onCompactOpen?: () => void }) {
  return (
    <aside className={`sidebar ${mobileCompact ? 'compact-orb' : ''}`} onClick={() => { if (mobileCompact) onCompactOpen?.(); }}>
      <div className="brand">
        <div className="brand-mark">M</div>
        {!collapsed && (
          <div className="brand-text">
            <h1>MemoryDock</h1>
            <p>Knowledge workspace</p>
          </div>
        )}
        <button className="icon-button sidebar-toggle" onClick={onToggle} title={collapsed ? '展开侧栏' : '折叠侧栏'}>
          {collapsed ? <ChevronRight size={16} /> : <ChevronLeft size={16} />}
        </button>
      </div>
      <nav className="nav">
        <button className={tab === 'dashboard' ? 'active' : ''} onClick={() => setTab('dashboard')} title="工作台">
          <Home size={17} /> {!collapsed && <span>工作台</span>}
        </button>
        <button className={tab === 'memories' ? 'active' : ''} onClick={() => setTab('memories')} title="记忆库">
          <Archive size={17} /> {!collapsed && <span>记忆库</span>}
        </button>
        <button className={tab === 'git' ? 'active' : ''} onClick={() => setTab('git')} title="变更审阅">
          <GitBranch size={17} /> {!collapsed && <span>变更审阅</span>}
        </button>
        <button className={tab === 'sync' ? 'active' : ''} onClick={() => setTab('sync')} title="同步设置">
          <Settings size={17} /> {!collapsed && <span>同步设置</span>}
        </button>
      </nav>
      {!collapsed && (
        <div className="sidebar-card">
          <strong>Git backed memory</strong>
          <span>Markdown 记忆库 · Git 审阅 · 目录整理 · 同步发布</span>
        </div>
      )}
    </aside>
  );
}

function Topbar({ tab, current, fileCount, dirCount, onCommand }: { tab: Tab; current: Memory | null; fileCount: number; dirCount: number; onCommand: () => void }) {
  const title = tab === 'dashboard' ? '记忆工作台' : tab === 'memories' ? 'Memory workspace' : tab === 'git' ? '变更审阅' : 'Sync center';
  const subtitle = tab === 'dashboard' ? '今日状态、最近记忆、同步健康和快速入口' : tab === 'memories' ? current?.path || '浏览、整理、编辑和审阅你的记忆文件' : tab === 'git' ? '像代码评审一样查看和放弃本地更改' : '查看同步状态并手动触发保存到远程';
  return (
    <header className="topbar">
      <div className="page-title">
        <h2>{title}</h2>
        <p>{subtitle}</p>
      </div>
      <div className="status-strip">
        <button className="command-button" onClick={onCommand}><Command size={14} />⌘K</button>
        <span className="pill ok">● Online</span>
        <span className="pill">{fileCount} files</span>
        <span className="pill">{dirCount} dirs</span>
      </div>
    </header>
  );
}

function Explorer(props: {
  tree: TreeNode;
  expanded: Set<string>;
  setExpanded: React.Dispatch<React.SetStateAction<Set<string>>>;
  currentPath: string;
  fileCount: number;
  dirCount: number;
  search: string;
  prefix: string;
  setSearch: (value: string) => void;
  setPrefix: (value: string) => void;
  onSearch: () => void;
  onRefresh: () => void;
  onOpen: (path: string) => void;
  onRename: (node: TreeNode) => void;
  onRenameCommit: (node: TreeNode, value: string) => void;
  onRenameCancel: () => void;
  renamingPath: string;
  renamingValue: string;
  setRenamingValue: (value: string) => void;
  onDelete: (node: TreeNode) => void;
  draggingPath: string;
  setDraggingPath: (path: string) => void;
  onMove: (from: string, to: string) => void;
  collapsed: boolean;
  onToggle: () => void;
}) {
  const compactPath = normalizePath(props.currentPath || props.prefix).split('/').filter(Boolean);
  const compactItems = compactPath.map((part, index) => ({
    name: part,
    path: compactPath.slice(0, index + 1).join('/'),
    isFile: index === compactPath.length - 1 && /\.[^/]+$/.test(part),
  }));
  return (
    <aside className={`explorer-panel ${props.collapsed ? 'explorer-orb-rail' : ''}`}>
      <div className="panel-head">
        <button className="icon-button" onClick={props.onToggle} title={props.collapsed ? '展开 Explorer' : '折叠 Explorer'}>
          {props.collapsed ? <ChevronRight size={16} /> : <ChevronLeft size={16} />}
        </button>
        {!props.collapsed && <h3>Explorer</h3>}
        {!props.collapsed && <span className="badge">{props.dirCount} 目录 · {props.fileCount} 文件</span>}
      </div>
      {props.collapsed && (
        <div className="path-orb-stack" aria-label="当前路径快捷球">
          <button className="path-orb root" title="展开 Explorer" onClick={props.onToggle}><Folder size={18} /></button>
          {compactItems.length ? compactItems.map((item) => (
            <button
              key={item.path}
              className={`path-orb ${item.isFile ? 'file' : 'folder'}`}
              title={item.path}
              onClick={() => {
                if (item.isFile) props.onOpen(item.path);
                else {
                  props.setPrefix(item.path);
                  props.onToggle();
                }
              }}
            >
              {item.isFile ? <FileText size={17} /> : <Folder size={17} />}
              <span>{item.name.replace(/\.(md|markdown|txt)$/i, '')}</span>
            </button>
          )) : <button className="path-orb empty" title="当前没有选中文件" onClick={props.onToggle}><Archive size={18} /></button>}
        </div>
      )}
      {!props.collapsed && (
        <>
          <div className="panel-search">
            <div className="input-row"><Search size={15} /><input value={props.search} onChange={(e) => props.setSearch(e.target.value)} onKeyDown={(e) => e.key === 'Enter' && props.onSearch()} placeholder="搜索关键词" /></div>
            <div className="input-row"><Folder size={15} /><input value={props.prefix} onChange={(e) => props.setPrefix(e.target.value)} placeholder="prefix，例如 shared/projects" /></div>
            <div className="button-row"><button className="primary" onClick={props.onSearch}>搜索</button><button onClick={props.onRefresh}><RefreshCw size={14} />刷新</button></div>
          </div>
          <div className="tree-scroll">
            {sortedChildren(props.tree).length ? sortedChildren(props.tree).map((node) => (
              <TreeItem
                key={node.path}
                node={node}
                depth={0}
                expanded={props.expanded}
                setExpanded={props.setExpanded}
                currentPath={props.currentPath}
                onOpen={props.onOpen}
                onRename={props.onRename}
                onRenameCommit={props.onRenameCommit}
                onRenameCancel={props.onRenameCancel}
                renamingPath={props.renamingPath}
                renamingValue={props.renamingValue}
                setRenamingValue={props.setRenamingValue}
                onDelete={props.onDelete}
                draggingPath={props.draggingPath}
                setDraggingPath={props.setDraggingPath}
                onMove={props.onMove}
              />
            )) : <div className="empty-state">没有记忆文件</div>}
          </div>
        </>
      )}
    </aside>
  );
}

function TreeItem({ node, depth, expanded, setExpanded, currentPath, onOpen, onRename, onRenameCommit, onRenameCancel, renamingPath, renamingValue, setRenamingValue, onDelete, draggingPath, setDraggingPath, onMove }: {
  node: TreeNode;
  depth: number;
  expanded: Set<string>;
  setExpanded: React.Dispatch<React.SetStateAction<Set<string>>>;
  currentPath: string;
  onOpen: (path: string) => void;
  onRename: (node: TreeNode) => void;
  onRenameCommit: (node: TreeNode, value: string) => void;
  onRenameCancel: () => void;
  renamingPath: string;
  renamingValue: string;
  setRenamingValue: (value: string) => void;
  onDelete: (node: TreeNode) => void;
  draggingPath: string;
  setDraggingPath: (path: string) => void;
  onMove: (from: string, to: string) => void;
}) {
  const open = expanded.has(node.path);
  const active = node.type === 'file' && node.path === currentPath;
  const isDir = node.type === 'directory';
  const renaming = renamingPath === node.path;
  const toggle = () => setExpanded((prev) => {
    const next = new Set(prev);
    open ? next.delete(node.path) : next.add(node.path);
    return next;
  });
  return (
    <>
      <div
        className={`tree-row ${isDir ? 'dir' : 'file'} ${active ? 'active' : ''} ${renaming ? 'renaming' : ''}`}
        style={{ paddingLeft: 8 + depth * 14 }}
        draggable={!isDir && !renaming}
        onDragStart={(e) => { if (!isDir) { setDraggingPath(node.path); e.dataTransfer.setData('text/plain', node.path); } }}
        onDragEnd={() => setDraggingPath('')}
        onDragOver={(e) => { if (isDir && draggingPath) e.preventDefault(); }}
        onDrop={(e) => { if (isDir) { e.preventDefault(); onMove(e.dataTransfer.getData('text/plain') || draggingPath, node.path); } }}
        onClick={() => renaming ? undefined : isDir ? toggle() : onOpen(node.path)}
      >
        <span className="tree-toggle">{isDir ? (open ? <ChevronDown size={13} /> : <ChevronRight size={13} />) : null}</span>
        <span className="tree-icon">{isDir ? (open ? <FolderOpen size={15} /> : <Folder size={15} />) : <FileText size={15} />}</span>
        {renaming ? (
          <input
            className="tree-rename-input"
            autoFocus
            value={renamingValue}
            onChange={(e) => setRenamingValue(e.target.value)}
            onClick={(e) => e.stopPropagation()}
            onMouseDown={(e) => e.stopPropagation()}
            onBlur={(e) => { if (e.currentTarget.dataset.cancel !== '1') onRenameCommit(node, renamingValue); }}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault();
                onRenameCommit(node, renamingValue);
              }
              if (e.key === 'Escape') {
                e.currentTarget.dataset.cancel = '1';
                onRenameCancel();
              }
            }}
          />
        ) : <span className="tree-name" title={node.path}>{node.name}</span>}
        <span className="tree-meta">{isDir ? `${countFiles(node)} 文件` : formatBytes(node.entry?.size_bytes)}</span>
        <button className="tree-action" onClick={(e) => { e.stopPropagation(); onRename(node); }} title="重命名"><PenLine size={13} /></button>
        <button className="tree-action danger" onClick={(e) => { e.stopPropagation(); onDelete(node); }} title="删除"><Trash2 size={13} /></button>
      </div>
      {isDir && open && sortedChildren(node).map((child) => <TreeItem key={child.path} node={child} depth={depth + 1} expanded={expanded} setExpanded={setExpanded} currentPath={currentPath} onOpen={onOpen} onRename={onRename} onRenameCommit={onRenameCommit} onRenameCancel={onRenameCancel} renamingPath={renamingPath} renamingValue={renamingValue} setRenamingValue={setRenamingValue} onDelete={onDelete} draggingPath={draggingPath} setDraggingPath={setDraggingPath} onMove={onMove} />)}
    </>
  );
}

function MemoryEditor(props: {
  current: Memory | null;
  editing: boolean;
  draftPath: string;
  draftContent: string;
  setDraftPath: (value: string) => void;
  setDraftContent: (value: string) => void;
  onNew: () => void;
  onEdit: () => void;
  onCancel: () => void;
  onSave: () => void;
  onDelete: () => void;
  onContentHover: (hovering: boolean) => void;
}) {
  const isMarkdown = MARKDOWN_EXTENSIONS.test(props.current?.path || props.draftPath);
  return (
    <main className="document-panel">
      <div className="doc-toolbar">
        <div>
          <div className="doc-path">{props.current?.path || (props.editing ? '新建记忆' : '未选择文件')}</div>
          <div className="muted">{props.editing ? '编辑草稿 · 左写右预览 · 保存后进入本地变更' : '阅读模式 · Markdown 自动渲染'}</div>
        </div>
        <div className="toolbar-actions">
          <button onClick={props.onNew}><Plus size={15} />新建</button>
          {!props.editing && <button disabled={!props.current} onClick={props.onEdit}><PenLine size={15} />编辑</button>}
          {props.editing && <button className="primary" onClick={props.onSave}><Save size={15} />保存</button>}
          {props.editing && <button onClick={props.onCancel}><X size={15} />取消</button>}
          <button className="danger" disabled={!props.current} onClick={props.onDelete}><Trash2 size={15} />删除</button>
        </div>
      </div>
      {props.editing ? (
        <div className="editor-body split-editor" onMouseEnter={() => props.onContentHover(true)} onMouseLeave={() => props.onContentHover(false)}>
          <div className="editor-pane">
            <div className="pane-title"><span>编辑</span><small>{props.draftPath || '未命名'}</small></div>
            <input value={props.draftPath} onChange={(e) => props.setDraftPath(e.target.value)} placeholder="memory-relative path，例如 inbox/note.md" />
            <textarea value={props.draftContent} onChange={(e) => props.setDraftContent(e.target.value)} spellCheck={false} />
          </div>
          <div className="preview-pane">
            <div className="pane-title"><span>预览</span><small>{MARKDOWN_EXTENSIONS.test(props.draftPath) ? 'Markdown' : 'Plain text'}</small></div>
            {MARKDOWN_EXTENSIONS.test(props.draftPath) ? <article className="markdown-body preview" dangerouslySetInnerHTML={{ __html: markdownToHtml(props.draftContent) }} /> : <pre className="plain-view preview">{props.draftContent}</pre>}
          </div>
        </div>
      ) : props.current ? (
        isMarkdown ? <article className="markdown-body" onMouseEnter={() => props.onContentHover(true)} onMouseLeave={() => props.onContentHover(false)} dangerouslySetInnerHTML={{ __html: markdownToHtml(props.current.content) }} /> : <pre className="plain-view" onMouseEnter={() => props.onContentHover(true)} onMouseLeave={() => props.onContentHover(false)}>{props.current.content}</pre>
      ) : (
        <div className="hero-empty">
          <FileText size={42} />
          <h3>选择一个记忆文件</h3>
          <p>从左侧 Explorer 选择 Markdown 文件，或新建一条记忆。</p>
        </div>
      )}
    </main>
  );
}

function GitView({ diff, commits, selectedCommit, onRefresh, onDiscard, onOpenFile, onSelectCommit, onFocusDiff }: {
  diff: GitDiff | null;
  commits: GitCommit[];
  selectedCommit: CommitDetail | null;
  onRefresh: () => Promise<void>;
  onDiscard: (path?: string) => Promise<void>;
  onOpenFile: (path: string) => void;
  onSelectCommit: (hash: string) => Promise<void>;
  onFocusDiff: (focus: boolean) => void;
}) {
  const sections = useMemo(() => parseSideBySideDiff([
    { title: '已暂存更改', diff: diff?.cached_diff || '' },
    { title: '工作区更改', diff: diff?.diff || '' },
  ]), [diff]);
  const commitSections = useMemo(() => parseSideBySideDiff([
    { title: selectedCommit?.commit?.short_hash ? `提交 ${selectedCommit.commit.short_hash}` : '提交详情', diff: selectedCommit?.diff || '' },
  ]), [selectedCommit]);
  const diffFiles = sections.flatMap((section) => section.files.map((file) => ({ status: 'M', path: file.name })));
  const changedFiles = (diff?.files?.length ? diff.files : diffFiles).filter((file) => Boolean(file.path));

  return (
    <section className={`git-workbench ${selectedCommit ? 'history-open' : ''}`}>
      <aside className="git-rail">
        <div className="rail-head"><strong>本地变更</strong><button className="icon-button" onClick={() => void onRefresh()} title="刷新"><RefreshCw size={14} /></button></div>
        <div className="changed-file-list">
          {changedFiles.length ? changedFiles.map((file) => (
            <button key={file.status + file.path} onClick={() => onOpenFile(file.path)}><span className="file-status">{file.status}</span><span>{file.path}</span></button>
          )) : <p className="muted rail-empty">没有本地变更</p>}
        </div>
        <div className="rail-actions">
          <button className="danger" disabled={!changedFiles.length} onClick={() => void onDiscard('')}><Undo2 size={14} />放弃全部</button>
        </div>
      </aside>

      <main className="git-main">
        <div className="git-toolbar">
          <div><h3>变更审阅</h3><p>{diff?.dirty ? '像 VS Code 一样审阅；需要修改时直接打开文件编辑。' : '没有需要审阅的本地更改'}</p></div>
          <div className="button-row"><button className="primary" onClick={() => void onRefresh()}><RefreshCw size={15} />刷新</button></div>
        </div>
        <div className="git-summary compact"><pre>{diff?.status || '工作区干净'}</pre><pre>{diff?.stat || ''}</pre></div>
        <div className="diff-viewer vscode-like" onMouseEnter={() => onFocusDiff(true)} onMouseLeave={() => onFocusDiff(false)}>
          {sections.length ? sections.map((section) => <DiffSectionView key={section.title} section={section} onDiscard={onDiscard} onOpenFile={onOpenFile} />) : changedFiles.length ? <ChangedFileCards files={changedFiles} onOpenFile={onOpenFile} onDiscard={onDiscard} /> : <div className="empty-state">没有 diff</div>}
        </div>
      </main>

      <aside className="git-history-panel">
        <div className="rail-head"><strong>版本历史</strong><Clock3 size={15} /></div>
        <div className="commit-list history-list">
          {commits.map((commit) => (
            <button className={`commit history-item ${selectedCommit?.commit?.hash === commit.hash ? 'active' : ''}`} key={commit.hash} onClick={() => void onSelectCommit(commit.hash)}>
              <div><strong>{commit.subject || '(no subject)'}</strong><span>{commit.short_hash}</span></div>
              <p>{[commit.author, commit.date].filter(Boolean).join(' · ')}</p>
            </button>
          ))}
        </div>
        {selectedCommit && (
          <div className="commit-detail">
            <div className="commit-detail-head"><strong>{selectedCommit.commit.subject}</strong><span>{selectedCommit.files.length} 文件</span></div>
            <div className="commit-files">
              {selectedCommit.files.map((file) => <button key={file.status + file.path} onClick={() => onOpenFile(file.path)}><span className="file-status">{file.status}</span><span>{file.path}</span></button>)}
            </div>
            <div className="commit-detail-diff" onMouseEnter={() => onFocusDiff(true)} onMouseLeave={() => onFocusDiff(false)}>
              {commitSections.length ? commitSections.map((section) => <DiffSectionView key={section.title} section={section} onDiscard={onDiscard} onOpenFile={onOpenFile} readonly />) : <div className="empty-state">这个提交没有可展示 diff</div>}
            </div>
          </div>
        )}
      </aside>
    </section>
  );
}

function ChangedFileCards({ files, onOpenFile, onDiscard }: { files: ChangedFile[]; onOpenFile: (path: string) => void; onDiscard: (path?: string) => Promise<void> }) {
  return (
    <div className="changed-card-grid">
      {files.map((file) => (
        <div className="changed-card" key={file.status + file.path}>
          <div>
            <span className="file-status">{file.status}</span>
            <strong>{file.path}</strong>
            <p>{file.status === '??' ? '新文件还没有进入 Git diff，但可以直接打开编辑或丢弃。' : '这个文件有本地变更，可以直接打开编辑。'}</p>
          </div>
          <div className="button-row">
            <button className="primary" onClick={() => onOpenFile(file.path)}><PenLine size={14} />打开编辑</button>
            <button className="danger" onClick={() => void onDiscard(file.path)}>丢弃</button>
          </div>
        </div>
      ))}
    </div>
  );
}

function DiffSectionView({ section, onDiscard, onOpenFile, readonly = false }: { section: DiffSection; onDiscard: (path?: string) => Promise<void>; onOpenFile: (path: string) => void; readonly?: boolean }) {
  return (
    <div className="diff-section">
      <div className="diff-stage">{section.title}</div>
      {section.files.map((file) => (
        <div className="diff-file" key={section.title + file.name}>
          <div className="diff-file-head">
            <Braces size={14} />
            <span>{file.name}</span>
            <button className="ghost" onClick={() => onOpenFile(file.name)}><PenLine size={13} />打开编辑</button>
            {!readonly && <button className="danger ghost" onClick={() => void onDiscard(file.name)}>丢弃此文件</button>}
          </div>
          {file.rows.map((row, index) => <DiffRowView key={index} row={row} />)}
        </div>
      ))}
    </div>
  );
}

function DiffRowView({ row }: { row: DiffRow }) {
  return <div className={`diff-row ${row.kind}`}><span className="ln">{row.oldNo || ''}</span><code className="left">{row.left || ' '}</code><span className="ln">{row.newNo || ''}</span><code className="right">{row.right || ' '}</code></div>;
}

function SyncCard({ label, value, tone = 'neutral' }: { label: string; value: string; tone?: 'ok' | 'warn' | 'danger' | 'neutral' }) {
  return <div className={`sync-card ${tone}`}><span>{label}</span><strong>{value}</strong></div>;
}

function SyncView({ status, onRefresh, onAction }: { status: SyncStatus | null; onRefresh: () => Promise<void>; onAction: (action: 'pull' | 'push' | 'now') => Promise<void> }) {
  const dirty = Boolean(status?.dirty);
  const pending = Boolean(status?.pending_push);
  const ahead = String(status?.ahead ?? '0');
  const behind = String(status?.behind ?? '0');
  const healthy = !dirty && !pending && ahead === '0' && behind === '0';
  return (
    <section className="sync-grid">
      <div className="panel-card sync-status-card">
        <div className="card-head">
          <div><h3>同步健康</h3><p>{healthy ? '本地和远程状态一致' : '有状态需要你确认'}</p></div>
          <button className="primary" onClick={() => void onRefresh()}><RefreshCw size={15} />刷新</button>
        </div>
        <div className="sync-card-grid">
          <SyncCard label="整体状态" value={healthy ? '健康' : '需处理'} tone={healthy ? 'ok' : 'warn'} />
          <SyncCard label="本地脏区" value={dirty ? '有更改' : '干净'} tone={dirty ? 'warn' : 'ok'} />
          <SyncCard label="待保存远程" value={pending ? '是' : '否'} tone={pending ? 'warn' : 'ok'} />
          <SyncCard label="领先远程" value={ahead} tone={ahead !== '0' ? 'warn' : 'ok'} />
          <SyncCard label="落后远程" value={behind} tone={behind !== '0' ? 'warn' : 'ok'} />
        </div>
        <details className="raw-status"><summary>查看原始状态</summary><pre className="json-view">{JSON.stringify(status || {}, null, 2)}</pre></details>
      </div>
      <div className="panel-card sync-actions-card">
        <div className="card-head"><div><h3>同步操作</h3><p>用清晰动作替代 Git 命令</p></div></div>
        <div className="sync-actions stacked">
          <button onClick={() => void onAction('pull')}><RefreshCw size={15} />从远程更新</button>
          <button onClick={() => void onAction('push')}><GitBranch size={15} />保存到远程</button>
          <button className="primary" onClick={() => void onAction('now')}><Save size={15} />更新并保存</button>
        </div>
      </div>
    </section>
  );
}

createRoot(document.getElementById('root')!).render(<App />);
