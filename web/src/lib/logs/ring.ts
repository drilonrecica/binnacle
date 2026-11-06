import type { LogEntry } from '../api/logs';

/** One stored line with a sequence number that never repeats, for keys. */
export interface LogLine {
  seq: number;
  entry: LogEntry;
}

/** Lines kept per view; older lines fall off the front. */
export const logCapacity = 5_000;
/** Lines rendered at once; earlier ones are revealed on request. */
export const logPageSize = 400;

/**
 * A fixed-size ring of log lines. Pushing a streamed line is O(1) and never
 * copies the buffer (UI-15: the Logs page used to copy a 5,000-entry array
 * per line). Views read a window with `tail()`; `version` changes on every
 * mutation so a component can hold the ring in `$state.raw` and react to a
 * counter instead of to a new array.
 */
export class LogRing {
  readonly capacity: number;
  private slots: Array<LogLine | undefined>;
  private start = 0;
  private count = 0;
  private nextSeq = 0;
  /** Bumped on every change; total lines ever pushed are `dropped + size`. */
  version = 0;
  /** Lines that fell off the front since the last `clear()`. */
  dropped = 0;

  constructor(capacity = logCapacity) {
    if (!Number.isInteger(capacity) || capacity < 1)
      throw new Error('capacity must be a positive integer');
    this.capacity = capacity;
    this.slots = new Array<LogLine | undefined>(capacity);
  }

  get size(): number {
    return this.count;
  }

  /** The newest line, if any. */
  get last(): LogLine | undefined {
    return this.count ? this.at(this.count - 1) : undefined;
  }

  push(entry: LogEntry): void {
    const line = { seq: this.nextSeq++, entry };
    if (this.count < this.capacity) {
      this.slots[(this.start + this.count) % this.capacity] = line;
      this.count++;
    } else {
      this.slots[this.start] = line;
      this.start = (this.start + 1) % this.capacity;
      this.dropped++;
    }
    this.version++;
  }

  pushAll(entries: Iterable<LogEntry>): void {
    for (const entry of entries) this.push(entry);
  }

  /** Replaces the contents, keeping only the newest `capacity` entries. */
  replace(entries: readonly LogEntry[]): void {
    this.clear();
    this.pushAll(entries.slice(-this.capacity));
  }

  clear(): void {
    this.slots = new Array<LogLine | undefined>(this.capacity);
    this.start = 0;
    this.count = 0;
    this.dropped = 0;
    this.version++;
  }

  /** The line at `index`, oldest first. */
  at(index: number): LogLine | undefined {
    if (index < 0 || index >= this.count) return undefined;
    return this.slots[(this.start + index) % this.capacity];
  }

  /** The newest `limit` lines, oldest first. */
  tail(limit: number): LogLine[] {
    const length = Math.max(0, Math.min(limit, this.count));
    const out = new Array<LogLine>(length);
    const offset = this.count - length;
    for (let index = 0; index < length; index++) {
      out[index] = this.slots[
        (this.start + offset + index) % this.capacity
      ] as LogLine;
    }
    return out;
  }
}
