import { api } from './api/client';

export type Theme = 'system' | 'dark' | 'light';
export type Density = 'comfortable' | 'compact';
export type LandingPage =
  'overview' | 'resources' | 'host' | 'alerts' | 'activity' | 'logs';
/** Values written by releases before the Overview/Host/Activity rename. */
type LegacyLandingPage = 'watch' | 'server' | 'events';
export type ChartRange = '1h' | '6h' | '24h' | '7d' | '30d';

export interface UserPreferences {
  schemaVersion: 1;
  theme: Theme;
  density: Density;
  pinnedResources: string[];
  landingPage: LandingPage;
  chartRange: ChartRange;
  updatedAt?: string;
}

export const landingPages: LandingPage[] = [
  'overview',
  'resources',
  'host',
  'alerts',
  'activity',
  'logs',
];
export const chartRanges: ChartRange[] = ['1h', '6h', '24h', '7d', '30d'];

const themeKey = 'binnacle.theme';
const densityKey = 'binnacle.density';
const mirrorKey = 'binnacle.preferences.v1';

export const defaultPreferences: UserPreferences = {
  schemaVersion: 1,
  theme: 'dark',
  density: 'comfortable',
  pinnedResources: [],
  landingPage: 'overview',
  chartRange: '24h',
};

const legacyLanding: Record<LegacyLandingPage, LandingPage> = {
  watch: 'overview',
  server: 'host',
  events: 'activity',
};

export function normalizeLandingPage(value: unknown): LandingPage | null {
  if (typeof value !== 'string') return null;
  if ((landingPages as string[]).includes(value)) return value as LandingPage;
  return legacyLanding[value as LegacyLandingPage] ?? null;
}

export function resolveTheme(
  theme: Theme,
  dark = matchMedia('(prefers-color-scheme: dark)').matches,
): 'dark' | 'light' {
  return theme === 'system' ? (dark ? 'dark' : 'light') : theme;
}

export function preferences(storage: Storage = localStorage): UserPreferences {
  try {
    const mirror = JSON.parse(storage.getItem(mirrorKey) ?? 'null') as unknown;
    if (validPreferences(mirror)) return normalizePreferences(mirror);
  } catch {
    // Fall through to the legacy keys for the one-time server migration.
  }
  const theme = storage.getItem(themeKey);
  const density = storage.getItem(densityKey);
  return {
    ...defaultPreferences,
    theme:
      theme === 'dark' || theme === 'light' || theme === 'system'
        ? theme
        : defaultPreferences.theme,
    density:
      density === 'compact' || density === 'comfortable'
        ? density
        : defaultPreferences.density,
  };
}

/** Applies the theme and density to the document and mirrors locally. */
export function applyPreferences(
  value: UserPreferences,
  storage: Storage = localStorage,
) {
  const normalized = normalizePreferences(value);
  storage.setItem(themeKey, normalized.theme);
  storage.setItem(densityKey, normalized.density);
  storage.setItem(mirrorKey, JSON.stringify(normalized));
  document.documentElement.dataset.theme = resolveTheme(normalized.theme);
  document.documentElement.dataset.density = normalized.density;
  window.dispatchEvent(
    new CustomEvent('binnacle:preferences', { detail: normalized }),
  );
  return normalized;
}

export async function loadServerPreferences(): Promise<UserPreferences> {
  const body = await api.get<{
    exists: boolean;
    preferences?: UserPreferences;
  }>('/api/v1/preferences', { fallback: 'Preferences could not be loaded.' });
  const value =
    body.exists &&
    validPreferences({
      ...body.preferences,
      pinnedResources: body.preferences?.pinnedResources ?? [],
    })
      ? normalizePreferences(body.preferences as UserPreferences)
      : await saveServerPreferences(preferences());
  return applyPreferences(value);
}

export async function saveServerPreferences(
  value: UserPreferences,
): Promise<UserPreferences> {
  if (!validPreferences(value)) throw new Error('Preferences are invalid.');
  const saved = await api.put<UserPreferences>(
    '/api/v1/preferences',
    normalizePreferences(value),
    { fallback: 'Preferences could not be saved.' },
  );
  const normalized = saved
    ? { ...saved, pinnedResources: saved.pinnedResources ?? [] }
    : null;
  if (!normalized || !validPreferences(normalized))
    throw new Error('Preferences could not be saved.');
  return applyPreferences(normalized);
}

export function normalizePreferences(value: UserPreferences): UserPreferences {
  return {
    ...value,
    pinnedResources: value.pinnedResources ?? [],
    landingPage: normalizeLandingPage(value.landingPage) ?? 'overview',
  };
}

export function validPreferences(value: unknown): value is UserPreferences {
  if (!value || typeof value !== 'object') return false;
  const candidate = value as Partial<UserPreferences>;
  return (
    candidate.schemaVersion === 1 &&
    ['system', 'dark', 'light'].includes(candidate.theme ?? '') &&
    ['comfortable', 'compact'].includes(candidate.density ?? '') &&
    normalizeLandingPage(candidate.landingPage) !== null &&
    (chartRanges as string[]).includes(candidate.chartRange ?? '') &&
    Array.isArray(candidate.pinnedResources) &&
    candidate.pinnedResources.length <= 12 &&
    new Set(candidate.pinnedResources).size ===
      candidate.pinnedResources.length &&
    candidate.pinnedResources.every(
      (id) => typeof id === 'string' && id.length > 0 && id.length <= 128,
    )
  );
}
