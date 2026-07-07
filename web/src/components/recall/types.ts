import type { FormEvent, RefObject } from 'react';

export type EntryType = 'file' | 'directory';
export type RecallEntry = { path: string; name?: string; type: EntryType; size_bytes?: number };
export type Recall = { path: string; content: string };
export type GitCommit = { hash: string; short_hash: string; date: string; author: string; subject: string };
export type ChangedFile = { status: string; path: string };
export type GitDiff = { ok?: boolean; git_repo?: boolean; dirty?: boolean; status?: string; stat?: string; files?: ChangedFile[] };
export type SyncStatus = Record<string, unknown> & {
  dirty?: boolean;
  ahead?: string | number;
  behind?: string | number;
  pending_push?: boolean;
  branch?: string;
  remote?: string;
};
export type Notice = { text: string; danger?: boolean } | null;
export type PendingRecallAction =
  | { kind: 'move'; path: string; nextPath: string; error?: string }
  | { kind: 'delete'; path: string; error?: string }
  | null;

export type RecallCard = {
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

export type CardSearchResult = { path: string; title?: string; score?: number; size_bytes?: number };
export type CardCaptureResult = {
  ok: boolean;
  card: RecallCard;
  warnings?: string[];
  capture_plan?: Record<string, unknown>;
  similar_results?: CardSearchResult[];
  similar_count?: number;
};
export type CardWriteResult = { ok: boolean; card: RecallCard; recall: Recall; warnings?: string[]; index_policy?: string };
export type EmbeddingStatus = {
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
export type EmbeddingSearchResult = { path: string; title?: string; score: number };
export type EmbeddingSearchResponse = { ok?: boolean; count?: number; model?: string; index?: { count?: number; dimension?: number }; results?: EmbeddingSearchResult[] };

export type CardDraft = {
  title: string;
  content: string;
  project: string;
  type: string;
  tags: string;
  source: string;
  evidence: string;
  path: string;
  allowWarnings: boolean;
  capture: CardCaptureResult | null;
};

export type EmbeddingPanelState = {
  status: EmbeddingStatus | null;
  query: string;
  results: EmbeddingSearchResult[];
};

export type RecallWorkspaceState = {
  entries: RecallEntry[];
  current: Recall | null;
  draftPath: string;
  draftContent: string;
  editing: boolean;
  creating: boolean;
  query: string;
  syncStatus: SyncStatus | null;
  gitDiff: GitDiff | null;
  commits: GitCommit[];
  loading: boolean;
  busy: boolean;
  notice: Notice;
  draftAvailable: boolean;
  pendingAction: PendingRecallAction;
  card: CardDraft;
  embedding: EmbeddingPanelState;
};

export type RecallWorkspaceActions = {
  refreshAll: () => void;
  syncNow: (action?: 'pull' | 'push' | 'now') => void;
  restoreDraft: () => void;
  discardDraft: () => void;
  closePendingAction: () => void;
  updatePendingMovePath: (value: string) => void;
  confirmMove: () => void;
  confirmDelete: () => void;
  searchMemories: (event?: FormEvent) => void;
  setQuery: (value: string) => void;
  openRecall: (path: string) => void;
  startNew: () => void;
  backToFileList: () => void;
  startEdit: () => void;
  requestMove: () => void;
  requestDelete: () => void;
  cancelEdit: () => void;
  saveRecall: () => void;
  setDraftPath: (value: string) => void;
  setDraftContent: (value: string) => void;
  setCardField: (field: keyof Omit<CardDraft, 'capture' | 'allowWarnings'>, value: string) => void;
  setAllowCardWarnings: (value: boolean) => void;
  captureCard: (event?: FormEvent) => void;
  writeCard: () => void;
  openSimilarCard: (path: string) => void;
  setEmbeddingQuery: (value: string) => void;
  searchCardEmbeddings: (event?: FormEvent) => void;
  reindexCards: () => void;
};

export type RecallWorkspaceViewModel = {
  state: RecallWorkspaceState;
  fileEntries: RecallEntry[];
  directoryCount: number;
  changedCount: number;
  dirty: boolean;
  hasUnsavedChanges: boolean;
  detailOpen: boolean;
  editorRef: RefObject<HTMLElement | null>;
  actions: RecallWorkspaceActions;
};
