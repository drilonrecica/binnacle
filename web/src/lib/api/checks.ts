import { api } from './client';

export type CheckFailureCode =
  | 'dns'
  | 'timeout'
  | 'connection'
  | 'tls_handshake'
  | 'unexpected_status'
  | 'body_mismatch'
  | 'target_blocked';

export interface CheckResult {
  checkId: string;
  status: 'success' | 'failure' | string;
  failureCode?: CheckFailureCode | '';
  httpStatus?: number;
  latencyMs?: number;
  checkedAt: string;
  consecutiveSuccesses: number;
  consecutiveFailures: number;
}

export interface Check {
  id: string;
  resourceId: string;
  name: string;
  url: string;
  method: 'GET' | 'HEAD' | string;
  /** Nanoseconds, as serialized by Go. */
  interval: number;
  /** Nanoseconds, as serialized by Go. */
  timeout: number;
  expectedStatusMin: number;
  expectedStatusMax: number;
  bodySubstring?: string;
  required: boolean;
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
  state?: CheckResult;
}

export interface CheckInput {
  resourceId: string;
  name: string;
  url: string;
  method: 'GET' | 'HEAD';
  intervalSeconds: number;
  timeoutSeconds: number;
  expectedStatusMin: number;
  expectedStatusMax: number;
  bodySubstring?: string;
  required: boolean;
  enabled: boolean;
}

export function listChecks(signal?: AbortSignal) {
  return api.get<Check[]>('/api/v1/checks', {
    query: { limit: 100 },
    signal,
    fallback: 'Health checks are unavailable.',
  });
}

export function createCheck(input: CheckInput) {
  return api.post<Check>('/api/v1/checks', input, {
    fallback: 'The check could not be created.',
  });
}

export function updateCheck(id: string, input: CheckInput) {
  return api.patch<Check>(`/api/v1/checks/${encodeURIComponent(id)}`, input, {
    fallback: 'The check could not be updated.',
  });
}

export function deleteCheck(id: string) {
  return api.delete(`/api/v1/checks/${encodeURIComponent(id)}`, {
    fallback: 'The check could not be deleted.',
  });
}

export function runCheck(id: string) {
  return api.post<CheckResult>(
    `/api/v1/checks/${encodeURIComponent(id)}/run`,
    undefined,
    {
      fallback: 'The check could not be run.',
    },
  );
}

export function checkToInput(check: Check): CheckInput {
  return {
    resourceId: check.resourceId,
    name: check.name,
    url: check.url,
    method: check.method === 'HEAD' ? 'HEAD' : 'GET',
    intervalSeconds: Math.round(check.interval / 1_000_000_000),
    timeoutSeconds: Math.round(check.timeout / 1_000_000_000),
    expectedStatusMin: check.expectedStatusMin,
    expectedStatusMax: check.expectedStatusMax,
    bodySubstring: check.bodySubstring ?? '',
    required: check.required,
    enabled: check.enabled,
  };
}

export function failureLabel(code: string | undefined): string {
  switch (code) {
    case 'dns':
      return 'DNS lookup failed';
    case 'timeout':
      return 'Timed out';
    case 'connection':
      return 'Connection refused';
    case 'tls_handshake':
      return 'TLS handshake failed';
    case 'unexpected_status':
      return 'Unexpected status';
    case 'body_mismatch':
      return 'Body mismatch';
    case 'target_blocked':
      return 'Target blocked';
    default:
      return code ? code.replaceAll('_', ' ') : 'Failed';
  }
}
