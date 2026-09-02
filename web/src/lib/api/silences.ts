import { api } from './client';

export type SilenceScope = 'server' | 'project' | 'resource' | 'rule';
export type SilencePreset = '30m' | '1h' | '4h' | 'tomorrow' | 'custom';

export interface Silence {
  id: string;
  scopeType: SilenceScope;
  scopeId?: string;
  reason: string;
  startsAt: string;
  endsAt: string;
  createdBy?: string;
  createdAt?: string;
}

export interface NewSilence {
  scopeType: SilenceScope;
  scopeId?: string;
  reason: string;
  preset: SilencePreset;
  customEnd?: string;
}

export function listSilences(active = false, signal?: AbortSignal) {
  return api.get<Silence[]>('/api/v1/silences', {
    query: { active: active ? 'true' : undefined },
    signal,
    fallback: 'Silences are unavailable.',
  });
}

export function createSilence(value: NewSilence) {
  return api.post<Silence>('/api/v1/silences', value, {
    fallback: 'The silence could not be created.',
  });
}

export function deleteSilence(id: string) {
  return api.delete(`/api/v1/silences/${encodeURIComponent(id)}`, {
    fallback: 'The silence could not be removed.',
  });
}

export function silenceActive(silence: Silence, now = Date.now()): boolean {
  return new Date(silence.endsAt).getTime() > now;
}

export const silencePresets: Array<{ value: SilencePreset; label: string }> = [
  { value: '30m', label: '30 minutes' },
  { value: '1h', label: '1 hour' },
  { value: '4h', label: '4 hours' },
  { value: 'tomorrow', label: 'Until tomorrow' },
  { value: 'custom', label: 'Custom end' },
];
