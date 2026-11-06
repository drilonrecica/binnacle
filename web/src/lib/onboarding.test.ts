import { describe, expect, it } from 'vitest';
import {
  diagnosticDisplay,
  diagnosticSummary,
  setupChecklistItems,
  type DiagnosticResult,
} from './onboarding';

function check(partial: Partial<DiagnosticResult>): DiagnosticResult {
  return {
    id: 'docker_api',
    name: 'Docker API access',
    status: 'passed',
    required: true,
    reason: '',
    ...partial,
  };
}

describe('diagnosticDisplay', () => {
  it('pairs every status with a tone and a word', () => {
    expect(diagnosticDisplay(check({ status: 'passed' }))).toEqual({
      tone: 'ok',
      label: 'passed',
    });
    expect(diagnosticDisplay(check({ status: 'failed' }))).toEqual({
      tone: 'critical',
      label: 'failed',
    });
    expect(diagnosticDisplay(check({ status: 'warning' }))).toEqual({
      tone: 'warn',
      label: 'warning',
    });
    expect(diagnosticDisplay(check({ status: 'not_run' }))).toEqual({
      tone: 'neutral',
      label: 'not run',
    });
  });

  it('reports a demo-mode source as synthetic data, never as failed', () => {
    expect(
      diagnosticDisplay(check({ status: 'passed', synthetic: true })),
    ).toEqual({ tone: 'info', label: 'synthetic data' });
  });
});

describe('diagnosticSummary', () => {
  it('counts synthetic checks apart from real passes and failures', () => {
    expect(
      diagnosticSummary([
        check({ id: 'docker_api', synthetic: true }),
        check({ id: 'host_metrics', synthetic: true }),
        check({ id: 'database' }),
        check({ id: 'persistent_storage', status: 'failed' }),
        check({ id: 'outbound_network', status: 'not_run' }),
      ]),
    ).toEqual({ total: 5, passed: 1, synthetic: 2, toReview: 1 });
  });
});

describe('setupChecklistItems', () => {
  it('asks for the Docker socket when the Docker check failed', () => {
    const [docker] = setupChecklistItems([
      check({ status: 'failed', suggestedFix: 'Mount the socket.' }),
    ]);
    expect(docker).toMatchObject({
      status: 'attention',
      detail: 'Mount the socket.',
    });
  });

  it('never claims Docker is missing in demo mode', () => {
    const [docker, metadata] = setupChecklistItems([
      check({ synthetic: true }),
      check({ id: 'deployment_metadata', synthetic: true, required: false }),
    ]);
    expect(docker).toMatchObject({
      status: 'done',
      detail: 'Demo mode: containers are synthetic data.',
    });
    expect(metadata?.status).toBe('done');
  });
});
