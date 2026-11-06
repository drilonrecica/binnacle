import { tick } from 'svelte';
import {
  buildPath,
  matchRoute,
  resolveLegacyPath,
  splitHref,
  type RouteMatch,
} from './router';
import { lastLogsResource } from './logs/last-resource';
import { leaveGuard } from './leave-guard.svelte';

/**
 * Reactive location store backed by the History API. Screens read
 * `router.match`, `router.params`, and `router.query`; they change location
 * with `router.navigate()` or plain same-origin anchors, which the shell
 * intercepts once via `interceptLinks()`.
 */
class Router {
  path = $state('/');
  search = $state('');

  readonly match = $derived<RouteMatch | null>(matchRoute(this.path));
  readonly params = $derived<Record<string, string>>(this.match?.params ?? {});
  readonly query = $derived(new URLSearchParams(this.search));
  readonly name = $derived(this.match?.name ?? null);

  private installed = false;

  /**
   * Reads the current browser location, applying legacy redirects. Returns
   * whether the path changed, so callers can move focus to the new page.
   */
  sync(): boolean {
    const legacy = resolveLegacyPath(
      location.pathname,
      location.search,
      lastLogsResource(),
    );
    if (legacy) {
      history.replaceState({}, '', legacy);
    }
    const changed = this.path !== location.pathname;
    this.path = location.pathname;
    this.search = location.search;
    return changed;
  }

  install() {
    if (this.installed) return;
    this.installed = true;
    this.sync();
    addEventListener('popstate', () => {
      if (location.pathname !== this.path) {
        const target = location.pathname + location.search + location.hash;
        // Back and forward cannot be cancelled; put the page back and ask.
        if (leaveGuard.intercept(target)) {
          history.pushState({}, '', this.path + this.search);
          return;
        }
      }
      if (this.sync()) void focusContent();
    });
  }

  /**
   * Changes location. A change of path waits for confirmation while a form
   * on the page has unsaved edits, unless `force` is set.
   */
  navigate(to: string, options: { replace?: boolean; force?: boolean } = {}) {
    const url = splitHref(to);
    const target = url.pathname + url.search + url.hash;
    if (
      !options.force &&
      url.pathname !== this.path &&
      leaveGuard.intercept(target)
    )
      return;
    if (options.replace) history.replaceState({}, '', target);
    else history.pushState({}, '', target);
    if (this.sync()) {
      document.getElementById('content')?.scrollTo?.({ top: 0 });
      window.scrollTo({ top: 0 });
      void focusContent();
    }
  }

  /** Merges query values into the current URL. */
  setQuery(
    values: Record<string, string | null | undefined>,
    options: { replace?: boolean } = { replace: true },
  ) {
    this.navigate(buildPath(this.path, values, this.search), options);
  }

  param(name: string): string {
    return this.query.get(name)?.trim() ?? '';
  }
}

export const router = new Router();

/**
 * Moves focus to the page's main landmark once the new page has rendered,
 * so keyboard and screen-reader users land on the content they asked for
 * instead of on the link they activated (UI-10). Query-only changes such as
 * switching a tab never move focus.
 */
export async function focusContent() {
  await tick();
  const main = document.getElementById('content');
  if (!main || main.contains(document.activeElement)) return;
  if (!main.hasAttribute('tabindex')) main.setAttribute('tabindex', '-1');
  main.focus({ preventScroll: true });
}

/**
 * Turns every same-origin `<a href>` click into a client-side navigation
 * unless the user asked for a new tab or the link opts out with
 * `data-native`.
 */
export function interceptLinks(root: Document = document) {
  const handler = (event: MouseEvent) => {
    if (event.defaultPrevented || event.button !== 0) return;
    if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey)
      return;
    const anchor = (event.target as Element | null)?.closest('a[href]');
    if (!(anchor instanceof HTMLAnchorElement)) return;
    if (anchor.target && anchor.target !== '_self') return;
    if (anchor.hasAttribute('download') || anchor.hasAttribute('data-native'))
      return;
    if (anchor.origin !== location.origin) return;
    if (anchor.pathname.startsWith('/api/')) return;
    if (
      !matchRoute(anchor.pathname) &&
      !resolveLegacyPath(anchor.pathname, anchor.search)
    )
      return;
    event.preventDefault();
    router.navigate(anchor.pathname + anchor.search + anchor.hash);
  };
  root.addEventListener('click', handler);
  return () => root.removeEventListener('click', handler);
}
