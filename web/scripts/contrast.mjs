// SPDX-License-Identifier: AGPL-3.0-only
// Checks the design tokens in src/styles/tokens.css against WCAG 2.2 AA in
// both themes (ADR 027). Text pairs must reach 4.5:1 and non-text pairs
// (focus rings, markers, chart series) 3:1. Exits 1 when any pair fails, so
// `make web-check` refuses a palette change that breaks contrast.
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';

export const TEXT = 4.5;
export const GRAPHIC = 3;

const tones = ['ok', 'warn', 'critical', 'info', 'neutral'];
const surfaces = ['--bg', '--panel', '--panel-2'];

/**
 * Every pair the gate checks, as [foreground, background, minimum ratio].
 * A background may be a tinted (alpha) token followed by the opaque surface it
 * sits on, written `--ok-bg over --panel`.
 */
export const pairs = [
  ...surfaces.flatMap((surface) => [
    ['--fg', surface, TEXT],
    ['--muted', surface, TEXT],
    ['--accent-text', surface, TEXT],
    ['--focus', surface, GRAPHIC],
    ['--accent', surface, GRAPHIC],
    ...tones.map((tone) => [`--${tone}-fg`, surface, TEXT]),
    ...tones.map((tone) => [`--${tone}-solid`, surface, GRAPHIC]),
    ...[1, 2, 3, 4, 5, 6].map((n) => [`--chart-${n}`, surface, GRAPHIC]),
  ]),
  // Tinted chips, notices and selected cards: the tone's own text, plus the
  // body and muted text that notices put on them, over both surface extremes.
  ...[...tones.map((tone) => `--${tone}-bg`), '--accent-bg'].flatMap((tint) =>
    ['--bg', '--panel-2'].flatMap((surface) => [
      ['--fg', `${tint} over ${surface}`, TEXT],
      ['--muted', `${tint} over ${surface}`, TEXT],
    ]),
  ),
  ...tones.flatMap((tone) => [
    [`--${tone}-fg`, `--${tone}-bg over --bg`, TEXT],
    [`--${tone}-fg`, `--${tone}-bg over --panel-2`, TEXT],
  ]),
  ['--accent-text', '--accent-bg over --bg', TEXT],
  ['--accent-text', '--accent-bg over --panel-2', TEXT],
  ['--accent-contrast', '--accent', TEXT],
  ['--fg-inverse', '--fg', TEXT],
];

