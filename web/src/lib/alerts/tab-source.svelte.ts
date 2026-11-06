import { errorMessage } from '../api/client';

/**
 * One independently loaded list on the Incidents page. Each tab owns a
 * source, so a failed request marks only its own tab as failed instead of
 * emptying the page (UI-9). A refresh that fails after a good load keeps the
 * last rows and records the error, so the tab shows stale data with a
 * notice rather than an empty table.
 */
export class TabSource<T> {
  data = $state<T | null>(null);
  error = $state('');
  /** Epoch ms of the last successful load. */
  loadedAt = $state<number | null>(null);
  readonly fetcher: (signal?: AbortSignal) => Promise<T>;

  constructor(fetcher: (signal?: AbortSignal) => Promise<T>) {
    this.fetcher = fetcher;
  }

  /** Nothing loaded yet and nothing failed: show the loading state. */
  get loading(): boolean {
    return this.data === null && !this.error;
  }

  /** Failed before anything loaded: show an error, never an empty table. */
  get failed(): boolean {
    return this.data === null && this.error !== '';
  }

  /** Loaded once, but the latest refresh failed: show the rows as stale. */
  get stale(): boolean {
    return this.data !== null && this.error !== '';
  }

  settle(result: PromiseSettledResult<T>, now = Date.now()) {
    if (result.status === 'fulfilled') {
      this.data = result.value;
      this.error = '';
      this.loadedAt = now;
    } else {
      this.error = errorMessage(result.reason);
    }
  }

  load(signal?: AbortSignal) {
    return refreshSources([this], signal);
  }
}

/**
 * Fetches every source at once and settles each on its own. Results that
 * arrive after `signal` aborted (the page was left) are dropped.
 */
export async function refreshSources(
  sources: ReadonlyArray<TabSource<unknown>>,
  signal?: AbortSignal,
): Promise<void> {
  const results = await Promise.allSettled(
    sources.map((source) => source.fetcher(signal)),
  );
  if (signal?.aborted) return;
  const now = Date.now();
  results.forEach((result, index) => sources[index]?.settle(result, now));
}
