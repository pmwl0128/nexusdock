import { useEffect, useMemo, useReducer, useRef, type FormEvent } from 'react';
import { api } from '../../api/client';
import { clearRecallDraft, loadRecallDraft, saveRecallDraft } from '../../lib/drafts';
import { initialRecallState, recallReducer } from './recallState';
import type {
  CardDraft, CardCaptureResult, CardWriteResult, EmbeddingSearchResponse,
  EmbeddingStatus, GitCommit, GitDiff, Recall, RecallEntry, RecallWorkspaceViewModel, SyncStatus,
} from './types';
import {
  NEW_RECALL_TEMPLATE, csvTags, initialPath, usesSinglePaneRecallLayout,
  messageOf, nameOf, normalizePath, updateRoute,
} from './utils';

export function useRecallWorkspaceController(refreshToken: number): RecallWorkspaceViewModel {
  const [state, dispatch] = useReducer(recallReducer, undefined, initialRecallState);
  const editorRef = useRef<HTMLElement | null>(null);
  const refreshAllRef = useRef<(path?: string) => Promise<void>>(async () => undefined);
  // 搜索可能被连续触发；用序号拒绝迟到响应，确保最后一次操作获胜且不制造取消请求噪声。
  const entryRequestRef = useRef(0);
  const loadingRequestRef = useRef(0);
  const detailRequestRef = useRef(0);
  const externalRefreshTokenRef = useRef(refreshToken);
  const draftSnapshotRef = useRef({ editing: false, path: '', content: '' });

  const fileEntries = useMemo(() => {
    const files = state.entries.filter((entry) => entry.type === 'file');
    return state.appliedQuery ? files : files.sort((a, b) => a.path.localeCompare(b.path, 'zh-CN'));
  }, [state.appliedQuery, state.entries]);
  const libraryFileEntries = useMemo(
    () => state.libraryEntries.filter((entry) => entry.type === 'file').sort((a, b) => a.path.localeCompare(b.path, 'zh-CN')),
    [state.libraryEntries],
  );
  const libraryFileCount = libraryFileEntries.length;
  const directoryCount = useMemo(() => {
    const directories = new Set<string>();
    for (const entry of libraryFileEntries) {
      const parts = normalizePath(entry.path).split('/').slice(0, -1);
      let current = '';
      for (const part of parts) {
        current = current ? `${current}/${part}` : part;
        directories.add(current);
      }
    }
    return directories.size;
  }, [libraryFileEntries]);
  const changedCount = state.gitDiff?.files?.length ?? 0;
  const dirty = Boolean(state.gitDiff?.dirty || state.syncStatus?.dirty || state.syncStatus?.pending_push);
  const hasUnsavedChanges = state.editing && (state.draftPath !== (state.current?.path || '') || state.draftContent !== (state.current?.content || ''));
  const detailOpen = Boolean(state.current || state.editing);
  draftSnapshotRef.current = { editing: state.editing, path: state.draftPath, content: state.draftContent };

  useEffect(() => {
    void refreshAllRef.current(initialPath());
  }, []);

  useEffect(() => {
    if (!state.editing) return;
    const timer = window.setTimeout(() => saveRecallDraft(state.draftPath, state.draftContent), 250);
    return () => window.clearTimeout(timer);
  }, [state.editing, state.draftPath, state.draftContent]);

  useEffect(() => () => {
    const draft = draftSnapshotRef.current;
    if (draft.editing) saveRecallDraft(draft.path, draft.content);
  }, []);

  useEffect(() => {
    if (!state.notice) return;
    const timer = window.setTimeout(() => dispatch({ type: 'notice', notice: null }), 3500);
    return () => window.clearTimeout(timer);
  }, [state.notice]);

  async function fetchLibraryEntries(): Promise<RecallEntry[]> {
    const response = await api<{ entries: RecallEntry[] }>('/v1/recall?max_entries=500');
    return response.entries || [];
  }

  async function fetchSearchEntries(query: string): Promise<RecallEntry[]> {
    const response = await api<{ results: Array<{ path: string; size_bytes?: number }> }>('/v1/recall/search', {
      method: 'POST',
      body: JSON.stringify({ query, prefix: '', max_results: 100 }),
    });
    return (response.results || []).map((item) => ({
      path: item.path,
      name: nameOf(item.path),
      type: 'file',
      size_bytes: item.size_bytes,
    }));
  }

  async function refreshEntries(query = state.appliedQuery): Promise<boolean> {
    const requestID = ++entryRequestRef.current;
    try {
      const libraryPromise = fetchLibraryEntries();
      const entriesPromise = query ? fetchSearchEntries(query) : libraryPromise;
      const [libraryEntries, entries] = await Promise.all([libraryPromise, entriesPromise]);
      if (requestID !== entryRequestRef.current) return false;
      dispatch({ type: 'libraryEntries', entries: libraryEntries });
      dispatch({ type: 'searchApplied', query, entries });
      return true;
    } catch (reason) {
      if (requestID !== entryRequestRef.current) return false;
      throw reason;
    }
  }

  async function applySearch(query: string): Promise<boolean> {
    const requestID = ++entryRequestRef.current;
    try {
      if (query) {
        const entries = await fetchSearchEntries(query);
        if (requestID !== entryRequestRef.current) return false;
        dispatch({ type: 'searchApplied', query, entries });
        return true;
      }
      const entries = state.libraryEntries.length > 0 ? state.libraryEntries : await fetchLibraryEntries();
      if (requestID !== entryRequestRef.current) return false;
      if (state.libraryEntries.length === 0) dispatch({ type: 'libraryEntries', entries });
      dispatch({ type: 'searchApplied', query: '', entries });
      return true;
    } catch (reason) {
      if (requestID !== entryRequestRef.current) return false;
      throw reason;
    }
  }

  async function loadSyncState() {
    const [syncStatus, gitDiff] = await Promise.all([
      api<SyncStatus>('/v1/sync/status'),
      api<GitDiff>('/v1/git/diff'),
    ]);
    dispatch({ type: 'syncState', syncStatus, gitDiff });
  }

  async function loadHistory() {
    const response = await api<{ commits: GitCommit[] }>('/v1/git/log?limit=12');
    dispatch({ type: 'commits', commits: response.commits || [] });
  }

  async function loadEmbeddingStatus() {
    const response = await api<EmbeddingStatus>('/v1/embeddings/status');
    dispatch({ type: 'embedding:status', status: response });
  }

  function revealCompactEditor() {
    if (!usesSinglePaneRecallLayout()) return;
    window.setTimeout(() => window.scrollTo({ top: 0, behavior: 'auto' }), 0);
  }

  async function openRecall(path: string): Promise<boolean> {
    const requestID = ++detailRequestRef.current;
    try {
      const response = await api<{ recall: Recall }>(`/v1/recall/${encodeURIComponent(path)}`);
      if (requestID !== detailRequestRef.current) return false;
      const preserveDraftAvailable = state.draftAvailable && !state.editing;
      if (state.editing) clearRecallDraft();
      dispatch({ type: 'opened', recall: response.recall, preserveDraftAvailable });
      updateRoute(response.recall.path, state.appliedQuery);
      revealCompactEditor();
      return true;
    } catch (reason) {
      if (requestID !== detailRequestRef.current) return false;
      throw reason;
    }
  }

  function blockIfRecoveryPending(): boolean {
    if (!state.draftAvailable || state.editing) return false;
    dispatch({ type: 'notice', notice: { text: '请先恢复或丢弃检测到的草稿。', danger: true } });
    return true;
  }

  function blockIfUnsaved(): boolean {
    if (!hasUnsavedChanges) return false;
    dispatch({ type: 'notice', notice: { text: '请先保存或取消当前编辑。', danger: true } });
    return true;
  }

  function openRecallFromUI(path: string) {
    if (blockIfUnsaved()) return;
    dispatch({ type: 'notice', notice: null });
    void openRecall(path).catch((reason) => {
      dispatch({ type: 'notice', notice: { text: messageOf(reason), danger: true } });
    });
  }

  async function refreshAll(path = state.current?.path || '') {
    const loadingID = ++loadingRequestRef.current;
    dispatch({ type: 'load:start' });
    try {
      const [entriesCurrent] = await Promise.all([refreshEntries(), loadSyncState(), loadHistory(), loadEmbeddingStatus()]);
      if (path && entriesCurrent && loadingID === loadingRequestRef.current) {
        try {
          await openRecall(path);
        } catch (reason) {
          detailRequestRef.current += 1;
          dispatch({ type: 'clearSelection' });
          updateRoute('', state.appliedQuery);
          throw reason;
        }
      }
    } catch (reason) {
      if (loadingID === loadingRequestRef.current) {
        dispatch({ type: 'notice', notice: { text: messageOf(reason), danger: true } });
      }
    } finally {
      if (loadingID === loadingRequestRef.current) dispatch({ type: 'load:finish' });
    }
  }
  refreshAllRef.current = refreshAll;

  async function searchMemories(event?: FormEvent) {
    event?.preventDefault();
    dispatch({ type: 'notice', notice: null });
    detailRequestRef.current += 1;
    const query = state.query.trim();
    const loadingID = ++loadingRequestRef.current;
    dispatch({ type: 'load:start' });
    try {
      if (await applySearch(query)) updateRoute(state.current?.path || '', query);
    } catch (reason) {
      if (loadingID === loadingRequestRef.current) {
        dispatch({ type: 'notice', notice: { text: messageOf(reason), danger: true } });
      }
    } finally {
      if (loadingID === loadingRequestRef.current) dispatch({ type: 'load:finish' });
    }
  }

  async function clearSearch() {
    dispatch({ type: 'notice', notice: null });
    detailRequestRef.current += 1;
    const loadingID = ++loadingRequestRef.current;
    dispatch({ type: 'query', query: '' });
    dispatch({ type: 'load:start' });
    try {
      if (await applySearch('')) updateRoute(state.current?.path || '', '');
    } catch (reason) {
      if (loadingID === loadingRequestRef.current) {
        dispatch({ type: 'notice', notice: { text: messageOf(reason), danger: true } });
      }
    } finally {
      if (loadingID === loadingRequestRef.current) dispatch({ type: 'load:finish' });
    }
  }

  function startNew() {
    if (blockIfUnsaved() || blockIfRecoveryPending()) return;
    dispatch({ type: 'notice', notice: null });
    detailRequestRef.current += 1;
    dispatch({ type: 'newDraft', path: 'inbox/new-recall.md', content: NEW_RECALL_TEMPLATE });
    revealCompactEditor();
  }

  function cancelEdit() {
    clearRecallDraft();
    dispatch({ type: 'cancelEdit' });
  }

  function startEdit() {
    if (blockIfRecoveryPending()) return;
    dispatch({ type: 'editCurrent' });
  }

  function backToFileList() {
    if (blockIfUnsaved()) return;
    detailRequestRef.current += 1;
    dispatch({ type: 'clearSelection' });
    updateRoute('', state.appliedQuery);
    window.scrollTo({ top: 0, behavior: 'auto' });
  }

  async function restoreDraft() {
    const saved = loadRecallDraft();
    if (!saved) return;
    const path = normalizePath(saved.path) || 'inbox/recovered-recall.md';
    detailRequestRef.current += 1;
    dispatch({ type: 'busy', busy: true });
    try {
      const exists = state.libraryEntries.some((entry) => entry.type === 'file' && normalizePath(entry.path) === path);
      const current = exists
        ? (await api<{ recall: Recall }>(`/v1/recall/${encodeURIComponent(path)}`)).recall
        : undefined;
      dispatch({ type: 'restoreDraft', path, content: saved.content, current });
      dispatch({ type: 'notice', notice: { text: current ? '已恢复已有文件的编辑草稿' : '已恢复新建草稿' } });
      revealCompactEditor();
    } catch (reason) {
      dispatch({ type: 'notice', notice: { text: messageOf(reason), danger: true } });
    } finally {
      dispatch({ type: 'busy', busy: false });
    }
  }

  async function saveRecall() {
    const path = normalizePath(state.draftPath);
    if (!path || !state.draftContent.trim()) {
      dispatch({ type: 'notice', notice: { text: '路径和内容不能为空', danger: true } });
      return;
    }
    if (!/\.(md|markdown|txt)$/i.test(path)) {
      dispatch({ type: 'notice', notice: { text: '召回文件必须使用 .md、.markdown 或 .txt 扩展名', danger: true } });
      return;
    }
    dispatch({ type: 'busy', busy: true });
    try {
      const existing = Boolean(state.current?.path) && !state.creating;
      const target = existing ? `/v1/recall/${encodeURIComponent(state.current!.path)}` : '/v1/recall';
      const response = await api<{ recall: Recall }>(target, {
        method: existing ? 'PATCH' : 'POST',
        body: JSON.stringify({ path: existing ? state.current!.path : path, content: state.draftContent, confirmed: true, overwrite: true }),
      });
      clearRecallDraft();
      dispatch({ type: 'draftAvailable', available: false });
      await Promise.all([refreshEntries(), loadSyncState(), loadHistory()]);
      await openRecall(response.recall.path);
      dispatch({ type: 'notice', notice: { text: '召回内容已保存' } });
    } catch (reason) {
      dispatch({ type: 'notice', notice: { text: messageOf(reason), danger: true } });
    } finally {
      dispatch({ type: 'busy', busy: false });
    }
  }

  async function confirmMove() {
    const pending = state.pendingAction;
    if (!pending || pending.kind !== 'move') return;
    const normalized = normalizePath(pending.nextPath);
    if (!normalized || normalized === pending.path) {
      dispatch({ type: 'pendingError', error: '请输入新的召回路径。' });
      return;
    }
    if (!/\.(md|markdown|txt)$/i.test(normalized)) {
      dispatch({ type: 'pendingError', error: '目标路径必须是 Markdown 或文本文件。' });
      return;
    }
    dispatch({ type: 'busy', busy: true });
    try {
      const response = await api<{ recall: Recall }>('/v1/recall/move', {
        method: 'POST',
        body: JSON.stringify({ from_path: pending.path, to_path: normalized, confirmed: true, overwrite: false }),
      });
      dispatch({ type: 'pending', pendingAction: null });
      await refreshEntries();
      await openRecall(response.recall.path);
      dispatch({ type: 'notice', notice: { text: '召回内容已移动' } });
    } catch (reason) {
      dispatch({ type: 'pendingError', error: messageOf(reason) });
    } finally {
      dispatch({ type: 'busy', busy: false });
    }
  }

  async function confirmDelete() {
    const pending = state.pendingAction;
    if (!pending || pending.kind !== 'delete') return;
    detailRequestRef.current += 1;
    dispatch({ type: 'busy', busy: true });
    try {
      await api(`/v1/recall/${encodeURIComponent(pending.path)}?confirmed=true`, { method: 'DELETE' });
      dispatch({ type: 'pending', pendingAction: null });
      dispatch({ type: 'clearSelection' });
      updateRoute('', state.appliedQuery);
      await Promise.all([refreshEntries(), loadSyncState(), loadHistory()]);
      dispatch({ type: 'notice', notice: { text: '召回内容已删除' } });
    } catch (reason) {
      dispatch({ type: 'pendingError', error: messageOf(reason) });
    } finally {
      dispatch({ type: 'busy', busy: false });
    }
  }

  function cardPayload(extra: Record<string, unknown> = {}) {
    return {
      title: state.card.title.trim(),
      content: state.card.content.trim(),
      project: state.card.project.trim() || 'agentdock',
      type: state.card.type,
      status: 'inbox',
      confidence: 'medium',
      source: state.card.source.trim() || 'nexus-recall-ui',
      evidence: state.card.evidence.trim(),
      path: state.card.path.trim(),
      tags: csvTags(state.card.tags),
      ...extra,
    };
  }

  async function captureCard(event?: FormEvent) {
    event?.preventDefault();
    if (!state.card.title.trim() || !state.card.content.trim()) {
      dispatch({ type: 'notice', notice: { text: '卡片标题和内容不能为空。', danger: true } });
      return;
    }
    dispatch({ type: 'busy', busy: true });
    try {
      const result = await api<CardCaptureResult>('/v1/recall/cards/capture', {
        method: 'POST',
        body: JSON.stringify(cardPayload({ max_results: 6 })),
      });
      dispatch({ type: 'card:capture', capture: result, path: result.card.path });
      dispatch({ type: 'notice', notice: { text: result.similar_count ? `已生成候选，发现 ${result.similar_count} 条相似卡片。` : '已生成候选卡片。' } });
    } catch (reason) {
      dispatch({ type: 'notice', notice: { text: messageOf(reason), danger: true } });
    } finally {
      dispatch({ type: 'busy', busy: false });
    }
  }

  async function writeCard() {
    if (blockIfUnsaved()) return;
    if (!state.card.capture) {
      dispatch({ type: 'notice', notice: { text: '请先生成候选卡片。', danger: true } });
      return;
    }
    if ((state.card.capture.warnings?.length || 0) > 0 && !state.card.allowWarnings) {
      dispatch({ type: 'notice', notice: { text: '候选卡片有警告，请确认后再写入。', danger: true } });
      return;
    }
    dispatch({ type: 'busy', busy: true });
    try {
      const result = await api<CardWriteResult>('/v1/recall/cards', {
        method: 'POST',
        body: JSON.stringify(cardPayload({ confirmed: true, overwrite: false, allow_warnings: state.card.allowWarnings })),
      });
      await Promise.all([refreshEntries(), loadSyncState(), loadHistory(), loadEmbeddingStatus()]);
      await openRecall(result.recall.path);
      dispatch({ type: 'card:capture', capture: null });
      dispatch({ type: 'notice', notice: { text: `卡片已写入：${result.card.path}` } });
    } catch (reason) {
      dispatch({ type: 'notice', notice: { text: messageOf(reason), danger: true } });
    } finally {
      dispatch({ type: 'busy', busy: false });
    }
  }

  async function reindexCards() {
    dispatch({ type: 'busy', busy: true });
    try {
      await api('/v1/embeddings/reindex', {
        method: 'POST',
        body: JSON.stringify({ prefix: 'cards', max_entries: 1000 }),
      });
      await loadEmbeddingStatus();
      dispatch({ type: 'notice', notice: { text: 'cards 向量索引已重建。' } });
    } catch (reason) {
      dispatch({ type: 'notice', notice: { text: messageOf(reason), danger: true } });
    } finally {
      dispatch({ type: 'busy', busy: false });
    }
  }

  async function searchCardEmbeddings(event?: FormEvent) {
    event?.preventDefault();
    const text = state.embedding.query.trim() || state.query.trim();
    if (!text) {
      dispatch({ type: 'notice', notice: { text: '请输入要搜索的经验问题。', danger: true } });
      return;
    }
    dispatch({ type: 'busy', busy: true });
    try {
      const response = await api<EmbeddingSearchResponse>('/v1/embeddings/search', {
        method: 'POST',
        body: JSON.stringify({ query: text, prefix: 'cards', max_results: 8 }),
      });
      dispatch({ type: 'embedding:results', results: response.results || [] });
      dispatch({ type: 'notice', notice: { text: `向量搜索返回 ${response.count ?? response.results?.length ?? 0} 条结果。` } });
    } catch (reason) {
      dispatch({ type: 'notice', notice: { text: messageOf(reason), danger: true } });
    } finally {
      dispatch({ type: 'busy', busy: false });
    }
  }

  async function syncNow(action: 'pull' | 'push' | 'now' = 'now') {
    const currentPath = state.current?.path || '';
    dispatch({ type: 'busy', busy: true });
    try {
      const response = await api<SyncStatus>(`/v1/sync/${action}`, { method: 'POST', body: '{}' });
      dispatch({ type: 'syncState', syncStatus: response, gitDiff: state.gitDiff });
      await Promise.all([refreshEntries(), loadSyncState(), loadHistory()]);
      if (currentPath && initialPath() === currentPath) {
        try {
          await openRecall(currentPath);
        } catch {
          detailRequestRef.current += 1;
          dispatch({ type: 'clearSelection' });
          updateRoute('', state.appliedQuery);
        }
      }
      dispatch({ type: 'notice', notice: { text: action === 'pull' ? '已从远端更新' : action === 'push' ? '已保存到远端' : '召回库已同步' } });
    } catch (reason) {
      dispatch({ type: 'notice', notice: { text: messageOf(reason), danger: true } });
    } finally {
      dispatch({ type: 'busy', busy: false });
    }
  }

  function refreshAllFromUI() {
    if (blockIfUnsaved()) return;
    dispatch({ type: 'notice', notice: null });
    void refreshAll();
  }

  function syncNowFromUI(action?: 'pull' | 'push' | 'now') {
    if (blockIfUnsaved()) return;
    dispatch({ type: 'notice', notice: null });
    void syncNow(action);
  }

  useEffect(() => {
    if (externalRefreshTokenRef.current === refreshToken) return;
    externalRefreshTokenRef.current = refreshToken;
    refreshAllFromUI();
  }, [refreshToken]);

  const actions = {
    refreshAll: refreshAllFromUI,
    syncNow: syncNowFromUI,
    restoreDraft: () => { void restoreDraft(); },
    discardDraft: () => dispatch({ type: 'draftAvailable', available: false }),
    clearNotice: () => dispatch({ type: 'notice', notice: null }),
    closePendingAction: () => dispatch({ type: 'pending', pendingAction: null }),
    updatePendingMovePath: (value: string) => dispatch({ type: 'pendingMovePath', path: value }),
    confirmMove: () => { void confirmMove(); },
    confirmDelete: () => { void confirmDelete(); },
    searchMemories: (event?: FormEvent) => { void searchMemories(event); },
    clearSearch: () => { void clearSearch(); },
    setQuery: (value: string) => dispatch({ type: 'query', query: value }),
    openRecall: openRecallFromUI,
    startNew,
    backToFileList,
    startEdit,
    requestMove: () => { if (state.current?.path) dispatch({ type: 'pending', pendingAction: { kind: 'move', path: state.current.path, nextPath: state.current.path } }); },
    requestDelete: () => { if (state.current?.path) dispatch({ type: 'pending', pendingAction: { kind: 'delete', path: state.current.path } }); },
    cancelEdit,
    saveRecall: () => { void saveRecall(); },
    setDraftPath: (path: string) => dispatch({ type: 'draft:path', path }),
    setDraftContent: (content: string) => dispatch({ type: 'draft:content', content }),
    setCardField: (field: keyof Omit<CardDraft, 'capture' | 'allowWarnings'>, value: string) => dispatch({ type: 'card:field', field, value }),
    setAllowCardWarnings: (allowWarnings: boolean) => dispatch({ type: 'card:allowWarnings', allowWarnings }),
    captureCard: (event?: FormEvent) => { void captureCard(event); },
    writeCard: () => { void writeCard(); },
    openSimilarCard: openRecallFromUI,
    setEmbeddingQuery: (query: string) => dispatch({ type: 'embedding:query', query }),
    searchCardEmbeddings: (event?: FormEvent) => { void searchCardEmbeddings(event); },
    reindexCards: () => { void reindexCards(); },
  };

  return { state, fileEntries, libraryFileCount, directoryCount, changedCount, dirty, hasUnsavedChanges, detailOpen, editorRef, actions };
}
