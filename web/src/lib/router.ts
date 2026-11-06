/**
 * Pure routing helpers. No DOM access, so they are unit-testable in node.
 * The reactive router store lives in `router.svelte.ts`.
 */

import type { LandingPage } from './preferences';

export type RouteName =
  | 'headroom'
  | 'resources'
  | 'resource'
  | 'server'
  | 'incidents'
  | 'incident'
  | 'changes'
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
  { name: 'headroom', pattern: '/headroom' },
  { name: 'resources', pattern: '/resources' },
  { name: 'resource', pattern: '/resources/:id' },
  { name: 'server', pattern: '/server' },
  { name: 'incidents', pattern: '/incidents' },
  { name: 'incident', pattern: '/incidents/:id' },
  { name: 'changes', pattern: '/changes' },
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
  'alerting',
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
      const token = expected[index] ?? '';
      if (token.startsWith(':')) {
        try {
          params[token.slice(1)] = decodeURIComponent(actual[index] ?? '');
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

/** The first screen after sign-in when nothing else is asked for. */
export const homePath = '/headroom';

/** Retired paths and their replacements. The query string is kept. */
const legacy: Record<string, string> = {
  '/': homePath,
  '/watch': homePath,
  '/overview': homePath,
  '/host': '/server',
  '/alerts': '/incidents',
  '/activity': '/changes',
  '/events': '/changes',
  '/settings/monitor-health': '/settings/system',
  '/settings/diagnostics': '/settings/system',
};

/**
 * Where the retired Logs page goes: the Logs tab of the resource it named,
 * else of the last resource viewed, else Resources with a prompt to pick one.
 */
export function resolveLogsPath(
  search: string,
  lastResource?: string | null,
): string {
  const query = new URLSearchParams(search);
  const resource = query.get('resource')?.trim() || lastResource?.trim();
  if (!resource) return '/resources?prompt=logs';
  return buildPath(resourcePath(resource), {
    tab: 'logs',
    container: query.get('resource')
      ? query.get('container')?.trim() || null
      : null,
    at: query.get('at')?.trim() || null,
  });
}

/**
 * Returns the replacement URL for a retired path, or null if none applies.
 * `lastLogsResource` is the resource whose detail was viewed last, for `/logs`.
 */
export function resolveLegacyPath(
  pathname: string,
  search: string,
  lastLogsResource?: string | null,
): string | null {
  const clean = pathname.length > 1 ? pathname.replace(/\/+$/, '') : pathname;
  if (clean === '/logs') return resolveLogsPath(search, lastLogsResource);
  if (clean === '/watch') {
    const inspect = new URLSearchParams(search).get('inspect')?.trim();
    if (inspect) return resourcePath(inspect);
    return homePath;
  }
  const incident = /^\/alerts\/incidents\/([^/]+)$/.exec(clean);
  if (incident) return `/incidents/${incident[1]}${search}`;
  const target = legacy[clean];
  return target ? `${target}${search}` : null;
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
  return `/incidents/${encodeURIComponent(id)}`;
}

/** The Logs tab of a resource, optionally centred on a moment. */
export function logsPath(resourceId: string, at?: string): string {
  return buildPath(resourcePath(resourceId), { tab: 'logs', at: at ?? null });
}

/**
 * The sign-in page with a `next` pointing back at `current`, so a session
 * that ends mid-visit resumes where it was. Public pages are never a target.
 */
export function loginPath(current: string): string {
  const { pathname } = splitHref(current);
  const match = matchRoute(pathname);
  const keep =
    pathname !== '/' &&
    (match
      ? !routeDefinition(match.name).public
      : resolveLegacyPath(pathname, '') !== null);
  return buildPath('/login', { next: keep ? current : null });
}

/** Paths for the stored start-page tokens, which predate the new names. */
const landingPaths: Record<LandingPage, string> = {
  overview: homePath,
  resources: '/resources',
  host: '/server',
  alerts: '/incidents',
  activity: '/changes',
  logs: '/logs',
};

export function landingPath(page: LandingPage): string {
  return landingPaths[page] ?? homePath;
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