/** Parses `:root` blocks into one map per theme, light inheriting from dark. */
export function parseTokens(css) {
  const source = css
    .replace(/\/\*[\s\S]*?\*\//g, '')
    // Media blocks (reduced motion) hold no colours; drop them whole.
    .replace(/@media[^{]*\{(?:[^{}]|\{[^{}]*\})*\}/g, '');
  const themes = { dark: {}, light: {} };
  const block = /([^{}]+)\{([^{}]*)\}/g;
  for (const [, rawSelector, body] of source.matchAll(block)) {
    const selector = rawSelector.trim();
    let target;
    if (selector === ':root') target = themes.dark;
    else if (selector === ":root[data-theme='light']") target = themes.light;
    else continue;
    const declaration = /(--[\w-]+)\s*:\s*([^;]+);/gs;
    for (const [, name, value] of body.matchAll(declaration)) {
      target[name] = value.replace(/\s+/g, ' ').trim();
    }
  }
  themes.light = { ...themes.dark, ...themes.light };
  return themes;
}

/** Resolves `var()` references in a token value, depth-capped. */
export function resolve(tokens, name, depth = 0) {
  if (depth > 16) throw new Error(`${name}: var() chain too deep`);
  const value = tokens[name];
  if (value === undefined) throw new Error(`${name}: token not defined`);
  return value.replace(/var\((--[\w-]+)(?:\s*,\s*[^)]*)?\)/g, (_, ref) =>
    resolve(tokens, ref, depth + 1),
  );
}

/** Parses `#rrggbb`, `#rgb` or `rgb(R G B / a)` into {r, g, b, a} in 0..255 / 0..1. */
export function parseColor(value) {
  const text = value.trim();
  const hex = /^#([0-9a-f]{3}|[0-9a-f]{6})$/i.exec(text);
  if (hex) {
    const digits =
      hex[1].length === 3 ? [...hex[1]].map((d) => d + d).join('') : hex[1];
    return {
      r: parseInt(digits.slice(0, 2), 16),
      g: parseInt(digits.slice(2, 4), 16),
      b: parseInt(digits.slice(4, 6), 16),
      a: 1,
    };
  }
  const rgb =
    /^rgb\(\s*(\d+)\s+(\d+)\s+(\d+)\s*(?:\/\s*([\d.]+%?))?\s*\)$/i.exec(text);
  if (rgb) {
    let a = 1;
    if (rgb[4] !== undefined) {
      a = rgb[4].endsWith('%')
        ? Number(rgb[4].slice(0, -1)) / 100
        : Number(rgb[4]);
    }
    return { r: Number(rgb[1]), g: Number(rgb[2]), b: Number(rgb[3]), a };
  }
  throw new Error(`unsupported colour: ${value}`);
}

/** Alpha-composites `top` over an opaque `base`. */
export function composite(top, base) {
  const mix = (channel) =>
    Math.round(top[channel] * top.a + base[channel] * (1 - top.a));
  return { r: mix('r'), g: mix('g'), b: mix('b'), a: 1 };
}

export function relativeLuminance({ r, g, b }) {
  const linear = (channel) => {
    const c = channel / 255;
    return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  };
  return 0.2126 * linear(r) + 0.7152 * linear(g) + 0.0722 * linear(b);
}

export function contrast(a, b) {
  const x = relativeLuminance(a);
  const y = relativeLuminance(b);
  return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05);
}

/** Resolves a background expression (`--x` or `--x over --y`) to an opaque colour. */
function background(tokens, expression) {
  const [top, base] = expression.split(' over ');
  const topColor = parseColor(resolve(tokens, top));
  if (!base) {
    if (topColor.a !== 1)
      throw new Error(`${top} is translucent; say what it sits on`);
    return topColor;
  }
  const baseColor = parseColor(resolve(tokens, base));
  if (baseColor.a !== 1) throw new Error(`${base} must be opaque`);
  return composite(topColor, baseColor);
}

/** Checks every pair against one theme's tokens and returns the results. */
export function checkTheme(tokens, checks = pairs) {
  return checks.map(([fg, bg, minimum]) => {
    const base = background(tokens, bg);
    const top = parseColor(resolve(tokens, fg));
    const color = top.a === 1 ? top : composite(top, base);
    const ratio = contrast(color, base);
    return { fg, bg, minimum, ratio, pass: ratio >= minimum };
  });
}

export function checkThemes(css) {
  const themes = parseTokens(css);
  return Object.fromEntries(
    Object.entries(themes).map(([name, tokens]) => [name, checkTheme(tokens)]),
  );
}

export function format(results) {
  const lines = [];
  let failures = 0;
  for (const [theme, checks] of Object.entries(results)) {
    for (const check of checks) {
      if (!check.pass) failures++;
      lines.push(
        `${check.pass ? 'ok  ' : 'FAIL'} ${theme.padEnd(5)} ${check.fg.padEnd(18)} on ${check.bg.padEnd(28)} ${check.ratio.toFixed(2).padStart(5)} (min ${check.minimum})`,
      );
    }
  }
  return { lines, failures };
}

const invokedDirectly =
  process.argv[1] && fileURLToPath(import.meta.url) === process.argv[1];

if (invokedDirectly) {
  const file = new URL('../src/styles/tokens.css', import.meta.url);
  const results = checkThemes(await readFile(file, 'utf8'));
  const { lines, failures } = format(results);
  console.log(lines.join('\n'));
  const total = Object.values(results).reduce((n, r) => n + r.length, 0);
  if (failures > 0) {
    console.error(`\n${failures} of ${total} contrast pairs fail WCAG 2.2 AA`);
    process.exit(1);
  }
  console.log(`\n${total} contrast pairs meet WCAG 2.2 AA in both themes`);
}
