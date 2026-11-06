import { describe, expect, it } from 'vitest';
import { formatAbsolute } from './relative-time';
import { missing } from '../i18n';

describe('formatAbsolute', () => {
  it('names the time zone and keeps one format everywhere (UI-18)', () => {
    const text = formatAbsolute('2026-11-06T11:32:00Z', 'en-US');
    expect(text).toMatch(/Nov 6, 2026/);
    expect(text).toMatch(/(UTC|GMT|[A-Z]{2,5}|GMT[+-]\d+)$/);
  });

  it('uses the shared missing glyph for unparsable input', () => {
    expect(formatAbsolute('not a date')).toBe(missing);
  });
});
