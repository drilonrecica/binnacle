import { describe, expect, it } from 'vitest';
import { resolveTheme } from './preferences';

describe('theme resolution', () => {
  it('uses operating-system preference only for system theme', () => {
    expect(resolveTheme('system', true)).toBe('dark');
    expect(resolveTheme('system', false)).toBe('light');
    expect(resolveTheme('light', true)).toBe('light');
  });
});
