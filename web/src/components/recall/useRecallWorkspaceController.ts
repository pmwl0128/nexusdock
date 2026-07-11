import { useEffect, useMemo, useReducer, useRef, type FormEvent } from 'react';
import { api } from '../../api/client';
import { clearRecallDraft, loadRecallDraft, saveRecallDraft } from '../../lib/drafts';
import { initialRecallState, recallReducer } from './recallState';
import type {
  CardDraft, CardCaptureResult, CardWriteResult, EmbeddingSearchResponse,
  EmbeddingStatus, GitCommit, GitDiff, Recall, RecallEntry, RecallWorkspaceViewModel, SyncStatus,
} from './types';
import {
  NEW_RECALL_TEMPLATE, csvTags, initialPath, isCompactViewport,
  messageOf, nameOf, normalizePath, updateRoute,
} from './utils';

export function useRecallWorkspaceController(): RecallWorkspaceViewModel {
  const [state, dispatch] = useReducer(recallReducer, undefined, initialRecallState);
  const editorRef = useRef<HTMLElement | null>(null);
  const refreshAllRef = useRef<(path?: string) => Promise<void>>(async () => undefined);

  const fileEntries = useMemo(
    () => state.entries.filter((entry) => entry.type === 'file').sort((a, b) => a.path.localeCompare(b.path, 'zh-CN')),
    [state.entries],
  );
  const directoryCount = useMemo(() => {
    const directories = new Set<string>();
    for (const entry of fileEntries) {
      const parts = normalizePath(entry.path).split('/').slice(0, -1);
      let current = '';
      for (const part of parts) {
        current = current ? `${current}/${part}` : part;
        directories.add(current);
      }
    }
    return directories.size;
  }, [fileEntries]);
  const changedCount = state.gitDiff?.files?.length ?? 0;
  const dirty = Boolean(state.gitDiff?.dirty || state.syncStatus?.dirty || state.syncStatus?.pending_push);
  const hasUnsavedChanges = state.editing && (state.draftPath !== (state.current?.path || '') || state.draftContent !== (state.current?.content || ''));
  const detailOpen = Boolean(state.current || state.editing);

  useEffect(() => {
    void refreshAllRef.current(initialPath());
  }, []);

  useEffect(() => {
    if (!state.editing) return;
    const timer = window.setTimeout(() => saveRecallDraft(state.draftPath, state.draftContent), 250);
    return () => window.clearTimeout(timer);
  }, [state.editing, state.draftPath, state.draftContent]);

  useEffect(() => {
    if (!state.notice) return;
    const timer = window.setTimeout(() => dispatch({ type: 'notice', notice: null }), 3500);
    return () => window.clearTimeout(timer);
  }, [state.notice]);

  async function loadList() {
    const response = await api<{ entries: RecallEntry[] }>('/v1/recall?max_entries=500');
    dispatch({ type: 'entries', entries: response.entries || [] });
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

  async function openRecall(path: string) {
    const response = await api<{ recall: Recall }>(`/v1/recall/${encodeURIComponent(path)}`);
    clearRecallDraft();
    dispatch({ type: 'opened', recall: response.recall, query: state.query });
    updateRoute(response.recall.path, state.query.trim());
    if (isCompactViewport()) {
      window.setTimeout(() => editorRef.current?.scrollIntoView({ block: 'start' }), 0);
    }
  }

  async function refreshAll(path = state.current?.path || '') {
    dispatch({ type: 'load:start' });
    try {
      await Promise.all([loadList(), loadSyncState(), loadHistory(), loadEmbeddingStatus()]);
      if (path) await openRecall(path);
    } catch (reason) {
      dispatch({ type: 'notice', notice: { text: messageOf(reason), danger: true } });
    } finally {
      dispatch({ type: 'load:finish' });
    }
  }
  refreshAllRef.current = refreshAll;

  async function searchMemories(event?: FormEvent) {
    event?.preventDefault();
    const query = state.query.trim();
    if (!query) {
      await loadList();
      updateRoute(state.current?.path || '', '');
      return;
    }
    const response = await api<{ results: Array<{ path: string; size_bytes?: number }> }>('/v1/recall/search', {
      method: 'POST',
      body: JSON.stringify({ query, prefix: '', max_results: 100 }),
    });
    dispatch({ type: 'entries', entries: (response.results || []).map((item) => ({ path: item.path, name: nameOf(item.path), type: 'file', size_bytes: item.size_bytes })) });
    updateRoute(state.current?.path || '', query);
  }

  function startNew() {
    dispatch({ type: 'newDraft', path: 'inbox/new-recall.md', content: NEW_RECALL_TEMPLATE });
    if (isCompactViewport()) {
      window.setTimeout(() => editorRef.current?.scrollIntoView({ block: 'start' }), 0);
    }
  }

  function cancelEdit() {
    clearRecallDraft();
    dispatch({ type: 'cancelEdit' });
  }

  function backToFileList() {
    if (hasUnsavedChanges) {
      dispatch({ type: 'notice', notice: { text: '请先保存或取消当前编辑。', danger: true } });
      return;
    }
    dispatch({ type: 'clearSelection', query: state.query.trim() });
    updateRoute('', state.query.trim());
    window.scrollTo({ top: 0, behavior: 'smooth' });
  }

  function restoreDraft() {
    const saved = loadRecallDraft();
    if (!saved) return;
    dispatch({ type: 'restoreDraft', path: saved.path || 'inbox/recovered-recall.md', content: saved.content });
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
      await Promise.all([loadList(), loadSyncState(), loadHistory()]);
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
      await loadList();
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
    dispatch({ type: 'busy', busy: true });
    try {
      await api(`/v1/recall/${encodeURIComponent(pending.path)}?confirmed=true`, { method: 'DELETE' });
      dispatch({ type: 'pending', pendingAction: null });
      dispatch({ type: 'clearSelection', query: state.query.trim() });
      updateRoute('', state.query.trim());
      await Promise.all([loadList(), loadSyncState(), loadHistory()]);
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
      await Promise.all([loadList(), loadSyncState(), loadHistory(), loadEmbeddingStatus()]);
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
    dispatch({ type: 'busy', busy: true });
    try {
      const response = await api<SyncStatus>(`/v1/sync/${action}`, { method: 'POST', body: '{}' });
      dispatch({ type: 'syncState', syncStatus: response, gitDiff: state.gitDiff });
      await Promise.all([loadList(), loadSyncState(), loadHistory()]);
      if (state.current?.path) await openRecall(state.current.path).catch(() => dispatch({ type: 'clearSelection', query: state.query.trim() }));
      dispatch({ type: 'notice', notice: { text: action === 'pull' ? '已从远端更新' : action === 'push' ? '已保存到远端' : '召回库已同步' } });
    } catch (reason) {
      dispatch({ type: 'notice', notice: { text: messageOf(reason), danger: true } });
    } finally {
      dispatch({ type: 'busy', busy: false });
    }
  }

  const actions = {
    refreshAll: () => { void refreshAll(); },
    syncNow: (action?: 'pull' | 'push' | 'now') => { void syncNow(action); },
    restoreDraft,
    discardDraft: () => dispatch({ type: 'draftAvailable', available: false }),
    closePendingAction: () => dispatch({ type: 'pending', pendingAction: null }),
    updatePendingMovePath: (value: string) => dispatch({ type: 'pendingMovePath', path: value }),
    confirmMove: () => { void confirmMove(); },
    confirmDelete: () => { void confirmDelete(); },
    searchMemories: (event?: FormEvent) => { void searchMemories(event); },
    setQuery: (value: string) => dispatch({ type: 'query', query: value }),
    openRecall: (path: string) => { void openRecall(path); },
    startNew,
    backToFileList,
    startEdit: () => dispatch({ type: 'editCurrent' }),
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
    openSimilarCard: (path: string) => { void openRecall(path); },
    setEmbeddingQuery: (query: string) => dispatch({ type: 'embedding:query', query }),
    searchCardEmbeddings: (event?: FormEvent) => { void searchCardEmbeddings(event); },
    reindexCards: () => { void reindexCards(); },
  };

  return { state, fileEntries, directoryCount, changedCount, dirty, hasUnsavedChanges, detailOpen, editorRef, actions };
}
