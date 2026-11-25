import { describe, expect, it } from 'vitest';
import {
  certLabel,
  domainStatus,
  httpLabel,
  unreachable,
  type SweepState,
  type SweptDomain,
} from './sweep';

function state(patch: Partial<SweepState> = {}): SweepState {
  return {
    domainId: 'd1',
    checkedAt: '2026-11-25T12:00:00Z',
    nextRunAt: '2026-11-25T13:00:00Z',
    httpStatus: 200,
    certDaysLeft: 60,
    nsPresent: true,
    resolves: true,
    spf: 'v=spf1 -all',
    dmarcPolicy: 'reject',
    hsts: true,
    csp: true,
    fingerprint: 'fp',
    consecutiveFailures: 0,
    ...patch,
  };
}

function domain(patch: Partial<SweptDomain> = {}): SweptDomain {
  return {
    id: 'd1',
    domain: 'example.com',
    interval: 3_600_000_000_000,
    enabled: true,
    createdAt: '2026-11-25T10:00:00Z',
    updatedAt: '2026-11-25T10:00:00Z',
    state: state(),
    ...patch,
  };
}

describe('domainStatus', () => {
  it('names a healthy domain', () => {
    expect(domainStatus(domain())).toEqual({ tone: 'ok', label: 'Healthy' });
  });

  it('ranks being down above a missing record above a missing header', () => {
    const everything = state({
      httpStatus: 525,
      nsPresent: false,
      certDaysLeft: 2,
      spf: '',
      hsts: false,
    });
    expect(domainStatus(domain({ state: everything })).label).toBe(
      'Unreachable',
    );
    expect(
      domainStatus(domain({ state: { ...everything, httpStatus: 200 } })).label,
    ).toBe('DNS missing');
    expect(
      domainStatus(
        domain({ state: { ...everything, httpStatus: 200, nsPresent: true } }),
      ),
    ).toEqual({ tone: 'warn', label: 'Certificate expiring' });
    expect(
      domainStatus(
        domain({
          state: {
            ...everything,
            httpStatus: 200,
            nsPresent: true,
            certDaysLeft: 60,
          },
        }),
      ).label,
    ).toBe('Mail auth missing');
    expect(domainStatus(domain({ state: state({ csp: false }) }))).toEqual({
      tone: 'warn',
      label: 'Headers missing',
    });
  });

  it('treats a 525 and a failure code as unreachable', () => {
    expect(unreachable(state({ httpStatus: 525 }))).toBe(true);
    expect(
      unreachable(state({ httpStatus: undefined, failureCode: 'timeout' })),
    ).toBe(true);
    expect(unreachable(state({ httpStatus: 404 }))).toBe(false);
  });

  it('says when a domain is disabled or not swept yet', () => {
    expect(domainStatus(domain({ enabled: false })).label).toBe('Disabled');
    expect(domainStatus(domain({ state: undefined }))).toEqual({
      tone: 'neutral',
      label: 'Not swept yet',
    });
  });
});

describe('labels', () => {
  it('words the certificate days', () => {
    expect(certLabel(60)).toBe('60 d');
    expect(certLabel(0)).toBe('today');
    expect(certLabel(-1)).toBe('expired');
    expect(certLabel(undefined)).toBe('—');
  });

  it('shows the status or the failure in the HTTP column', () => {
    expect(httpLabel(state())).toBe('200');
    expect(
      httpLabel(state({ httpStatus: undefined, failureCode: 'tls_handshake' })),
    ).toBe('tls handshake');
    expect(httpLabel(undefined)).toBe('—');
  });
});
