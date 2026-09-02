import {
  buildPath,
  matchRoute,
  resolveLegacyPath,
  splitHref,
  type RouteMatch,
} from './router';

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

  /** Reads the current browser location, applying legacy redirects. */
  sync() {
    const legacy = resolveLegacyPath(location.pathname, location.search);
    if (legacy) {
      history.replaceState({}, '', legacy);
    }
    this.path = location.pathname;
    this.search = location.search;
  }

  install() {
    if (this.installed) return;
    this.installed = true;
    this.sync();
    addEventListener('popstate', () => this.sync());
  }

  navigate(to: string, options: { replace?: boolean } = {}) {
    const url = splitHref(to);
    const changedPath = url.pathname !== location.pathname;
    const target = url.pathname + url.search + url.hash;
    if (options.replace) history.replaceState({}, '', target);
    else history.pushState({}, '', target);
    this.sync();
    if (changedPath) {
      document.getElementById('content')?.scrollTo?.({ top: 0 });
      window.scrollTo({ top: 0 });
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
