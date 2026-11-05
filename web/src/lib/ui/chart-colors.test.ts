// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  invalidateChartColors,
  readChartColors,
  seriesColor,
} from './chart-colors';

function setTokens(values: Record<string, string>) {
  for (const [name, value] of Object.entries(values))
    document.documentElement.style.setProperty(name, value);
}

describe('chart colours', () => {
  afterEach(() => {
    invalidateChartColors();
    document.documentElement.removeAttribute('style');
    delete document.documentElement.dataset.theme;
    vi.restoreAllMocks();
  });

  it('reads the tokens once per theme and caches them', () => {
    setTokens({
      '--chart-1': 'brass',
      '--chart-2': 'steel',
      '--chart-area': '0.2',
    });
    const spy = vi.spyOn(window, 'getComputedStyle');
    const first = readChartColors();
    const second = readChartColors();
    expect(spy).toHaveBeenCalledTimes(1);
    expect(second).toBe(first);
    expect(first.series.slice(0, 2)).toEqual(['brass', 'steel']);
    expect(first.area).toBe(0.2);
  });

  it('reads again when the theme changes', () => {
    setTokens({ '--chart-1': 'dark-brass' });
    expect(readChartColors().series[0]).toBe('dark-brass');
    document.documentElement.dataset.theme = 'light';
    setTokens({ '--chart-1': 'light-brass' });
    expect(readChartColors().series[0]).toBe('light-brass');
  });

  it('has no hard-coded fallback colour', () => {
    expect(readChartColors().series[0]).toBe('');
  });

  it('picks series by palette slot or position, wrapping at six', () => {
    setTokens(
      Object.fromEntries(
        [1, 2, 3, 4, 5, 6].map((n) => [`--chart-${n}`, `c${n}`]),
      ),
    );
    const colors = readChartColors();
    expect(seriesColor(colors, 0)).toBe('c1');
    expect(seriesColor(colors, 7)).toBe('c2');
    expect(seriesColor(colors, 0, 3)).toBe('c3');
  });
});
