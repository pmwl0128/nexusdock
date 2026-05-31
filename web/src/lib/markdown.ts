export function escapeHtml(input: string): string {
  return String(input).replace(/[&<>"']/g, (ch) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[ch] || ch));
}

export function splitFrontmatter(content: string): { meta: string; body: string } {
  if (!content.startsWith('---\n')) return { meta: '', body: content };
  const end = content.indexOf('\n---\n', 4);
  if (end < 0) return { meta: '', body: content };
  return { meta: content.slice(4, end), body: content.slice(end + 5) };
}
