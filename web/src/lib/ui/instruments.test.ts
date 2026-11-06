// @vitest-environment jsdom
import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Gauge from './Gauge.svelte';
import Scale from './Scale.svelte';
import Sparkline from './Sparkline.svelte';
import { arcPath, arcPoint, describeSeries, summarize } from './instrument';

describe('instrument geometry', () => {
  it('runs the arc from the left end over the top to the right end', () => {
    expect(arcPoint(60, 60, 50, 0)).toEqual({ x: 10, y: 60 });
    expect(arcPoint(60, 60, 50, 0.5)).toEqual({ x: 60, y: 10 });
    expect(arcPoint(60, 60, 50, 1)).toEqual({ x: 110, y: 60 });
    expect(arcPath(60, 60, 50, 0, 1)).toBe('M10 60 A50 50 0 0 1 110 60');
  });

  it('summarizes only finite values and keeps the latest as now', () => {
    expect(summarize([null, 4, 2, Number.NaN, 6, null])).toEqual({
      min: 2,
      max: 6,
      avg: 4,
      now: 6,
    });
    expect(summarize([null, undefined])).toBeNull();
    expect(describeSeries('CPU', [], String)).toBe('CPU: no measurements.');
  });
});

describe('Gauge', () => {
  it('is a meter on a true 0..100 scale', () => {
    render(Gauge, { value: 42.4, label: 'CPU' });
    const meter = screen.getByRole('meter', { name: 'CPU' });
    expect(meter.getAttribute('aria-valuemin')).toBe('0');
    expect(meter.getAttribute('aria-valuemax')).toBe('100');
    expect(meter.getAttribute('aria-valuenow')).toBe('42.4');
    expect(meter.getAttribute('aria-valuetext')).toBe('42 percent');
  });

  it('clamps readings outside the scale', () => {
    render(Gauge, { value: 140, label: 'Memory' });
    expect(
      screen
        .getByRole('meter', { name: 'Memory' })
        .getAttribute('aria-valuenow'),
    ).toBe('100');
  });

  it('turns critical at the threshold and says so in words', () => {
    const { container } = render(Gauge, {
      value: 93,
      label: 'Disk',
      threshold: 90,
    });
    expect(container.querySelector('.gauge')?.classList.contains('over')).toBe(
      true,
    );
    expect(screen.getByText(/Above 90%/)).toBeTruthy();
    expect(
      screen
        .getByRole('meter', { name: 'Disk' })
        .getAttribute('aria-valuetext'),
    ).toContain('at or above the 90 percent threshold');
  });

  it('takes the warn tone between the warn and critical lines', () => {
    const { container } = render(Gauge, {
      value: 86,
      label: 'Memory',
      warn: 85,
      threshold: 95,
      unit: '',
      valueText: '6.9 GiB',
      trend: 'full in ~3 d',
      trendTone: 'critical',
    });
    const gauge = container.querySelector('.gauge');
    expect(gauge?.classList.contains('warning')).toBe(true);
    expect(gauge?.classList.contains('over')).toBe(false);
    expect(screen.getByText(/Above 85%/)).toBeTruthy();
    expect(container.querySelectorAll('.tick')).toHaveLength(2);
    expect(container.querySelector('.trend.critical')?.textContent).toBe(
      'full in ~3 d',
    );
    expect(
      screen
        .getByRole('meter', { name: 'Memory' })
        .getAttribute('aria-valuetext'),
    ).toBe('6.9 GiB, 86 percent, threshold 95 percent');
  });

  it('states missing data instead of drawing a needle at zero', () => {
    const { container } = render(Gauge, { value: null, label: 'CPU' });
    expect(screen.queryByRole('meter')).toBeNull();
    expect(screen.getByRole('img', { name: 'CPU: no data' })).toBeTruthy();
    expect(container.querySelector('.needle')).toBeNull();
    expect(screen.getByText('—')).toBeTruthy();
  });
});

describe('Scale', () => {
  it('is a meter with a threshold tick and text', () => {
    const { container } = render(Scale, {
      value: 50,
      label: '/data usage',
      threshold: 80,
      minLabel: '0',
      maxLabel: '100%',
    });
    const meter = screen.getByRole('meter', { name: '/data usage' });
    expect(meter.getAttribute('aria-valuetext')).toBe(
      '50 percent, threshold 80',
    );
    const tick = container.querySelector<HTMLElement>('.tick');
    expect(tick?.style.left).toBe('80%');
    expect(container.querySelector('.ends')?.textContent).toBe('0100%');
  });

  it('fills critical at or above the threshold', () => {
    const { container } = render(Scale, {
      value: 85,
      label: 'Memory',
      threshold: 80,
    });
    expect(
      container.querySelector('.track')?.classList.contains('critical'),
    ).toBe(true);
  });
});

describe('Sparkline', () => {
  it('hides the drawing and gives screen readers now, min, avg and max', () => {
    const { container } = render(Sparkline, {
      values: [10, 30, null, 20],
      label: 'CPU, last hour',
      format: (value: number) => `${value}%`,
    });
    expect(container.querySelector('svg')?.getAttribute('aria-hidden')).toBe(
      'true',
    );
    expect(
      screen
        .getByText(
          'CPU, last hour: now 20%, minimum 10%, average 20%, maximum 30%.',
        )
        .classList.contains('sr-only'),
    ).toBe(true);
  });

  it('draws a baseline, a threshold line and an emphasised last point', () => {
    const { container } = render(Sparkline, {
      values: [10, 30, 20],
      max: 100,
      threshold: 80,
      height: 32,
    });
    expect(container.querySelector('.baseline')).toBeTruthy();
    const threshold = container.querySelector('.threshold');
    expect(threshold).toBeTruthy();
    // 80 of 100 on a 32px box with 2.5px padding: 2.5 + 27 * 0.2 = 7.9.
    expect(Number(threshold?.getAttribute('y1'))).toBeCloseTo(7.9, 5);
    expect(container.querySelector('.halo')).toBeTruthy();
    expect(container.querySelector('.dot')).toBeTruthy();
  });

  it('keeps the baseline when there is too little data for a line', () => {
    const { container } = render(Sparkline, { values: [5] });
    expect(container.querySelector('.baseline')).toBeTruthy();
    expect(container.querySelector('.line')).toBeNull();
  });
});
