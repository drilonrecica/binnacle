import { describe, expect, it } from 'vitest';
import { incidentTitle } from './incidents';

describe('incidentTitle', () => {
  it('replaces the raw resource id with its name (UI-18)', () => {
    expect(
      incidentTitle(
        { title: 'resource incident on res_01HZXQ', targetId: 'res_01HZXQ' },
        'api.production',
      ),
    ).toBe('Resource incident on api.production');
  });

  it('keeps titles that carry no id', () => {
    expect(
      incidentTitle(
        { title: 'Demo service health incident', targetId: 'res_demo_2' },
        'demo',
      ),
    ).toBe('Demo service health incident');
  });

  it('keeps the title when no name is known', () => {
    expect(
      incidentTitle({ title: 'resource incident on r1', targetId: 'r1' }, ''),
    ).toBe('resource incident on r1');
  });
});
