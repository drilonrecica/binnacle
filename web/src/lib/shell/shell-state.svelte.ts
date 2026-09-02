import { listIncidents } from '../api/incidents';

/**
 * Small cross-cutting state for the shell: the command palette toggle and
 * the open-incident count shown as a badge on the Alerts nav item.
 */
class ShellState {
  paletteOpen = $state(false);
  openIncidents = $state<number | null>(null);
  private timer: number | undefined;

  startIncidentPolling(intervalMs = 30_000) {
    this.stopIncidentPolling();
    const poll = async () => {
      if (document.visibilityState !== 'visible') return;
      try {
        const incidents = await listIncidents({ status: 'open', limit: 100 });
        this.openIncidents = incidents.length;
      } catch {
        // The badge is advisory; the Alerts page reports errors itself.
      }
    };
    void poll();
    this.timer = window.setInterval(() => void poll(), intervalMs);
  }

  /** Called by pages that already fetched incidents so the badge stays fresh. */
  reportOpenIncidents(count: number) {
    this.openIncidents = count;
  }

  stopIncidentPolling() {
    if (this.timer) window.clearInterval(this.timer);
    this.timer = undefined;
  }
}

export const shell = new ShellState();
