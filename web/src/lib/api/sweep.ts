import { api } from './client';
import type { CheckFailureCode } from './checks';

/** The sweep shares its failure codes with the HTTP checks. */
export type SweepFailureCode = Exclude<
  CheckFailureCode,
  'unexpected_status' | 'body_mismatch'
>;

/** One probe of a domain, as returned by "run now". */
export interface SweepResult {
  domainId: string;
  checkedAt: string;
  httpStatus?: number;
  failureCode?: SweepFailureCode | '';
  finalUrl?: string;
  certIssuer?: string;
  certNotAfter?: string;
  /** Whole days until the certificate expires; negative once expired. */
  certDaysLeft?: number;
  nsPresent: boolean;
  resolves: boolean;
  ns?: string[];
  addresses?: string[];
  spf?: string;
  dmarcPolicy?: string;
  hsts: boolean;
  csp: boolean;
  fingerprint: string;
}

/** The stored latest result of a domain plus its schedule. */
export interface SweepState extends SweepResult {
  nextRunAt: string;
  consecutiveFailures: number;
}

export interface SweptDomain {
  id: string;
  domain: string;
  /** Nanoseconds, as serialized by Go. */
  interval: number;
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
  state?: SweepState;
}

export interface DomainInput {
  domain?: string;
  intervalSeconds?: number;
  enabled?: boolean;
}

/** The default of the certificate rule, in days. */
export const certExpiringDays = 14;

export function listDomains(signal?: AbortSignal) {
  return api.get<SweptDomain[]>('/api/v1/sweep/domains', {
    signal,
    fallback: 'Swept domains are unavailable.',
  });
}

export function createDomain(input: DomainInput) {
  return api.post<SweptDomain>('/api/v1/sweep/domains', input, {
    fallback: 'The domain could not be added.',
  });
}

export function updateDomain(id: string, input: DomainInput) {
  return api.patch<SweptDomain>(
    `/api/v1/sweep/domains/${encodeURIComponent(id)}`,
    input,
    { fallback: 'The domain could not be updated.' },
  );
}

export function deleteDomain(id: string) {
  return api.delete(`/api/v1/sweep/domains/${encodeURIComponent(id)}`, {
    fallback: 'The domain could not be removed.',
  });
}

export function runDomain(id: string) {
  return api.post<SweepResult>(
    `/api/v1/sweep/domains/${encodeURIComponent(id)}/run`,
    undefined,
    { fallback: 'The domain could not be swept.' },
  );
}

/** Unreachable: no response or a 5xx (a Cloudflare 525 included). */
export function unreachable(state: SweepResult): boolean {
  return (
    Boolean(state.failureCode) || !state.httpStatus || state.httpStatus >= 500
  );
}

export type DomainTone = 'ok' | 'warn' | 'critical' | 'neutral';

/**
 * One word per domain, in the order the alert families matter: being down
 * outranks a missing record, which outranks a missing header.
 */
export function domainStatus(domain: SweptDomain): {
  tone: DomainTone;
  label: string;
} {
  if (!domain.enabled) return { tone: 'neutral', label: 'Disabled' };
  const state = domain.state;
  if (!state) return { tone: 'neutral', label: 'Not swept yet' };
  if (unreachable(state)) return { tone: 'critical', label: 'Unreachable' };
  if (!state.nsPresent || !state.resolves)
    return { tone: 'critical', label: 'DNS missing' };
  if (state.certDaysLeft != null && state.certDaysLeft <= certExpiringDays)
    return { tone: 'warn', label: 'Certificate expiring' };
  if (!state.spf || !state.dmarcPolicy)
    return { tone: 'warn', label: 'Mail auth missing' };
  if (!state.hsts || !state.csp)
    return { tone: 'warn', label: 'Headers missing' };
  return { tone: 'ok', label: 'Healthy' };
}

/** Days left on the certificate as a short word. */
export function certLabel(days: number | null | undefined): string {
  if (days == null) return '—';
  if (days < 0) return 'expired';
  if (days === 0) return 'today';
  return `${days} d`;
}

/** The HTTP column: the status, or the failure when there was no response. */
export function httpLabel(state: SweepResult | undefined): string {
  if (!state) return '—';
  if (state.httpStatus) return String(state.httpStatus);
  return state.failureCode ? state.failureCode.replaceAll('_', ' ') : '—';
}
