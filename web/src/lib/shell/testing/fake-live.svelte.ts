import type {
  ConnectionState,
  LiveEvent,
  LiveSnapshot,
} from '../../live.svelte';

/** A LiveStore stand-in whose state a test sets directly. */
export class FakeLive {
  snapshot = $state<LiveSnapshot | null>(null);
  events = $state<LiveEvent[]>([]);
  state = $state<ConnectionState>('disconnected');
  error = $state('');
  lastReceivedAt = $state<string | null>(null);
  lastEventId = '';
  ageMs() {
    return null;
  }
  connect() {
    this.state = 'live';
  }
  close() {
    this.state = 'disconnected';
  }
  retry() {}
}

export const fakeLive = { current: null as FakeLive | null };
