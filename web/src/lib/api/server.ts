import { api } from './client';

/** What the machine is; read once by the server and fixed while it runs. */
export interface HostIdentity {
  hostname?: string;
  os?: string;
  kernel?: string;
  cpuModel?: string;
  cpuCores?: number;
}

/**
 * Slowly changing detail of one container. Served apart from the live frames
 * so pages that do not show it never download it.
 */
export interface ContainerDetail {
  id: string;
  /** The memory limit in force (host memory when unlimited). */
  memoryLimitBytes?: number | null;
  startedAt?: string;
  image?: string;
  /** Published or exposed ports, e.g. "8080->80/tcp". */
  ports?: string[];
}

export function fetchContainerDetails(signal?: AbortSignal) {
  return api
    .get<{ containers?: ContainerDetail[] }>('/api/v1/server/containers', {
      signal,
      fallback: 'Container details are unavailable.',
    })
    .then((value) => value.containers ?? []);
}

export function fetchServerIdentity(signal?: AbortSignal) {
  return api
    .get<{ identity?: HostIdentity }>('/api/v1/server', {
      signal,
      fallback: 'Server details are unavailable.',
    })
    .then((value) => value.identity ?? {});
}
