import { describe, expect, it } from 'vitest';
import type { Check } from '../api/checks';
import { checkStatus, nextRunAt, nextRunLabel } from './CheckStatusTab.svelte';

function check(patch: Partial<Check> = {}): Check {
  return {
    id: 'c1',
    resourceId: 'r1',
    name: 'API',
    url: 'https://example.com/health',
    method: 'GET',
    interval: 30_000_000_000,
    timeout: 5_000_000_000,
    expectedStatusMin: 200,
    expectedStatusMax: 299,
    required: true,
    enabled: true,
    createdAt: '2026-11-06T10:00:00Z',
    updatedAt: '2026-11-06T10:00:00Z',
    state: {
      checkId: 'c1',
      status: 'success',
      httpStatus: 200,
      latencyMs: 42,
      checkedAt: '2026-11-06T10:00:00Z',
      consecutiveSuccesses: 3,
      consecutiveFailures: 0,
    },
    ...patch,
  };
}

describe('check status', () => {
  it('schedules the next run one interval after the last', () => {
    expect(nextRunAt(check())).toBe(Date.parse('2026-11-06T10:00:30Z'));
    expect(nextRunAt(check({ enabled: false }))).toBeNull();
    expect(nextRunAt(check({ state: undefined }))).toBeNull();
  });

  it('words near runs instead of calling them "just now"', () => {
    const at = Date.parse('2026-11-06T10:00:30Z');
    expect(nextRunLabel(at, at - 20_000)).toBe('in <1m');
    expect(nextRunLabel(at, at + 1)).toBe('due now');
    expect(nextRunLabel(at, at - 5 * 60_000)).toBe('in 5m');
    expect(nextRunLabel(null)).toBe('—');
  });

  it('names every state with a word and a tone', () => {
    expect(checkStatus(check())).toEqual({ tone: 'ok', label: 'Passing' });
    const failing = check({
      state: { ...check().state!, status: 'failure', failureCode: 'timeout' },
    });
    expect(checkStatus(failing)).toEqual({
      tone: 'critical',
      label: 'Failing',
    });
    expect(checkStatus({ ...failing, required: false }).tone).toBe('warn');
    expect(checkStatus(check({ enabled: false })).label).toBe('Disabled');
    expect(checkStatus(check({ state: undefined })).label).toBe('Not run yet');
  });
});
