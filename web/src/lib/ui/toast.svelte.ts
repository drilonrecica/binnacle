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
  duration?: number;
}

class ToastStore {
  items = $state<ToastItem[]>([]);
  private next = 1;
  private timers = new Map<number, number>();

  push(tone: ToastTone, title: string, options: ToastOptions = {}): number {
    const id = this.next++;
    const duration = options.duration ?? (tone === 'error' ? 8000 : 4500);
    this.items = [
      ...this.items.slice(-4),
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
      this.timers.set(
        id,
        window.setTimeout(() => this.dismiss(id), duration),
      );
    }
    return id;
  }

  dismiss(id: number) {
    const timer = this.timers.get(id);
    if (timer) window.clearTimeout(timer);
    this.timers.delete(id);
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
}

export const toasts = new ToastStore();
