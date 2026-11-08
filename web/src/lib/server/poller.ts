/**
 * Runs `run` now and then every `intervalMs` after the previous run settles,
 * only while the document is visible. The next run is scheduled when one
 * finishes, so runs never overlap; a hidden page schedules nothing and runs
 * again as soon as it is visible. Returns a stop function that cancels the
 * timer, aborts a run in flight and removes the visibility listener.
 */
export interface PollerOptions<T> {
  intervalMs: number;
  run: (signal: AbortSignal) => Promise<T>;
  onResult: (value: T) => void;
  onError: (reason: unknown) => void;
  /** Defaults to the document's visibility. */
  isVisible?: () => boolean;
  /** Where visibility changes are heard; defaults to `document`. */
  events?: Pick<Document, 'addEventListener' | 'removeEventListener'>;
}

export function startPoller<T>(options: PollerOptions<T>): () => void {
  const visible =
    options.isVisible ?? (() => document.visibilityState === 'visible');
  const events = options.events ?? document;
  let timer: ReturnType<typeof setTimeout> | null = null;
  let controller: AbortController | null = null;
  let stopped = false;

  const schedule = () => {
    if (stopped || timer != null || controller != null || !visible()) return;
    timer = setTimeout(tick, options.intervalMs);
  };

  const tick = () => {
    timer = null;
    if (stopped || controller != null || !visible()) return;
    const current = new AbortController();
    controller = current;
    options
      .run(current.signal)
      .then(
        (value) => {
          if (!stopped && !current.signal.aborted) options.onResult(value);
        },
        (reason: unknown) => {
          if (!stopped && !current.signal.aborted) options.onError(reason);
        },
      )
      .finally(() => {
        if (controller === current) controller = null;
        schedule();
      });
  };

  const onVisibility = () => {
    if (!visible()) {
      if (timer != null) clearTimeout(timer);
      timer = null;
      return;
    }
    // Back in view: refresh now rather than waiting out a full interval.
    if (timer == null && controller == null) tick();
  };

  events.addEventListener('visibilitychange', onVisibility);
  tick();

  return () => {
    stopped = true;
    if (timer != null) clearTimeout(timer);
    timer = null;
    controller?.abort();
    controller = null;
    events.removeEventListener('visibilitychange', onVisibility);
  };
}
