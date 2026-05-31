export type Tab = 'dashboard' | 'memories' | 'git' | 'sync';
export type EntryType = 'file' | 'directory';

export type MemoryEntry = {
  path: string;
  name: string;
  type: EntryType;
  size_bytes?: number;
};

export type Memory = {
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

export type CommitFile = {
  status: string;
  path: string;
};

export type CommitDetail = {
  ok: boolean;
  git_repo: boolean;
  commit: GitCommit;
  files: CommitFile[];
  stat: string;
  diff: string;
};

export type SyncStatus = Record<string, unknown> & {
  dirty?: boolean;
  ahead?: string;
  behind?: string;
  pending_push?: boolean;
};

export type AccessConfig = {
  ok: boolean;
  enabled: boolean;
  username: string;
};
