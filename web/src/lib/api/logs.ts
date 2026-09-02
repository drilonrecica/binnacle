import { api, withQuery } from './client';

export interface LogEntry {
  timestamp: string;
  component: string;
  stream: 'stdout' | 'stderr' | string;
  severity: string;
  message: string;
}

export interface LogResult {
  entries: LogEntry[];
  truncated: boolean;
  redaction: string;
}

export type LogRange = '5m' | '30m' | '1h' | 'custom';

export interface LogQuery {
  resource?: string;
  container?: string;
  range: LogRange;
  from?: Date;
  to?: Date;
  search?: string;
  limit?: number;
}

function queryFor(query: LogQuery, follow = false) {
  return {
    resource: query.container ? undefined : query.resource,
    container: query.container,
    range: query.range,
    from: query.range === 'custom' ? query.from?.toISOString() : undefined,
    to: query.range === 'custom' ? query.to?.toISOString() : undefined,
    search: query.search || undefined,
    limit: query.limit,
    follow: follow ? 'true' : undefined,
  };
}

export function fetchLogs(query: LogQuery, signal?: AbortSignal) {
  return api.get<LogResult>('/api/v1/logs', {
    query: queryFor(query),
    signal,
    fallback: 'Logs are unavailable.',
  });
}

/**
 * Streams log lines over SSE. The server ends the stream after 30 minutes
 * or when the limit is reached; `onend` fires either way.
 */
export function followLogs(
  query: LogQuery,
  handlers: {
    onentry: (entry: LogEntry) => void;
    onend: () => void;
    onerror: (message: string) => void;
  },
): () => void {
  const source = new EventSource(
    withQuery('/api/v1/logs', queryFor(query, true)),
  );
  let ended = false;
  source.addEventListener('log', (event) => {
    try {
      handlers.onentry(JSON.parse((event as MessageEvent).data) as LogEntry);
    } catch {
      /* Skip malformed frames. */
    }
  });
  source.addEventListener('end', () => {
    ended = true;
    source.close();
    handlers.onend();
  });
  source.onerror = () => {
    if (ended) return;
    source.close();
    handlers.onerror(
      'The live log stream stopped. Start it again to keep following.',
    );
  };
  return () => source.close();
}
