import { api } from './client';
import type { Severity } from './incidents';

export interface Rule {
  id: string;
  family: string;
  name: string;
  builtIn: boolean;
  enabled: boolean;
  severity: Severity;
  scopeType:
    'global' | 'host' | 'filesystem' | 'project' | 'resource' | 'check';
  scopeId?: string;
  threshold?: number | null;
  recoveryThreshold?: number | null;
  triggerSeconds: number;
  recoverySeconds: number;
  windowSeconds?: number;
  cooldownSeconds?: number;
  repeatSeconds?: number;
  suppressDuringDeployment: boolean;
}

export interface RuleUpdate {
  enabled?: boolean;
  severity?: Severity;
  threshold?: number;
  recoveryThreshold?: number;
  triggerSeconds?: number;
  recoverySeconds?: number;
}

export function listRules(signal?: AbortSignal) {
  return api.get<Rule[]>('/api/v1/alert-rules', {
    signal,
    fallback: 'Alert rules are unavailable.',
  });
}

export function updateRule(id: string, update: RuleUpdate) {
  return api.patch<Rule>(
    `/api/v1/alert-rules/${encodeURIComponent(id)}`,
    update,
    {
      fallback: 'The rule could not be updated.',
    },
  );
}
