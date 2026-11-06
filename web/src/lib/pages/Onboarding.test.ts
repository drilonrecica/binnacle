// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, render, screen, within } from '@testing-library/svelte';
import type { OnboardingState } from '../onboarding';

const mocks = vi.hoisted(() => ({ onboardingState: vi.fn() }));

vi.mock('../onboarding', async () => ({
  ...(await vi.importActual<typeof import('../onboarding')>('../onboarding')),
  onboardingState: mocks.onboardingState,
}));

const { default: Onboarding } = await import('./Onboarding.svelte');

// What the server returns in demo mode (internal/diagnostics, Demo: true).
const demo: OnboardingState = {
  checklistDismissed: false,
  retentionPreset: 'balanced',
  diagnostics: [
    {
      id: 'host_metrics',
      name: 'Host metrics access',
      status: 'passed',
      required: true,
      synthetic: true,
      reason: 'Demo mode: host CPU, memory and disks are synthetic data.',
    },
    {
      id: 'docker_api',
      name: 'Docker API access',
      status: 'passed',
      required: true,
      synthetic: true,
      reason: 'Demo mode: containers are synthetic data.',
    },
    {
      id: 'database',
      name: 'Database initialization',
      status: 'passed',
      required: true,
      reason: 'SQLite is initialized and responsive.',
    },
  ],
};

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe('Onboarding diagnostics in demo mode (P4-12)', () => {
  it('reports synthetic Docker as synthetic data, never as failed', async () => {
    mocks.onboardingState.mockResolvedValue(demo);
    render(Onboarding, { oncomplete: vi.fn() });

    const docker = (await screen.findByText('Docker API access')).closest(
      'li',
    )!;
    expect(within(docker).getByText('synthetic data')).toBeTruthy();
    expect(screen.queryByText('failed')).toBeNull();
    expect(screen.queryByText(/Required checks failed/)).toBeNull();
    expect(
      screen.getByText('Demo mode: host and Docker data are synthetic.'),
    ).toBeTruthy();
    const database = screen.getByText('Database initialization').closest('li')!;
    expect(within(database).getByText('passed')).toBeTruthy();
  });

  it('still reports a real Docker failure outside demo mode', async () => {
    mocks.onboardingState.mockResolvedValue({
      ...demo,
      diagnostics: [
        {
          id: 'docker_api',
          name: 'Docker API access',
          status: 'failed',
          required: true,
          reason: 'Docker access is not configured.',
        },
      ],
    });
    render(Onboarding, { oncomplete: vi.fn() });

    const docker = (await screen.findByText('Docker API access')).closest(
      'li',
    )!;
    expect(within(docker).getByText('failed')).toBeTruthy();
    expect(screen.getByText(/Required checks failed/)).toBeTruthy();
    expect(screen.queryByText(/synthetic/)).toBeNull();
  });
});
