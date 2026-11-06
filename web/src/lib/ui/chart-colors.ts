/**
 * Chart colours read from the design tokens once per theme. uPlot draws on a
 * canvas and cannot use CSS variables, so the values are resolved with
 * getComputedStyle; doing that on every hover or redraw forces style
 * recalculation, so the result is cached and invalidated only when the theme
 * attribute changes. There are no hard-coded fallbacks: a missing token shows
 * up as a missing colour instead of a colour from another palette.
 */
export interface ChartColors {
  series: string[];
  axis: string;
  grid: string;
  gap: string;
  marker: string;
  cursor: string;
  area: number;
}

const SERIES_COUNT = 6;

let cached: { key: string; colors: ChartColors } | null = null;

function themeKey(root: HTMLElement): string {
  return `${root.dataset.theme ?? ''}|${root.dataset.density ?? ''}`;
}

export function readChartColors(
  root: HTMLElement = document.documentElement,
): ChartColors {
  const key = themeKey(root);
  if (cached && cached.key === key) return cached.colors;
  const style = getComputedStyle(root);
  const token = (name: string) => style.getPropertyValue(name).trim();
  const area = Number.parseFloat(token('--chart-area'));
  const colors: ChartColors = {
    series: Array.from({ length: SERIES_COUNT }, (_, index) =>
      token(`--chart-${index + 1}`),
    ),
    axis: token('--chart-axis'),
    grid: token('--chart-grid'),
    gap: token('--chart-gap'),
    marker: token('--chart-marker'),
    cursor: token('--chart-cursor'),
    area: Number.isFinite(area) ? area : 0.2,
  };
  cached = { key, colors };
  return colors;
}

/** Drops the cache; the next read resolves the tokens again. */
export function invalidateChartColors() {
  cached = null;
}

/** Series colour by 1-based palette slot, or by position when none is given. */
export function seriesColor(
  colors: ChartColors,
  index: number,
  slot?: number,
): string {
  const position = slot != null ? slot - 1 : index;
  return (
    colors.series[((position % SERIES_COUNT) + SERIES_COUNT) % SERIES_COUNT] ??
    ''
  );
}
