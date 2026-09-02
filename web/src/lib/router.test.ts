import { describe, expect, it } from 'vitest';
import {
  buildPath,
  matchRoute,
  resolveLegacyPath,
  routes,
  type RouteName,
} from './router';

describe('matchRoute', () => {
  it('matches top-level pages', () => {
    expect(matchRoute('/overview')).toEqual({
      name: 'overview',
      params: {},
    });
    expect(matchRoute('/alerts')).toEqual({ name: 'alerts', params: {} });
  });

  it('matches parameterized routes and decodes params', () => {
    expect(matchRoute('/resources/res_a%20b')).toEqual({
      name: 'resource',
      params: { id: 'res_a b' },
    });
    expect(matchRoute('/alerts/incidents/inc_1')).toEqual({
      name: 'incident',
      params: { id: 'inc_1' },
    });
    expect(matchRoute('/settings/access')).toEqual({
      name: 'settings',
      params: { section: 'access' },
    });
  });

  it('treats bare /settings as the general section', () => {
    expect(matchRoute('/settings')).toEqual({
      name: 'settings',
      params: { section: 'general' },
    });
  });

  it('returns null for unknown paths and trailing garbage', () => {
    expect(matchRoute('/nope')).toBeNull();
    expect(matchRoute('/overview/extra')).toBeNull();
  });

  it('ignores a trailing slash', () => {
    expect(matchRoute('/resources/')).toEqual({
      name: 'resources',
      params: {},
    });
  });

  it('covers every declared route name', () => {
    const names = new Set<RouteName>(routes.map((route) => route.name));
    for (const name of [
      'overview',
      'resources',
      'resource',
      'host',
      'alerts',
      'incident',
      'activity',
      'logs',
      'settings',
      'login',
      'setup',
      'onboarding',
    ] as const) {
      expect(names.has(name)).toBe(true);
    }
  });
});

describe('resolveLegacyPath', () => {
  it('maps retired paths to their replacements', () => {
    expect(resolveLegacyPath('/watch', '')).toBe('/overview');
    expect(resolveLegacyPath('/server', '')).toBe('/host');
    expect(resolveLegacyPath('/events', '')).toBe('/activity');
    expect(resolveLegacyPath('/settings/monitor-health', '')).toBe(
      '/settings/system',
    );
    expect(resolveLegacyPath('/settings/diagnostics', '')).toBe(
      '/settings/system',
    );
  });

  it('sends the watch inspector deep link to the resource page', () => {
    expect(resolveLegacyPath('/watch', '?inspect=res_1')).toBe(
      '/resources/res_1',
    );
  });

  it('sends the root to the overview', () => {
    expect(resolveLegacyPath('/', '')).toBe('/overview');
  });

  it('leaves current paths alone', () => {
    expect(resolveLegacyPath('/overview', '?x=1')).toBeNull();
    expect(resolveLegacyPath('/resources/res_1', '')).toBeNull();
  });
});

describe('buildPath', () => {
  it('merges query params and drops empty values', () => {
    expect(buildPath('/alerts', { tab: 'rules', empty: '' })).toBe(
      '/alerts?tab=rules',
    );
  });

  it('returns the bare path when nothing is set', () => {
    expect(buildPath('/logs', {})).toBe('/logs');
  });

  it('can start from an existing query string', () => {
    expect(
      buildPath('/activity', { severity: 'critical' }, '?range=6h&severity=x'),
    ).toBe('/activity?range=6h&severity=critical');
  });
});
