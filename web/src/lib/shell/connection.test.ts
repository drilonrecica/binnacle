import { describe, expect, it } from 'vitest';
import { connectionView, staleAfterMs } from './connection';

const at = Date.parse('2026-11-06T11:32:00Z');
const iso = new Date(at).toISOString();

describe('connectionView', () => {
  it('reads live while snapshots keep arriving', () => {
    const view = connectionView('connected', iso, at + 2_000);
    expect(view).toMatchObject({ tone: 'ok', label: 'Live', since: '' });
  });

  it('turns stale when a connected stream goes quiet, with a since and an age', () => {
    const view = connectionView('connected', iso, at + staleAfterMs + 50_000);
    expect(view.tone).toBe('warn');
    expect(view.label).toBe('Stale');
    expect(view.since).toMatch(/^since /);
    expect(view.age).toBe('1m');
  });

  it('keeps the state word fixed while the age ticks', () => {
    const early = connectionView('disconnected', iso, at + 20_000);
    const later = connectionView('disconnected', iso, at + 200_000);
    expect(early.label).toBe(later.label);
    expect(early.since).toBe(later.since);
    expect(early.age).not.toBe(later.age);
  });

  it('names every other state with a word, not only a colour', () => {
    expect(connectionView('connecting', null).label).toBe('Connecting');
    expect(connectionView('disconnected', null)).toMatchObject({
      tone: 'critical',
      label: 'Disconnected',
      since: '',
    });
    expect(connectionView('unauthorized', null).label).toBe('Signed out');
  });
});
