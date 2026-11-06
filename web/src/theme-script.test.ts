// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import indexHtml from '../index.html?raw';
import { applyPreferences, preferences } from './lib/preferences';

// The inline script in index.html applies the saved theme before the app
// bundle loads, so light-theme users never see a dark first paint (UI-20).
const inline = /<script>([\s\S]*?)<\/script>/.exec(indexHtml)?.[1] ?? '';

function runThemeScript() {
  // Runs the exact bytes the browser runs; the CSP pins them by hash.
  new Function(inline)();
}

function prefersDark(dark: boolean) {
  vi.stubGlobal(
    'matchMedia',
    vi.fn((query: string) => ({
      matches: dark && query === '(prefers-color-scheme: dark)',
      addEventListener: () => {},
      removeEventListener: () => {},
    })),
  );
}

describe('index.html theme script', () => {
  beforeEach(() => {
    localStorage.clear();
    delete document.documentElement.dataset.theme;
    delete document.documentElement.dataset.density;
    prefersDark(true);
  });
  afterEach(() => vi.unstubAllGlobals());

  it('exists and runs before the module bundle', () => {
    expect(inline).toContain('dataset.theme');
    expect(indexHtml.indexOf('<script>')).toBeLessThan(
      indexHtml.indexOf('<script type="module"'),
    );
  });

  it('defaults to dark when nothing is stored', () => {
    runThemeScript();
    expect(document.documentElement.dataset.theme).toBe('dark');
    expect(document.documentElement.dataset.density).toBe('comfortable');
  });

  it('applies the theme the app mirrored to localStorage', () => {
    applyPreferences({ ...preferences(), theme: 'light', density: 'compact' });
    delete document.documentElement.dataset.theme;
    delete document.documentElement.dataset.density;
    runThemeScript();
    expect(document.documentElement.dataset.theme).toBe('light');
    expect(document.documentElement.dataset.density).toBe('compact');
  });

  it('resolves the system theme from the colour-scheme query', () => {
    localStorage.setItem('binnacle.theme', 'system');
    prefersDark(false);
    runThemeScript();
    expect(document.documentElement.dataset.theme).toBe('light');
    prefersDark(true);
    runThemeScript();
    expect(document.documentElement.dataset.theme).toBe('dark');
  });

  it('ignores corrupt or unknown values', () => {
    localStorage.setItem('binnacle.preferences.v1', '{not json');
    localStorage.setItem('binnacle.theme', 'sepia');
    runThemeScript();
    expect(document.documentElement.dataset.theme).toBe('dark');
  });
});
