/**
 * The single HTTP client for the Binnacle API.
 *
 * - Same-origin cookies carry the session.
 * - Mutations send the CSRF token read from the `binnacle_csrf` cookie.
 * - Errors are normalized into `ApiError` with the server's code and message.
 */

export interface ApiErrorBody {
  error?: {
    code?: string;
    message?: string;
    details?: Record<string, unknown> & { retryAfterSeconds?: number };
  };
}

export class ApiError extends Error {
  constructor(
    message: string,
    public readonly status: number,
    public readonly code = '',
    public readonly details: Record<string, unknown> = {},
  ) {
    super(message);
    this.name = 'ApiError';
  }

  get retryAfterSeconds(): number | undefined {
    const value = this.details.retryAfterSeconds;
    return typeof value === 'number' ? value : undefined;
  }
}

export function isApiError(value: unknown, code?: string): value is ApiError {
  return (
    value instanceof ApiError && (code === undefined || value.code === code)
  );
}

export function csrfToken(): string {
  const prefix = 'binnacle_csrf=';
  const raw =
    document.cookie
      .split(';')
      .map((value) => value.trim())
      .find((value) => value.startsWith(prefix))
      ?.slice(prefix.length) ?? '';
  try {
    return decodeURIComponent(raw);
  } catch {
    return raw;
  }
}

type Method = 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE';

export interface RequestOptions {
  body?: unknown;
  query?: Record<string, string | number | boolean | null | undefined>;
  signal?: AbortSignal;
  /** Generic message used when the server response carries none. */
  fallback?: string;
}

export function withQuery(
  path: string,
  query?: RequestOptions['query'],
): string {
  if (!query) return path;
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(query)) {
    if (value == null || value === '') continue;
    params.set(key, String(value));
  }
  const encoded = params.toString();
  return encoded ? `${path}${path.includes('?') ? '&' : '?'}${encoded}` : path;
}

async function errorFrom(
  response: Response,
  fallback: string,
): Promise<ApiError> {
  let body: ApiErrorBody = {};
  try {
    body = (await response.json()) as ApiErrorBody;
  } catch {
    // Non-JSON error bodies fall back to the generic message.
  }
  const details = body.error?.details ?? {};
  const retryAfter = response.headers.get('Retry-After');
  if (retryAfter && details.retryAfterSeconds == null) {
    const seconds = Number(retryAfter);
    if (Number.isFinite(seconds)) details.retryAfterSeconds = seconds;
  }
  return new ApiError(
    body.error?.message ?? fallback,
    response.status,
    body.error?.code ?? '',
    details,
  );
}

async function request<T>(
  method: Method,
  path: string,
  options: RequestOptions = {},
): Promise<T> {
  const headers = new Headers();
  if (method !== 'GET') headers.set('X-CSRF-Token', csrfToken());
  if (options.body !== undefined)
    headers.set('Content-Type', 'application/json');
  const response = await fetch(withQuery(path, options.query), {
    method,
    credentials: 'same-origin',
    headers,
    body: options.body === undefined ? undefined : JSON.stringify(options.body),
    signal: options.signal,
  });
  if (!response.ok) {
    throw await errorFrom(
      response,
      options.fallback ?? 'The request could not be completed.',
    );
  }
  if (response.status === 204 || response.status === 205) {
    return undefined as T;
  }
  const text = await response.text();
  return (text ? JSON.parse(text) : undefined) as T;
}

export const api = {
  get: <T>(path: string, options?: RequestOptions) =>
    request<T>('GET', path, options),
  post: <T = void>(path: string, body?: unknown, options?: RequestOptions) =>
    request<T>('POST', path, { ...options, body }),
  put: <T = void>(path: string, body?: unknown, options?: RequestOptions) =>
    request<T>('PUT', path, { ...options, body }),
  patch: <T = void>(path: string, body?: unknown, options?: RequestOptions) =>
    request<T>('PATCH', path, { ...options, body }),
  delete: <T = void>(path: string, options?: RequestOptions) =>
    request<T>('DELETE', path, options),
};

/** Human message for any thrown value, never leaking `Error:` prefixes. */
export function errorMessage(
  reason: unknown,
  fallback = 'Something went wrong.',
) {
  if (reason instanceof Error && reason.message) return reason.message;
  return fallback;
}
