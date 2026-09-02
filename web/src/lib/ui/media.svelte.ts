/** Reactive viewport breakpoints shared across the shell and tables. */
class Viewport {
  width = $state(typeof window === 'undefined' ? 1280 : window.innerWidth);
  readonly isMobile = $derived(this.width < 900);
  readonly isNarrow = $derived(this.width < 720);

  constructor() {
    if (typeof window === 'undefined') return;
    window.addEventListener('resize', () => (this.width = window.innerWidth), {
      passive: true,
    });
  }
}

export const viewport = new Viewport();
