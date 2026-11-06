import { describe, expect, it } from 'vitest';
import { silenceSummary } from './silence-summary';

const base = {
  scope: 'resource' as const,
  scopeId: 'res_1',
  resourceName: 'api.production',
  preset: '1h' as const,
  customEnd: '',
};

describe('silenceSummary', () => {
  it('names the resource, never its id', () => {
    const text = silenceSummary(base);
    expect(text).toMatch(/^Notifications for api\.production pause until /);
    expect(text).not.toContain('res_1');
  });

  it('is empty until the form says what to silence and for how long', () => {
    expect(silenceSummary({ ...base, scopeId: '' })).toBe('');
    expect(silenceSummary({ ...base, preset: 'custom' })).toBe('');
  });

  it('covers the server, projects and the tomorrow preset', () => {
    expect(
      silenceSummary({
        ...base,
        scope: 'server',
        scopeId: '',
        preset: 'tomorrow',
      }),
    ).toBe(
      'Notifications for the whole server pause until tomorrow. Alerts keep evaluating.',
    );
    expect(
      silenceSummary({ ...base, scope: 'project', scopeId: 'shop' }),
    ).toContain('project shop');
  });
});
