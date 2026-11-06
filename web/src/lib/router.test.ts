import { describe, expect, it } from 'vitest';
import {
  buildPath,
  incidentPath,
  landingPath,
  loginPath,
  matchRoute,
  resolveLegacyPath,
  routes,
  type RouteName,
} from './router';
import { landingPages } from './preferences';

describe('matchRoute', () => {
  it('matches the six pages of the information architecture', () => {
    for (const [path, name] of [
      ['/headroom', 'headroom'],
      ['/resources', 'resources'],
      ['/server', 'server'],
      ['/incidents', 'incidents'],
      ['/changes', 'changes'],
      ['/settings', 'settings'],
    ] as const)
      expect(matchRoute(path)?.name).toBe(name);
  });

  it('matches parameterized routes and decodes params', () => {
    expect(matchRoute('/resources/res_a%20b')).toEqual({
      name: 'resource',
      params: { id: 'res_a b' },
    });
    expect(matchRoute('/incidents/inc_1')).toEqual({
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

  it('returns null for unknown paths, retired names and trailing garbage', () => {
    expect(matchRoute('/nope')).toBeNull();
    expect(matchRoute('/headroom/extra')).toBeNull();
    expect(matchRoute('/overview')).toBeNull();
    expect(matchRoute('/alerts')).toBeNull();
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
      'headroom',
      'resources',
      'resource',
      'server',
      'incidents',
      'incident',
      'changes',
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
  it('maps retired page names to the new ones', () => {
    expect(resolveLegacyPath('/overview', '')).toBe('/headroom');
    expect(resolveLegacyPath('/watch', '')).toBe('/headroom');
    expect(resolveLegacyPath('/host', '')).toBe('/server');
    expect(resolveLegacyPath('/alerts', '')).toBe('/incidents');
    expect(resolveLegacyPath('/activity', '')).toBe('/changes');
    expect(resolveLegacyPath('/events', '')).toBe('/changes');
    expect(resolveLegacyPath('/settings/monitor-health', '')).toBe(
      '/settings/system',
    );
    expect(resolveLegacyPath('/settings/diagnostics', '')).toBe(
      '/settings/system',
    );
  });

  it('keeps the query so old deep links land on the same tab', () => {
    expect(resolveLegacyPath('/alerts', '?tab=silences&new=1')).toBe(
      '/incidents?tab=silences&new=1',
    );
    expect(resolveLegacyPath('/host/', '?tab=collectors')).toBe(
      '/server?tab=collectors',
    );
  });

  it('moves incident detail links under /incidents', () => {
    expect(resolveLegacyPath('/alerts/incidents/inc_1', '')).toBe(
      '/incidents/inc_1',
    );
  });

  it('sends the watch inspector deep link to the resource page', () => {
    expect(resolveLegacyPath('/watch', '?inspect=res_1')).toBe(
      '/resources/res_1',
    );
  });

  it('sends the root to Headroom', () => {
    expect(resolveLegacyPath('/', '')).toBe('/headroom');
  });

  it('leaves current paths alone', () => {
    expect(resolveLegacyPath('/headroom', '?x=1')).toBeNull();
    expect(resolveLegacyPath('/server', '')).toBeNull();
    expect(resolveLegacyPath('/resources/res_1', '')).toBeNull();
  });
});

describe('loginPath', () => {
  it('carries the current page as next', () => {
    expect(loginPath('/resources/res_1?range=6h')).toBe(
      `/login?next=${encodeURIComponent('/resources/res_1?range=6h')}`,
    );
    expect(loginPath('/alerts?tab=checks')).toBe(
      `/login?next=${encodeURIComponent('/alerts?tab=checks')}`,
    );
  });

  it('drops next for the root, public pages and unknown paths', () => {
    expect(loginPath('/')).toBe('/login');
    expect(loginPath('/login?next=%2Fserver')).toBe('/login');
    expect(loginPath('/setup')).toBe('/login');
    expect(loginPath('/nope')).toBe('/login');
  });
});

describe('landingPath', () => {
  it('opens a routable page for every stored start-page token', () => {
    for (const page of landingPages)
      expect(matchRoute(landingPath(page))).not.toBeNull();
    expect(landingPath('overview')).toBe('/headroom');
    expect(landingPath('host')).toBe('/server');
    expect(landingPath('alerts')).toBe('/incidents');
    expect(landingPath('activity')).toBe('/changes');
  });
});

describe('path helpers', () => {
  it('builds incident links under /incidents', () => {
    expect(incidentPath('inc 1')).toBe('/incidents/inc%201');
  });
});

describe('buildPath', () => {
  it('merges query params and drops empty values', () => {
    expect(buildPath('/incidents', { tab: 'firing', empty: '' })).toBe(
      '/incidents?tab=firing',
    );
  });

  it('returns the bare path when nothing is set', () => {
    expect(buildPath('/logs', {})).toBe('/logs');
  });

  it('can start from an existing query string', () => {
    expect(
      buildPath('/changes', { severity: 'critical' }, '?range=6h&severity=x'),
    ).toBe('/changes?range=6h&severity=critical');
  });
});
