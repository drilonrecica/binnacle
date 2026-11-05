export type ToastTone = 'success' | 'error' | 'info' | 'warning';

export interface ToastAction {
  label: string;
  onclick: () => void;
}

export interface ToastItem {
  id: number;
  tone: ToastTone;
  title: string;
  description?: string;
  action?: ToastAction;
  /** Milliseconds before auto-dismiss; 0 keeps it until dismissed. */
  duration: number;
}

export interface ToastOptions {
  description?: string;
  action?: ToastAction;
  /** Ignored when the toast has an action: those stay until dismissed. */
  duration?: number;
}

/** Why a toast's timer is held: the pointer is over it or focus is inside it. */
export type HoldReason = 'hover' | 'focus';

interface Timer {
  handle: number | undefined;
  remaining: number;
  startedAt: number;
  holds: Set<HoldReason>;
}

export const DEFAULT_DURATION = 5000;
export const ERROR_DURATION = 8000;
const MAX_VISIBLE = 5;

/**
 * Toast queue (WCAG 2.2.1, UI-17): a toast with an action never times out,
 * and every timer pauses while the toast is hovered or holds focus, resuming
 * with the time it had left.
 */
export class ToastStore {
  items = $state<ToastItem[]>([]);
  private next = 1;
  // Timer bookkeeping is never rendered, so it stays out of the reactive graph.
  // eslint-disable-next-line svelte/prefer-svelte-reactivity
  private timers = new Map<number, Timer>();

  push(tone: ToastTone, title: string, options: ToastOptions = {}): number {
    const id = this.next++;
    const duration = options.action
      ? 0
      : (options.duration ??
        (tone === 'error' ? ERROR_DURATION : DEFAULT_DURATION));
    // Keep the newest few; the oldest make room and lose their timers.
    const overflow = Math.max(0, this.items.length - (MAX_VISIBLE - 1));
    for (const dropped of this.items.slice(0, overflow))
      this.clearTimer(dropped.id);
    const kept = this.items.slice(overflow);
    this.items = [
      ...kept,
      {
        id,
        tone,
        title,
        description: options.description,
        action: options.action,
        duration,
      },
    ];
    if (duration > 0) {
      this.timers.set(id, {
        handle: undefined,
        remaining: duration,
        startedAt: 0,
        // eslint-disable-next-line svelte/prefer-svelte-reactivity
        holds: new Set(),
      });
      this.start(id);
    }
    return id;
  }

  /** Stops the countdown for `reason` until `resume` is called with it. */
  pause(id: number, reason: HoldReason) {
    const timer = this.timers.get(id);
    if (!timer) return;
    if (!timer.holds.size && timer.handle !== undefined) {
      window.clearTimeout(timer.handle);
      timer.handle = undefined;
      timer.remaining = Math.max(
        0,
        timer.remaining - (Date.now() - timer.startedAt),
      );
    }
    timer.holds.add(reason);
  }

  resume(id: number, reason: HoldReason) {
    const timer = this.timers.get(id);
    if (!timer || !timer.holds.delete(reason) || timer.holds.size) return;
    this.start(id);
  }

  dismiss(id: number) {
    this.clearTimer(id);
    this.items = this.items.filter((item) => item.id !== id);
  }

  success(title: string, options?: ToastOptions) {
    return this.push('success', title, options);
  }
  error(title: string, options?: ToastOptions) {
    return this.push('error', title, options);
  }
  info(title: string, options?: ToastOptions) {
    return this.push('info', title, options);
  }
  warning(title: string, options?: ToastOptions) {
    return this.push('warning', title, options);
  }

  private start(id: number) {
    const timer = this.timers.get(id);
    if (!timer) return;
    timer.startedAt = Date.now();
    timer.handle = window.setTimeout(() => this.dismiss(id), timer.remaining);
  }

  private clearTimer(id: number) {
    const timer = this.timers.get(id);
    if (timer?.handle !== undefined) window.clearTimeout(timer.handle);
    this.timers.delete(id);
  }
}

export const toasts = new ToastStore();
