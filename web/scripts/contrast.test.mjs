// SPDX-License-Identifier: AGPL-3.0-only
import { readFile } from 'node:fs/promises';

import { describe, expect, it } from 'vitest';

import {
  checkTheme,
  checkThemes,
  composite,
  contrast,
  format,
  pairs,
  parseColor,
  parseTokens,
  relativeLuminance,
  resolve,
} from './contrast.mjs';

const black = { r: 0, g: 0, b: 0, a: 1 };
const white = { r: 255, g: 255, b: 255, a: 1 };

describe('WCAG maths', () => {
  it('computes relative luminance at the extremes', () => {
    expect(relativeLuminance(black)).toBe(0);
    expect(relativeLuminance(white)).toBeCloseTo(1, 6);
  });

  it('gives 21:1 for black on white and 1:1 for a colour on itself', () => {
    expect(contrast(black, white)).toBeCloseTo(21, 6);
    expect(contrast(white, black)).toBeCloseTo(21, 6);
    expect(contrast(white, white)).toBe(1);
  });

  it('matches a published reference pair', () => {
    // #767676 on white is the canonical 4.54:1 AA boundary example.
    const gray = parseColor('#767676');
    expect(contrast(gray, white)).toBeCloseTo(4.54, 2);
  });

  it('composites alpha over an opaque base', () => {
    const half = { ...white, a: 0.5 };
    expect(composite(half, black)).toEqual({ r: 128, g: 128, b: 128, a: 1 });
    expect(composite(white, black)).toEqual(white);
  });
});

describe('colour parsing', () => {
  it('reads long hex, short hex and modern rgb syntax', () => {
    expect(parseColor('#1f242b')).toEqual({ r: 31, g: 36, b: 43, a: 1 });
    expect(parseColor('#fff')).toEqual(white);
    expect(parseColor('rgb(210 162 76 / 0.4)')).toEqual({
      r: 210,
      g: 162,
      b: 76,
      a: 0.4,
    });
    expect(parseColor('rgb(1 2 3 / 50%)').a).toBe(0.5);
    expect(parseColor('rgb(1 2 3)').a).toBe(1);
  });

  it('refuses colour syntax it cannot measure', () => {
    expect(() => parseColor('hsl(0 0% 0%)')).toThrow(/unsupported/);
    expect(() => parseColor('red')).toThrow(/unsupported/);
  });
});

describe('token parsing', () => {
  const css = `
    /* dark */
    :root {
      --bg: #000000;
      --fg: #ffffff;
      --muted: var(--fg);
      --font-sans:
        'Barlow', ui-sans-serif,
        sans-serif;
      --tint: rgb(255 255 255 / 0.1);
    }
    :root[data-theme='light'] { --bg: #ffffff; --fg: #000000; }
    @media (prefers-reduced-motion: reduce) { :root { --motion: 0ms; } }
    :root { --alias: var(--muted, #ff0000); }
  `;

  it('splits themes, inherits dark into light and follows var() chains', () => {
    const themes = parseTokens(css);
    expect(themes.dark['--bg']).toBe('#000000');
    expect(themes.light['--bg']).toBe('#ffffff');
    expect(themes.light['--tint']).toBe('rgb(255 255 255 / 0.1)');
    expect(themes.dark['--font-sans']).toBe(
      "'Barlow', ui-sans-serif, sans-serif",
    );
    expect(resolve(themes.dark, '--alias')).toBe('#ffffff');
    expect(resolve(themes.light, '--alias')).toBe('#000000');
    expect(themes.dark['--motion']).toBeUndefined();
  });

  it('names an undefined token and stops a circular chain', () => {
    expect(() => resolve({}, '--nope')).toThrow('--nope: token not defined');
    const loop = { '--a': 'var(--b)', '--b': 'var(--a)' };
    expect(() => resolve(loop, '--a')).toThrow(/too deep/);
  });
});

describe('the gate', () => {
  it('reports a failing pair by name and passes a good one', () => {
    const tokens = {
      '--bg': '#ffffff',
      '--fg': '#000000',
      '--weak': '#aaaaaa',
    };
    const results = checkTheme(tokens, [
      ['--fg', '--bg', 4.5],
      ['--weak', '--bg', 4.5],
    ]);
    expect(results[0].pass).toBe(true);
    expect(results[1]).toMatchObject({ fg: '--weak', bg: '--bg', pass: false });
    const { lines, failures } = format({ dark: results });
    expect(failures).toBe(1);
    expect(lines[1]).toMatch(/^FAIL dark\s+--weak\s+on --bg/);
  });

  it('composites a tinted background over the named surface', () => {
    const tokens = {
      '--panel': '#000000',
      '--fg': '#ffffff',
      '--tint': 'rgb(255 255 255 / 0.5)',
    };
    const [result] = checkTheme(tokens, [['--fg', '--tint over --panel', 3]]);
    expect(result.ratio).toBeCloseTo(
      contrast(white, composite({ ...white, a: 0.5 }, black)),
      6,
    );
  });

  it('refuses a translucent background with no surface under it', () => {
    const tokens = { '--fg': '#ffffff', '--tint': 'rgb(0 0 0 / 0.5)' };
    expect(() => checkTheme(tokens, [['--fg', '--tint', 3]])).toThrow(
      /translucent/,
    );
  });

  it('covers text on every surface and every tone on its own tint', () => {
    const named = pairs.map(([fg, bg]) => `${fg}|${bg}`);
    for (const surface of ['--bg', '--panel', '--panel-2']) {
      expect(named).toContain(`--fg|${surface}`);
      expect(named).toContain(`--muted|${surface}`);
      expect(named).toContain(`--chart-6|${surface}`);
    }
    for (const tone of ['ok', 'warn', 'critical', 'info', 'neutral']) {
      expect(named).toContain(`--${tone}-fg|--${tone}-bg over --panel-2`);
      expect(named).toContain(`--muted|--${tone}-bg over --bg`);
      expect(named).toContain(`--fg|--${tone}-bg over --panel-2`);
    }
    expect(named).toContain('--muted|--accent-bg over --panel-2');
    expect(named).toContain('--accent-contrast|--accent');
  });
});

describe('the committed tokens', () => {
  it('meet WCAG 2.2 AA in both themes', async () => {
    const css = await readFile(
      new URL('../src/styles/tokens.css', import.meta.url),
      'utf8',
    );
    const results = checkThemes(css);
    const failing = Object.entries(results).flatMap(([theme, checks]) =>
      checks
        .filter((c) => !c.pass)
        .map((c) => `${theme} ${c.fg} on ${c.bg} ${c.ratio.toFixed(2)}`),
    );
    expect(failing).toEqual([]);
    expect(results.dark.length).toBe(pairs.length);
    expect(results.light.length).toBe(pairs.length);
  });
});
