const MEMORY_DRAFT_KEY = 'agentdock-nexus:draft:memory';
const MAX_DRAFT_BYTES = 256 * 1024;

export type MemoryDraft = {
  path: string;
  content: string;
  savedAt: string;
};

export function loadMemoryDraft(): MemoryDraft | null {
  try {
    const raw = window.sessionStorage.getItem(MEMORY_DRAFT_KEY);
    if (!raw) return null;
    const value = JSON.parse(raw) as Partial<MemoryDraft>;
    if (typeof value.path !== 'string' || typeof value.content !== 'string' || typeof value.savedAt !== 'string') {
      clearMemoryDraft();
      return null;
    }
    return value as MemoryDraft;
  } catch {
    return null;
  }
}

export function saveMemoryDraft(path: string, content: string): void {
  const value: MemoryDraft = { path, content, savedAt: new Date().toISOString() };
  const raw = JSON.stringify(value);
  if (new TextEncoder().encode(raw).byteLength > MAX_DRAFT_BYTES) return;
  try {
    window.sessionStorage.setItem(MEMORY_DRAFT_KEY, raw);
  } catch {
    // Storage can be unavailable in private browsing; the in-memory draft still works.
  }
}

export function clearMemoryDraft(): void {
  try {
    window.sessionStorage.removeItem(MEMORY_DRAFT_KEY);
  } catch {
    // Ignore unavailable storage.
  }
}
