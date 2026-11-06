import {
  newSeenMark,
  parseSeen,
  pruneSeen,
  seenStorageKey,
  type SeenMark,
} from './seen';

export type { SeenMark } from './seen';

function storage(): Storage | null {
  try {
    return typeof localStorage === 'undefined' ? null : localStorage;
  } catch {
    return null;
  }
}

function readStored(): Record<string, SeenMark> {
  try {
    return parseSeen(storage()?.getItem(seenStorageKey) ?? null);
  } catch {
    return {};
  }
}

export class SeenMarkers {
  private marks = $state<Record<string, SeenMark>>(readStored());

  private all(): Record<string, SeenMark> {
    return this.marks;
  }

  private save(next: Record<string, SeenMark>) {
    this.marks = pruneSeen(next);
    try {
      storage()?.setItem(seenStorageKey, JSON.stringify(this.marks));
    } catch {
      // Storage full or blocked: the mark still holds for this page view.
    }
  }

  get(id: string): SeenMark | undefined {
    return this.all()[id];
  }

  mark(id: string, by: string, now = Date.now()) {
    this.save({ ...this.all(), [id]: newSeenMark(by, now) });
  }

  clear(id: string) {
    const next = { ...this.all() };
    delete next[id];
    this.save(next);
  }

  /** Re-read storage, e.g. after another tab changed it. */
  reload() {
    this.marks = readStored();
  }
}

export const seen = new SeenMarkers();
