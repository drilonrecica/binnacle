// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { tick } from 'svelte';
import { router } from './router.svelte';

let main: HTMLElement;
let link: HTMLAnchorElement;

beforeEach(() => {
  history.replaceState({}, '', '/headroom');
  router.sync();
  window.scrollTo = () => {};
  document.body.innerHTML =
    '<nav><a href="/server">Server</a></nav><main id="content" tabindex="-1"><input /></main>';
  main = document.getElementById('content') as HTMLElement;
  link = document.querySelector('a') as HTMLAnchorElement;
  link.focus();
});

afterEach(() => {
  document.body.innerHTML = '';
});

describe('router focus (UI-10)', () => {
  it('moves focus to main after the page changes', async () => {
    router.navigate('/server');
    await tick();
    await tick();
    expect(document.activeElement).toBe(main);
  });

  it('leaves focus alone when only the query changes', async () => {
    router.setQuery({ tab: 'collectors' });
    await tick();
    await tick();
    expect(document.activeElement).toBe(link);
  });

  it('moves focus on back and forward too', async () => {
    router.install();
    history.pushState({}, '', '/changes');
    dispatchEvent(new PopStateEvent('popstate'));
    await tick();
    await tick();
    expect(document.activeElement).toBe(main);
  });

  it('keeps focus a new page already placed inside main', async () => {
    router.navigate('/incidents');
    const input = main.querySelector('input') as HTMLInputElement;
    input.focus();
    await tick();
    await tick();
    expect(document.activeElement).toBe(input);
  });
});
