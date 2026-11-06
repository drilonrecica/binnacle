import type { SilencePreset, SilenceScope } from '../api/silences';
import { formatAbsolute, formatClock } from '../ui/relative-time';

const presetMinutes: Partial<Record<SilencePreset, number>> = {
  '30m': 30,
  '1h': 60,
  '4h': 240,
};

/**
 * One sentence that states what a silence will do before it is created, so
 * the dialog doubles as the confirmation: "Notifications for api.production
 * pause until 14:30. Alerts keep evaluating." Empty while the form is
 * incomplete.
 */
export function silenceSummary(
  input: {
    scope: SilenceScope;
    scopeId: string;
    resourceName?: string;
    ruleName?: string;
    preset: SilencePreset;
    customEnd: string;
  },
  now = Date.now(),
): string {
  const { scope, scopeId } = input;
  if (scope !== 'server' && !scopeId) return '';
  const target =
    scope === 'server'
      ? 'the whole server'
      : scope === 'project'
        ? `project ${scopeId}`
        : scope === 'rule'
          ? `rule ${input.ruleName ?? scopeId}`
          : (input.resourceName ?? 'this resource');
  let until: string;
  const minutes = presetMinutes[input.preset];
  if (minutes)
    until = `until ${formatClock(now + minutes * 60_000, undefined, false)}`;
  else if (input.preset === 'tomorrow') until = 'until tomorrow';
  else {
    const end = Date.parse(input.customEnd);
    if (!Number.isFinite(end)) return '';
    until = `until ${formatAbsolute(end)}`;
  }
  return `Notifications for ${target} pause ${until}. Alerts keep evaluating.`;
}
