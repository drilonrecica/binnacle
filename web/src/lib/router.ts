/**
 * Pure routing helpers. No DOM access, so they are unit-testable in node.
 * The reactive router store lives in `router.svelte.ts`.
 */

export type RouteName =
  | 'overview'
  | 'resources'
  | 'resource'
  | 'host'
  | 'alerts'
  | 'incident'
  | 'activity'
  | 'logs'
  | 'settings'
  | 'login'
  | 'setup'
  | 'onboarding';

export interface RouteDefinition {
  name: RouteName;
  pattern: string;
  /** Rendered without the authenticated shell. */
  bare?: boolean;
  /** Reachable without a session. */
  public?: boolean;
}

export interface RouteMatch {
  name: RouteName;
  params: Record<string, string>;
}

export const routes: RouteDefinition[] = [
  { name: 'overview', pattern: '/overview' },
  { name: 'resources', pattern: '/resources' },
  { name: 'resource', pattern: '/resources/:id' },
  { name: 'host', pattern: '/host' },
  { name: 'alerts', pattern: '/alerts' },
  { name: 'incident', pattern: '/alerts/incidents/:id' },
  { name: 'activity', pattern: '/activity' },
  { name: 'logs', pattern: '/logs' },
  { name: 'settings', pattern: '/settings' },
  { name: 'settings', pattern: '/settings/:section' },
  { name: 'login', pattern: '/login', bare: true, public: true },
  { name: 'setup', pattern: '/setup', bare: true, public: true },
  { name: 'onboarding', pattern: '/onboarding', bare: true },
];

export const settingsSections = [
  'general',
  'data',
  'access',
  'notifications',
  'appearance',
  'integrations',
  'system',
] as const;
export type SettingsSection = (typeof settingsSections)[number];

function segments(path: string): string[] {
  return path.split('/').filter(Boolean);
}

export function matchRoute(pathname: string): RouteMatch | null {
  const actual = segments(pathname);
  for (const route of routes) {
    const expected = segments(route.pattern);
    if (expected.length !== actual.length) continue;
    const params: Record<string, string> = {};
    let ok = true;
    for (let index = 0; index < expected.length; index++) {
      const token = expected[index];
      if (token.startsWith(':')) {
        try {
          params[token.slice(1)] = decodeURIComponent(actual[index]);
        } catch {
          ok = false;
          break;
        }
      } else if (token !== actual[index]) {
        ok = false;
        break;
      }
    }
    if (!ok) continue;
    if (route.name === 'settings' && !params.section)
      params.section = 'general';
    return { name: route.name, params };
  }
  return null;
}

export function routeDefinition(name: RouteName): RouteDefinition {
  const found = routes.find((route) => route.name === name);
  if (!found) throw new Error(`Unknown route ${name}`);
  return found;
}

const legacy: Record<string, string> = {
  '/': '/overview',
  '/watch': '/overview',
  '/server': '/host',
  '/events': '/activity',
  '/settings/monitor-health': '/settings/system',
  '/settings/diagnostics': '/settings/system',
};

/** Returns the replacement path for a retired URL, or null if none applies. */
export function resolveLegacyPath(
  pathname: string,
  search: string,
): string | null {
  const clean = pathname.length > 1 ? pathname.replace(/\/+$/, '') : pathname;
  if (clean === '/watch') {
    const inspect = new URLSearchParams(search).get('inspect')?.trim();
    if (inspect) return `/resources/${encodeURIComponent(inspect)}`;
  }
  return legacy[clean] ?? null;
}

/** Builds a path with the given query values merged over an existing query. */
export function buildPath(
  pathname: string,
  values: Record<string, string | null | undefined>,
  existing = '',
): string {
  const query = new URLSearchParams(existing);
  for (const [key, value] of Object.entries(values)) {
    if (value == null || value === '') query.delete(key);
    else query.set(key, value);
  }
  const encoded = query.toString();
  return encoded ? `${pathname}?${encoded}` : pathname;
}

export function resourcePath(id: string): string {
  return `/resources/${encodeURIComponent(id)}`;
}

export function incidentPath(id: string): string {
  return `/alerts/incidents/${encodeURIComponent(id)}`;
}

export function logsPath(resourceId: string, at?: string): string {
  return buildPath('/logs', { resource: resourceId, at: at ?? null });
}

/** Splits a same-origin href into path, search, and hash without the URL API. */
export function splitHref(href: string): {
  pathname: string;
  search: string;
  hash: string;
} {
  const hashIndex = href.indexOf('#');
  const hash = hashIndex >= 0 ? href.slice(hashIndex) : '';
  const withoutHash = hashIndex >= 0 ? href.slice(0, hashIndex) : href;
  const queryIndex = withoutHash.indexOf('?');
  const search = queryIndex >= 0 ? withoutHash.slice(queryIndex) : '';
  const pathname =
    queryIndex >= 0 ? withoutHash.slice(0, queryIndex) : withoutHash;
  return { pathname: pathname || '/', search, hash };
}
