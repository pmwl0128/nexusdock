export type EntryType = 'file' | 'directory';

export type RecallEntry = {
  path: string;
  name: string;
  type: EntryType;
  size_bytes?: number;
};

export type Recall = {
  path: string;
  content: string;
};

export type ChangedFile = {
  status: string;
  path: string;
};

export type GitDiff = {
  ok: boolean;
  git_repo: boolean;
  dirty: boolean;
  status: string;
  stat: string;
  diff: string;
  cached_diff: string;
  files?: ChangedFile[];
};

export type GitCommit = {
  hash: string;
  short_hash: string;
  date: string;
  author: string;
  subject: string;
};

export type SyncStatus = Record<string, unknown> & {
  dirty?: boolean;
  ahead?: string;
  behind?: string;
  pending_push?: boolean;
};

export type ItemsResponse<T> = { items: T[] };
