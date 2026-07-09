const RECALL_DRAFT_KEY = 'nexusdock:draft:recall';
const MAX_DRAFT_BYTES = 256 * 1024;

export type RecallDraft = {
  path: string;
  content: string;
  savedAt: string;
};

export function loadRecallDraft(): RecallDraft | null {
  try {
    const raw = window.sessionStorage.getItem(RECALL_DRAFT_KEY);
    if (!raw) return null;
    const value = JSON.parse(raw) as Partial<RecallDraft>;
    if (typeof value.path !== 'string' || typeof value.content !== 'string' || typeof value.savedAt !== 'string') {
      clearRecallDraft();
      return null;
    }
    return value as RecallDraft;
  } catch {
    return null;
  }
}

export function saveRecallDraft(path: string, content: string): void {
  const value: RecallDraft = { path, content, savedAt: new Date().toISOString() };
  const raw = JSON.stringify(value);
  if (new TextEncoder().encode(raw).byteLength > MAX_DRAFT_BYTES) return;
  try {
    window.sessionStorage.setItem(RECALL_DRAFT_KEY, raw);
  } catch {
    // Storage can be unavailable in private browsing; the RAM-only draft still works.
  }
}

export function clearRecallDraft(): void {
  try {
    window.sessionStorage.removeItem(RECALL_DRAFT_KEY);
  } catch {
    // Ignore unavailable storage.
  }
}
