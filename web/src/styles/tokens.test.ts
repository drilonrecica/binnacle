import { describe, expect, it } from 'vitest';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';
import { fileURLToPath } from 'node:url';

// The token contract (ADR 027): colours live only in tokens.css, and every
// var() a component reads resolves to a declared property. CSS ignores an
// undefined var() silently, so a stale name would render as a missing border
// or colour instead of failing; this test makes it fail.

const src = fileURLToPath(new URL('..', import.meta.url));
const tokensFile = join(src, 'styles', 'tokens.css');

function walk(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) return walk(path);
    return /\.(svelte|css|ts)$/.test(name) && !name.endsWith('.test.ts')
      ? [path]
      : [];
  });
}

const files = walk(src).map((path) => ({
  path: relative(src, path),
  text: readFileSync(path, 'utf8'),
}));

/** The ADR 020 names the P4-01 compatibility block mapped; none may return. */
const retired = [
  'surface',
  'surface-2',
  'surface-3',
  'bg-subtle',
  'border',
  'border-strong',
  'text',
  'text-2',
  'text-3',
  'text-inverse',
  'text-xs',
  'text-sm',
  'text-base',
  'text-md',
  'text-lg',
  'text-xl',
  'text-2xl',
  'text-num',
  'tracking-tight',
  'space-5',
  'space-10',
  'radius-sm',
  'radius',
  'radius-lg',
  'radius-full',
  'shadow-sm',
  'shadow-md',
  'shadow-lg',
  'motion-fast',
  'ease',
];

function declared(): Set<string> {
  const names = new Set<string>();
  const patterns = [
    /(--[\w-]+)\s*:/g, // CSS declarations, in tokens.css and component styles
    /style:(--[\w-]+)/g, // Svelte style directives
    /setProperty\(\s*['`](--[\w-]+)/g, // runtime properties (uPlot cursor)
  ];
  for (const { text } of files)
    for (const pattern of patterns)
      for (const match of text.matchAll(pattern)) names.add(match[1]);
  return names;
}

describe('design tokens', () => {
  it('has no compatibility aliases left in tokens.css', () => {
    const tokens = readFileSync(tokensFile, 'utf8');
    for (const name of retired)
      expect(tokens, `--${name} is still declared`).not.toMatch(
        new RegExp(`--${name}\\s*:`),
      );
  });

  it('resolves every var() in the frontend to a declared property', () => {
    const names = declared();
    const missing: string[] = [];
    for (const { path, text } of files)
      for (const match of text.matchAll(/var\(\s*(--[\w-]+)(\$\{)?/g))
        // `var(--chart-${n})` is built at runtime; its targets are declared.
        if (!match[2] && !names.has(match[1]))
          missing.push(`${path}: ${match[1]}`);
    expect(missing).toEqual([]);
  });

  it('keeps hex colours inside tokens.css', () => {
    const offenders = files
      .filter(({ path }) => !path.endsWith('tokens.css'))
      .flatMap(({ path, text }) =>
        [...text.matchAll(/(?<![\w&])#[0-9a-fA-F]{3,8}\b(?![\w-])/g)]
          // Fragment links such as href="#main" are not colours.
          .filter((match) => !/href=["'`]?$/.test(text.slice(0, match.index)))
          .map((match) => `${path}: ${match[0]}`),
      );
    expect(offenders).toEqual([]);
  });
});
