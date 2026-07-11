import { describe, expect, it } from 'vitest';
import { formatBytes, formatNumber, formatRate } from './i18n';
describe('formatters', () => { it('preserves unavailable values and formats units', () => { expect(formatBytes(null)).toBe('Unavailable'); expect(formatBytes(1024, 'en-US')).toBe('1 KiB'); expect(formatRate(1024, 'en-US')).toBe('1 KiB/s'); expect(formatNumber(1.25, 'en-US')).toBe('1.3'); }); });
