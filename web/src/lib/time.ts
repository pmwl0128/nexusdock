const LOCALE = 'zh-CN';
const DEFAULT_TIME_ZONE = 'Asia/Shanghai';
const ISO_WITHOUT_ZONE = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}(:\d{2}(\.\d+)?)?$/;

export function displayTimeZone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || DEFAULT_TIME_ZONE;
  } catch {
    return DEFAULT_TIME_ZONE;
  }
}

export function timeZoneLabel(): string {
  return displayTimeZone().replace(/_/g, ' ');
}

export function parseTimestamp(value?: string): Date | null {
  if (!value) return null;
  const raw = value.trim();
  if (!raw) return null;
  const normalized = ISO_WITHOUT_ZONE.test(raw) ? `${raw}Z` : raw;
  const date = new Date(normalized);
  return Number.isNaN(date.getTime()) ? null : date;
}

export function formatTime(value?: string, options: { seconds?: boolean; compact?: boolean; fallback?: string } = {}): string {
  const date = parseTimestamp(value);
  if (!date) return value || options.fallback || '暂无';
  const timeZone = displayTimeZone();
  const formatter = new Intl.DateTimeFormat(LOCALE, {
    timeZone,
    year: '2-digit',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    ...(options.seconds ? { second: '2-digit' as const } : {}),
    hour12: false,
    timeZoneName: options.compact ? undefined : 'shortOffset',
  });
  return formatter.format(date);
}

export function formatTimeCompact(value?: string): string {
  return formatTime(value, { compact: true });
}
