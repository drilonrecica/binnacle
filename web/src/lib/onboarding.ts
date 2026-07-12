import { authenticatedMutation } from './auth';

export interface DiagnosticResult {
  id: string;
  name: string;
  status: 'passed' | 'warning' | 'failed' | 'not_run';
  required: boolean;
  reason: string;
  suggestedFix?: string;
  technicalDetail?: string;
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
