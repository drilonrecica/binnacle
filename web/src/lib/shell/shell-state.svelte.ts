import { listIncidents } from '../api/incidents';

/**
 * Small cross-cutting state for the shell: the command palette toggle and
 * the open-incident count shown as a badge on the Incidents nav item.
 *
 * Pages that already fetch open incidents (Headroom, Incidents) report the
 * count here; while such a report is fresher than the poll interval the
 * shell's own poll is skipped, so open incidents are fetched from one place
 * at a time rather than two (UI-9).
 */
class ShellState {
  paletteOpen = $state(false);
  openIncidents = $state<number | null>(null);
  private timer: number | undefined;
  private intervalMs = 30_000;
  private reportedAt = Number.NEGATIVE_INFINITY;

  startIncidentPolling(intervalMs = 30_000) {
    this.stopIncidentPolling();
    this.intervalMs = intervalMs;
    void this.pollOpenIncidents();
    this.timer = window.setInterval(
      () => void this.pollOpenIncidents(),
      intervalMs,
    );
  }

  async pollOpenIncidents(now = Date.now()) {
    if (document.visibilityState !== 'visible') return;
    if (now - this.reportedAt < this.intervalMs) return;
    try {
      const incidents = await listIncidents({ status: 'open', limit: 100 });
      this.openIncidents = incidents.length;
    } catch {
      // The badge is advisory; the Incidents page reports errors itself.
    }
  }

  /** Called by pages that already fetched incidents so the badge stays fresh. */
  reportOpenIncidents(count: number, now = Date.now()) {
    this.openIncidents = count;
    this.reportedAt = now;
  }

  stopIncidentPolling() {
    if (this.timer) window.clearInterval(this.timer);
    this.timer = undefined;
  }
}

export const shell = new ShellState();
