// @vitest-environment jsdom
import { afterEach, beforeAll, describe, expect, it } from 'vitest';
import { cleanup, render, screen, within } from '@testing-library/svelte';
import LogViewer from './LogViewer.svelte';
import { polyfillDom } from '../ui/testing';

beforeAll(polyfillDom);
afterEach(cleanup);

const lines = [
  {
    seq: 1,
    entry: {
      timestamp: '2026-07-11T11:59:58Z',
      component: 'api-1',
      stream: 'stdout',
      severity: 'info',
      message: 'GET /health 200',
    },
  },
  {
    seq: 2,
    entry: {
      timestamp: '2026-07-11T11:59:59Z',
      component: 'api-2',
      stream: 'stderr',
      severity: 'error',
      message: 'connection refused',
    },
  },
];

describe('LogViewer', () => {
  it('names each stream in text, not an aria-label on a bare span (axe)', () => {
    render(LogViewer, { lines });
    const items = within(
      screen.getByRole('region', { name: 'Log lines' }),
    ).getAllByRole('listitem');
    expect(items[0]!.textContent).toContain('stdout');
    expect(items[1]!.textContent).toContain('stderr');
    // aria-label is prohibited on an element without a role.
    for (const element of document.querySelectorAll('[aria-label]'))
      expect(
        element.getAttribute('role') ??
          ['button', 'a', 'input', 'ol', 'ul', 'section', 'nav'].includes(
            element.localName,
          ),
      ).toBeTruthy();
  });
});
