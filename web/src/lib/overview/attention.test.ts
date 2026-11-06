import { describe, expect, it } from 'vitest';
import type { Incident } from '../api/incidents';
import type { LiveSnapshot } from '../live.svelte';
import {
  attentionItems,
  attentionState,
  type AttentionInput,
} from './attention';

const now = Date.parse('2026-11-06T12:00:00Z');

const snapshot: LiveSnapshot = {
  seq: 1,
  ts: '2026-11-06T12:00:00Z',
  bootIdentity: 'boot-abc123',
  host: {},
  resources: [
    {
      id: 'res_01HZX',
      name: 'api.production',
      status: 'healthy',
      lastSeenAt: '2026-11-06T11:59:59Z',
    },
    {
      id: 'res_02HZX',
      name: 'worker.production',
      status: 'down',
      lastSeenAt: '2026-11-06T11:59:59Z',
    },
  ],
  collectors: { docker: { state: 'healthy' } },
  filesystems: [],
};

const incident: Incident = {
  id: 'inc-1',
  // The server stores titles with the raw target id (UI-18).
  title: 'resource incident on res_01HZX',
  status: 'open',
  severity: 'critical',
  targetType: 'resource',
  targetId: 'res_01HZX',
  alertCount: 2,
  firingAlertCount: 2,
  openedAt: '2026-11-06T11:40:00Z',
};

function input(partial: Partial<AttentionInput> = {}): AttentionInput {
  return {
    snapshot,
    connection: 'live',
    lastReceivedAt: '2026-11-06T11:59:59Z',
    incidents: [],
    incidentsLoadedAt: '2026-11-06T11:59:30Z',
    incidentsFailed: false,
    openedAt: '2026-11-06T11:30:00Z',
    now,
    ...partial,
  };
}

describe('attention items', () => {
  it('names incidents and resources without raw ids', () => {
    const items = attentionItems(snapshot, [incident]);
    expect(items.map((item) => [item.title, item.detail])).toEqual([
      ['api.production', 'Critical incident · 2 alerts firing'],
      ['worker.production', 'Down'],
    ]);
    for (const item of items) {
      expect(`${item.title} ${item.detail}`).not.toMatch(/res_0/);
    }
  });
});

describe('attention state (UI-1)', () => {
  it('shows the all-clear only with live data and loaded incidents', () => {
    const healthy = {
      ...snapshot,
      resources: snapshot.resources.slice(0, 1),
    };
    expect(attentionState(input({ snapshot: healthy })).kind).toBe('clear');
    expect(
      attentionState(input({ snapshot: healthy, incidents: null })).kind,
    ).toBe('loading');
  });

  it('says status unknown since the last update when the stream is stale', () => {
    const quiet = input({ lastReceivedAt: '2026-11-06T11:58:00Z' });
    expect(attentionState(quiet)).toMatchObject({
      kind: 'unknown',
      reason: 'stream',
      since: '2026-11-06T11:58:00Z',
      items: [],
    });
    expect(attentionState(input({ connection: 'disconnected' })).kind).toBe(
      'unknown',
    );
  });

  it('says status unknown when the incident fetch failed', () => {
    const state = attentionState(
      input({ incidentsFailed: true, incidentsLoadedAt: null }),
    );
    expect(state).toMatchObject({
      kind: 'unknown',
      reason: 'incidents',
      since: '2026-11-06T11:30:00Z',
    });
    // What the live stream does know is still listed.
    expect(state.kind === 'unknown' && state.items[0]?.title).toBe(
      'worker.production',
    );
  });
});
