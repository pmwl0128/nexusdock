export type ApiErrorBody = {
  error?: {
    code?: string;
    message?: string;
    details?: unknown;
    trace_id?: string;
  };
  message?: string;
};

export class ApiError extends Error {
  readonly status: number;
  readonly code?: string;
  readonly details?: unknown;
  readonly traceId?: string;

  constructor(message: string, status: number, body: ApiErrorBody = {}) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = body.error?.code;
    this.details = body.error?.details;
    this.traceId = body.error?.trace_id;
  }
}

export type ApiOptions = RequestInit & { timeoutMs?: number };

function decodeURLCredential(value: string): string {
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
}

function encodeBasicAuth(value: string): string {
  const bytes = new TextEncoder().encode(value);
  let binary = '';
  bytes.forEach((byte) => { binary += String.fromCharCode(byte); });
  return `Basic ${btoa(binary)}`;
}

function readEmbeddedBasicAuth(): string {
  const current = new URL(window.location.href);
  if (!current.username && !current.password) return '';
  const username = decodeURLCredential(current.username);
  const password = decodeURLCredential(current.password);
  current.username = '';
  current.password = '';
  window.history.replaceState(window.history.state, document.title, `${current.pathname}${current.search}${current.hash}`);
  return encodeBasicAuth(`${username}:${password}`);
}

const embeddedBasicAuth = readEmbeddedBasicAuth();

function requestURL(path: string): URL {
  const url = new URL(path, window.location.origin);
  url.username = '';
  url.password = '';
  if (url.origin !== window.location.origin) {
    throw new ApiError('拒绝跨源 API 请求，避免泄漏管理凭据', 0);
  }
  return url;
}

export async function api<T>(path: string, options: ApiOptions = {}): Promise<T> {
  const controller = new AbortController();
  const timeoutMs = options.timeoutMs ?? 15_000;
  const timeout = window.setTimeout(() => controller.abort(), timeoutMs);
  const externalSignal = options.signal;
  const abort = () => controller.abort();
  externalSignal?.addEventListener('abort', abort, { once: true });

  try {
    const headers = new Headers(options.headers);
    headers.set('Accept', 'application/json');
    if (options.body && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json');
    const url = requestURL(path);
    if (embeddedBasicAuth && !headers.has('Authorization')) headers.set('Authorization', embeddedBasicAuth);

    const response = await fetch(url.toString(), {
      ...options,
      headers,
      signal: controller.signal,
      credentials: 'same-origin',
    });

    if (response.status === 204) return undefined as T;

    const text = await response.text();
    let body: ApiErrorBody | T = {} as T;
    if (text) {
      try {
        body = JSON.parse(text) as ApiErrorBody | T;
      } catch {
        throw new ApiError('服务返回了无法解析的响应', response.status, { error: { code: 'INVALID_JSON' } });
      }
    }

    const errorBody = body as ApiErrorBody;
    if (!response.ok || errorBody.error) {
      throw new ApiError(
        errorBody.error?.message || errorBody.message || response.statusText || '请求失败',
        response.status,
        errorBody,
      );
    }
    return body as T;
  } catch (error) {
    if (error instanceof ApiError) throw error;
    if (error instanceof DOMException && error.name === 'AbortError') {
      throw new Error(externalSignal?.aborted ? '请求已取消' : '请求超时');
    }
    throw error instanceof Error ? error : new Error('网络请求失败');
  } finally {
    window.clearTimeout(timeout);
    externalSignal?.removeEventListener('abort', abort);
  }
}
