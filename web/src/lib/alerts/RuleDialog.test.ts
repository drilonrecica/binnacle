// @vitest-environment jsdom
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest';
import { cleanup, render, screen, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import type { Rule } from '../api/alerts';
import { field, polyfillDom } from '../ui/testing';

vi.mock('../api/alerts', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/alerts')>()),
  updateRule: vi.fn(),
}));

const { default: RuleDialog } = await import('./RuleDialog.svelte');
const alertsApi = await import('../api/alerts');

beforeAll(polyfillDom);
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const rule: Rule = {
  id: 'builtin-host-cpu-warning',
  family: 'host_cpu_warning',
  name: 'Host CPU warning',
  builtIn: true,
  enabled: true,
  severity: 'warning',
  scopeType: 'global',
  threshold: 90,
  recoveryThreshold: 80,
  triggerSeconds: 300,
  recoverySeconds: 120,
  suppressDuringDeployment: false,
};

function form() {
  const element = screen.getByRole('dialog');
  return {
    element,
    field: (text: string) => field(element, text) as HTMLInputElement,
    save: () =>
      userEvent.click(
        within(element).getByRole('button', { name: 'Save rule' }),
      ),
    error: () => within(element).getByRole('alert').textContent,
  };
}

describe('RuleDialog', () => {
  it('opens with the rule values in minutes', () => {
    render(RuleDialog, { open: true, rule });
    const f = form();
    expect(screen.getByRole('heading', { name: 'Edit Host CPU' })).toBeTruthy();
    expect(f.field('Trigger above').value).toBe('90');
    expect(f.field('Recover below').value).toBe('80');
    expect(f.field('Must persist for').value).toBe('5');
    expect(f.field('Recovery time').value).toBe('2');
  });

  it('refuses a percentage above 100', async () => {
    render(RuleDialog, { open: true, rule });
    const f = form();
    await userEvent.clear(f.field('Trigger above'));
    await userEvent.type(f.field('Trigger above'), '120');
    await f.save();
    expect(f.error()).toContain('Threshold must be between 0 and 100.');
    expect(alertsApi.updateRule).not.toHaveBeenCalled();
  });

  it('refuses a recovery threshold above the trigger', async () => {
    render(RuleDialog, { open: true, rule });
    const f = form();
    await userEvent.clear(f.field('Recover below'));
    await userEvent.type(f.field('Recover below'), '95');
    await f.save();
    expect(f.error()).toContain('must not exceed the trigger threshold');
    expect(alertsApi.updateRule).not.toHaveBeenCalled();
  });

  it('saves durations in seconds and closes', async () => {
    vi.mocked(alertsApi.updateRule).mockResolvedValue(rule);
    const onsaved = vi.fn();
    render(RuleDialog, { open: true, rule, onsaved });
    const f = form();
    await userEvent.selectOptions(f.field('Severity'), 'critical');
    await userEvent.clear(f.field('Must persist for'));
    await userEvent.type(f.field('Must persist for'), '10');
    await f.save();
    expect(alertsApi.updateRule).toHaveBeenCalledWith(rule.id, {
      severity: 'critical',
      threshold: 90,
      recoveryThreshold: 80,
      triggerSeconds: 600,
      recoverySeconds: 120,
    });
    expect(onsaved).toHaveBeenCalledWith(rule);
    expect((f.element as HTMLDialogElement).open).toBe(false);
  });
});
