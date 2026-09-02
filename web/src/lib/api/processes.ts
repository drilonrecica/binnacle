import { api } from './client';

export interface HostProcess {
  pid: number;
  command: string;
  cpuPct?: number | null;
  rssBytes?: number | null;
  user?: string;
  uid?: number;
  state?: string;
  uptimeSeconds?: number | null;
  containerId?: string;
}

export interface ProcessSample {
  processes: HostProcess[];
  sampled: boolean;
}

/** Samples host processes on demand; the server serializes scans. */
export function sampleProcesses(limit = 25, signal?: AbortSignal) {
  return api.get<ProcessSample>('/api/v1/processes', {
    query: { limit },
    signal,
    fallback: 'Host processes are unavailable.',
  });
}

export function processStateLabel(state: string | undefined): string {
  switch ((state ?? '').toUpperCase()) {
    case 'R':
      return 'Running';
    case 'S':
      return 'Sleeping';
    case 'D':
      return 'Waiting on I/O';
    case 'Z':
      return 'Zombie';
    case 'T':
      return 'Stopped';
    case 'I':
      return 'Idle';
    default:
      return state || '—';
  }
}
