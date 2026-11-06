import { authenticatedMutation } from './auth';
import type { Tone } from './ui/status';

export interface DiagnosticResult {
  id: string;
  name: string;
  status: 'passed' | 'warning' | 'failed' | 'not_run';
  required: boolean;
  reason: string;
  suggestedFix?: string;
  technicalDetail?: string;
  /** Demo mode replaced this check's source with synthetic data. */
  synthetic?: boolean;
}

/**
 * How one check reads in the interface: a tone (which sets the marker shape)
 * and a word. A synthetic check is neither a pass nor a failure (P4-12).
 */
export function diagnosticDisplay(check: DiagnosticResult): {
  tone: Tone;
  label: string;
} {
  if (check.synthetic) return { tone: 'info', label: 'synthetic data' };
  switch (check.status) {
    case 'passed':
      return { tone: 'ok', label: 'passed' };
    case 'failed':
      return { tone: 'critical', label: 'failed' };
    case 'warning':
      return { tone: 'warn', label: 'warning' };
    default:
      return { tone: 'neutral', label: 'not run' };
  }
}

/** Counts for the onboarding summary; synthetic checks are their own group. */
export function diagnosticSummary(checks: DiagnosticResult[]) {
  const real = checks.filter((check) => !check.synthetic);
  return {
    total: checks.length,
    passed: real.filter((check) => check.status === 'passed').length,
    synthetic: checks.length - real.length,
    toReview: real.filter((check) => check.status === 'failed').length,
  };
}

export interface OnboardingState {
  exposureMode?: 'public' | 'private';
  retentionPreset?: 'minimal' | 'balanced' | 'long-term';
  diagnostics?: DiagnosticResult[];
  completedAt?: string;
  checklistDismissed: boolean;
}

export async function setupAvailable(): Promise<boolean> {
  const response = await fetch('/api/v1/setup');
  if (!response.ok) return false;
  return ((await response.json()) as { available: boolean }).available;
}

export async function verifySetupToken(token: string): Promise<void> {
  const response = await fetch('/api/v1/setup/verify', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ token }),
  });
  if (!response.ok) throw new Error('The setup token is invalid or expired.');
}

export async function claimSetup(
  token: string,
  username: string,
  password: string,
): Promise<void> {
  const response = await fetch('/api/v1/setup/claim', {
    method: 'POST',
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ token, username, password }),
  });
  if (!response.ok) throw new Error('The administrator could not be created.');
}

export async function onboardingState(): Promise<OnboardingState> {
  const response = await fetch('/api/v1/onboarding', {
    credentials: 'same-origin',
  });
  if (!response.ok) throw new Error('Onboarding state is unavailable.');
  return (await response.json()) as OnboardingState;
}

export function saveOnboarding(retentionPreset: string) {
  return authenticatedMutation<OnboardingState>('/api/v1/onboarding', 'PATCH', {
    retentionPreset,
  });
}
export function runDiagnostics(includeOutbound: boolean) {
  return authenticatedMutation<OnboardingState>(
    '/api/v1/onboarding/diagnostics',
    'POST',
    { includeOutbound },
  );
}
export function completeOnboarding() {
  return authenticatedMutation<OnboardingState>(
    '/api/v1/onboarding/complete',
    'POST',
  );
}
export function dismissChecklist() {
  return authenticatedMutation<never>(
    '/api/v1/onboarding/checklist/dismiss',
    'POST',
  );
}

export interface ChecklistItem {
  id: string;
  label: string;
  status: 'done' | 'attention' | 'optional';
  detail: string;
  href: string;
}

/**
 * The "Finish setting up" items on Headroom, from the last diagnostics run.
 * Demo mode's synthetic Docker counts as done and says so; it never asks the
 * user to mount a socket that demo mode does not use (P4-12).
 */
export function setupChecklistItems(
  diagnostics: DiagnosticResult[],
): ChecklistItem[] {
  const docker = diagnostics.find((item) => item.id === 'docker_api');
  const metadata = diagnostics.find(
    (item) => item.id === 'deployment_metadata',
  );
  return [
    {
      id: 'docker',
      label: 'Docker monitoring',
      status: docker?.status === 'passed' ? 'done' : 'attention',
      detail: docker?.synthetic
        ? 'Demo mode: containers are synthetic data.'
        : docker?.status === 'passed'
          ? 'Containers are being observed.'
          : (docker?.suggestedFix ??
            'Mount the Docker socket or configure a socket proxy.'),
      href: '/server?tab=collectors',
    },
    {
      id: 'metadata',
      label: 'Compose or Coolify grouping',
      status: metadata?.status === 'passed' ? 'done' : 'optional',
      detail: metadata?.synthetic
        ? 'Demo mode: projects come from the synthetic catalogue.'
        : metadata?.status === 'passed'
          ? 'Resources are grouped by project.'
          : 'Connect Coolify to enrich names and grouping.',
      href: '/settings/integrations',
    },
    {
      id: 'channel',
      label: 'Add a notification channel',
      status: 'optional',
      detail: 'Get incidents by webhook or email.',
      href: '/settings/notifications',
    },
    {
      id: 'check',
      label: 'Create a health check',
      status: 'optional',
      detail: 'Probe an HTTP endpoint on a schedule.',
      href: '/incidents?tab=checks',
    },
  ];
}
