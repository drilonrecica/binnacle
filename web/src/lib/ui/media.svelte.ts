/**
 * Reactive viewport shared across the shell and tables. The breakpoints are
 * the design system's three (ADR 027): 560, 900 and 1200 px, and nothing else.
 */
export const breakpoints = { sm: 560, md: 900, lg: 1200 } as const;

class Viewport {
  width = $state(typeof window === 'undefined' ? 1280 : window.innerWidth);
  /** Below 900 px: the sidebar becomes the tab bar and tables become cards. */
  readonly isMobile = $derived(this.width < breakpoints.md);
  /** Below 560 px: phone layouts. */
  readonly isNarrow = $derived(this.width < breakpoints.sm);
  readonly isWide = $derived(this.width >= breakpoints.lg);

  constructor() {
    if (typeof window === 'undefined') return;
    window.addEventListener('resize', () => (this.width = window.innerWidth), {
      passive: true,
    });
  }
}

export const viewport = new Viewport();
