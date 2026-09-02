import { api } from './client';

/* MFA (advanced-auth feature gate) */

export function mfaStatus() {
  return api.get<{ enabled: boolean }>('/api/v1/auth/mfa', {
    fallback: 'MFA status is unavailable.',
  });
}

export function enrollMfa(password: string) {
  return api.post<{ seed: string; uri: string; expiresAt: string }>(
    '/api/v1/auth/mfa/enroll',
    { password },
    {
      fallback: 'Enrollment could not start.',
    },
  );
}

export function confirmMfa(code: string) {
  return api.post<{ recoveryCodes: string[] }>(
    '/api/v1/auth/mfa/confirm',
    { code },
    {
      fallback: 'The code could not be confirmed.',
    },
  );
}

export function disableMfa(password: string, code: string) {
  return api.post(
    '/api/v1/auth/mfa/disable',
    { password, code },
    { fallback: 'MFA could not be disabled.' },
  );
}

/* Coolify enrichment */

export interface CoolifyStatus {
  enabled: boolean;
  url?: string;
  tokenConfigured: boolean;
  environmentAuthoritative: boolean;
  collector: {
    state: 'unknown' | 'healthy' | 'degraded' | string;
    lastAttemptAt?: string;
    lastSuccessAt?: string;
    errorCode?: string;
    resources: number;
  };
}

export function coolifyStatus(signal?: AbortSignal) {
  return api.get<CoolifyStatus>('/api/v1/integrations/coolify', {
    signal,
    fallback: 'Coolify status is unavailable.',
  });
}

export function saveCoolify(url: string, token: string) {
  return api.put<CoolifyStatus>(
    '/api/v1/integrations/coolify',
    { URL: url, Token: token },
    {
      fallback: 'Coolify settings could not be saved.',
    },
  );
}

export function testCoolify(url: string, token: string) {
  return api.post(
    '/api/v1/integrations/coolify/test',
    { URL: url, Token: token },
    {
      fallback: 'Coolify could not be reached.',
    },
  );
}

/* API tokens (portability feature gate) */

export type TokenScope =
  | 'server:read'
  | 'resources:read'
  | 'metrics:read'
  | 'events:read'
  | 'incidents:read';

export interface ApiToken {
  id: string;
  name: string;
  prefix: string;
  scopes: TokenScope[];
  createdAt: string;
  expiresAt?: string;
  lastUsedAt?: string;
  revokedAt?: string;
}

export const scopeLabels: Record<TokenScope, string> = {
  'server:read': 'Host metrics',
  'resources:read': 'Resources',
  'metrics:read': 'History and Prometheus',
  'events:read': 'Events',
  'incidents:read': 'Incidents',
};

export function listTokens(signal?: AbortSignal) {
  return api.get<{ tokens: ApiToken[]; scopes: TokenScope[] }>(
    '/api/v1/api-tokens',
    {
      signal,
      fallback: 'API tokens are unavailable.',
    },
  );
}

export function createToken(input: {
  name: string;
  scopes: TokenScope[];
  expiresAt?: string;
}) {
  return api.post<{ token: ApiToken; plaintext: string }>(
    '/api/v1/api-tokens',
    input,
    {
      fallback: 'The token could not be created.',
    },
  );
}

export function revokeToken(id: string) {
  return api.delete(`/api/v1/api-tokens/${encodeURIComponent(id)}`, {
    fallback: 'The token could not be revoked.',
  });
}

/* Self-monitoring and diagnostics */

export interface MonitorMetric {
  id: string;
  label: string;
  value: number | string | null;
  unit?: string;
  status: string;
  help: string;
}

export function monitorHealth(signal?: AbortSignal) {
  return api.get<{ at: string; metrics: MonitorMetric[] }>(
    '/api/v1/monitor-health',
    {
      signal,
      fallback: 'Monitor health is unavailable.',
    },
  );
}

export interface DiagnosticsPreview {
  id: string;
  createdAt: string;
  expiresAt: string;
  fields: Record<string, unknown>;
  partialFailures?: string[];
}

export function createDiagnosticsPreview() {
  return api.post<DiagnosticsPreview>(
    '/api/v1/diagnostics/previews',
    undefined,
    {
      fallback: 'Diagnostics could not be generated.',
    },
  );
}

export function diagnosticsDownloadUrl(id: string) {
  return `/api/v1/diagnostics/previews/${encodeURIComponent(id)}/download`;
}

/* History deletion */

export type DeletionKind = 'before' | 'resource' | 'archived_resource' | 'all';

export interface DeletionPreview {
  token: string;
  confirmation: string;
  totalRows: number;
  expiresAt: string;
  scope?: string;
  fenceAt?: string;
}

export interface DeletionJob {
  id: string;
  kind: DeletionKind;
  resourceId?: string;
  state:
    | 'queued'
    | 'running'
    | 'cancelling'
    | 'completed'
    | 'failed'
    | 'cancelled'
    | string;
  totalRows: number;
  deletedRows: number;
  error?: string;
}

export function previewDeletion(input: {
  kind: DeletionKind;
  before?: string;
  resourceId?: string;
}) {
  return api.post<DeletionPreview>('/api/v1/history/deletion-previews', input, {
    fallback: 'The deletion preview could not be created.',
  });
}

export function startDeletion(token: string, confirmation: string) {
  return api.post<DeletionJob>(
    '/api/v1/history/deletion-jobs',
    { token, confirmation },
    {
      fallback: 'The deletion could not be started.',
    },
  );
}

export function deletionJob(id: string) {
  return api.get<DeletionJob>(
    `/api/v1/history/deletion-jobs/${encodeURIComponent(id)}`,
    {
      fallback: 'The deletion job is unavailable.',
    },
  );
}

export function controlDeletion(id: string, action: 'cancel' | 'retry') {
  return api.post(
    `/api/v1/history/deletion-jobs/${encodeURIComponent(id)}/${action}`,
    undefined,
    {
      fallback: 'The deletion job could not be updated.',
    },
  );
}
