import { SvelteSet } from 'svelte/reactivity';

/**
 * Keeps the user from leaving a page with unsaved edits (UI-21). Forms
 * register a dirty check; the router asks `intercept()` before it changes
 * the path, and the page's leave dialog settles `pending`. Closing or
 * reloading the tab falls back to the browser's own prompt.
 */
class LeaveGuard {
  private checks = new SvelteSet<() => boolean>();
  private unloadInstalled = false;

  /** Where the user tried to go while edits were unsaved. */
  pending = $state<string | null>(null);

  get blocked(): boolean {
    for (const check of this.checks) if (check()) return true;
    return false;
  }

  /** Adds a dirty check; the returned function removes it. */
  register(check: () => boolean): () => void {
    this.checks.add(check);
    this.installUnload();
    return () => {
      this.checks.delete(check);
      if (!this.checks.size) this.pending = null;
    };
  }

  /** True when leaving for `target` must wait for the user to confirm. */
  intercept(target: string): boolean {
    if (!this.blocked) return false;
    this.pending = target;
    return true;
  }

  /** Clears the request to leave; returns where it pointed. */
  settle(): string | null {
    const target = this.pending;
    this.pending = null;
    return target;
  }

  private installUnload() {
    if (this.unloadInstalled || typeof window === 'undefined') return;
    this.unloadInstalled = true;
    window.addEventListener('beforeunload', (event) => {
      if (!this.blocked) return;
      event.preventDefault();
      // Older engines show the prompt only when returnValue is set.
      event.returnValue = '';
    });
  }
}

export const leaveGuard = new LeaveGuard();
