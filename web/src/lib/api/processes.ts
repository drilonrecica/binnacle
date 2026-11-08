import { missing } from '../i18n';
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
  threads?: number;
  nice?: number;
  virtBytes?: number;
  cpuTimeSeconds?: number;
  memPct?: number | null;
}

/** Counts over every process the scan saw, not only the returned rows. */
export interface TaskCounts {
  total: number;
  threads: number;
  running: number;
  sleeping: number;
  other: number;
}

export interface ProcessSample {
  processes: HostProcess[];
  tasks?: TaskCounts;
  /** When the server took the sample; a recent one is shared for 4 s. */
  sampledAt?: string;
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
      return state || missing;
  }
}
