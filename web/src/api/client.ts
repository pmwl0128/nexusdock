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

    const response = await fetch(path, {
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
        if (!response.ok) throw new ApiError('服务返回了无法解析的响应', response.status);
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
