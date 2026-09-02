import { api } from './client';

export interface SettingValue {
  value: string;
  source: string;
  applyMode: 'live' | 'restart_required';
}

export interface SettingsSnapshot {
  revision: number;
  values: Record<string, SettingValue>;
  features: {
    advancedAuth: boolean;
    portability: boolean;
  };
}

export function loadSettings(signal?: AbortSignal) {
  return api.get<SettingsSnapshot>('/api/v1/settings', {
    signal,
    fallback: 'Settings are unavailable.',
  });
}

/** Applies up to 16 changes atomically against the given revision. */
export function saveSettings(
  revision: number,
  changes: Record<string, string>,
) {
  return api.patch<SettingsSnapshot>(
    '/api/v1/settings',
    { revision, changes },
    { fallback: 'Settings could not be saved.' },
  );
}

export const retentionPresets = [
  {
    value: 'minimal',
    title: 'Minimal',
    description: 'Lowest disk use.',
    tiers: {
      raw: '12h',
      one_minute: '7d',
      fifteen_minute: '90d',
      one_hour: '1y',
    },
  },
  {
    value: 'balanced',
    title: 'Balanced',
    description: 'Recommended for most installations.',
    tiers: {
      raw: '48h',
      one_minute: '30d',
      fifteen_minute: '1y',
      one_hour: 'off',
    },
  },
  {
    value: 'long-term',
    title: 'Long-term',
    description: 'More history and more disk use.',
    tiers: {
      raw: '7d',
      one_minute: '90d',
      fifteen_minute: '2y',
      one_hour: 'off',
    },
  },
  {
    value: 'advanced',
    title: 'Advanced',
    description: 'Set each tier yourself.',
    tiers: null,
  },
] as const;

export function sourceLabel(source: string): string {
  switch (source.toLowerCase()) {
    case 'default':
      return 'Default';
    case 'admin':
    case 'admin override':
      return 'Set here';
    case 'config file':
    case 'file':
      return 'Config file';
    case 'environment':
    case 'env':
      return 'Environment';
    default:
      return source;
  }
}
