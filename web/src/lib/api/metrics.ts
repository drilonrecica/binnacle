import { api } from './client';
import type { HistoryResponse, Metric } from '../history';

export type SparklineMetric = 'cpu' | 'memory' | 'network_rx' | 'network_tx';
export type SparklineRange = '1h' | '3h' | '6h';

export interface SparklineResponse {
  from: string;
  to: string;
  stepSeconds: number;
  resources: Record<
    string,
    Partial<Record<SparklineMetric, Array<number | null>>>
  >;
}

/** One request that feeds every row sparkline on a resource list. */
export function fetchSparklines(
  metrics: SparklineMetric[],
  range: SparklineRange = '1h',
  signal?: AbortSignal,
) {
  return api.get<SparklineResponse>('/api/v1/metrics/sparklines', {
    query: { metrics: metrics.join(','), range },
    signal,
    fallback: 'Sparklines are unavailable.',
  });
}

export function fetchMetrics(
  scope: 'host' | 'resource',
  metrics: Metric[],
  from: Date,
  to: Date,
  id?: string,
  signal?: AbortSignal,
) {
  return api.get<HistoryResponse>('/api/v1/metrics', {
    query: {
      scope,
      id: scope === 'resource' ? id : undefined,
      metrics: metrics.join(','),
      from: from.toISOString(),
      to: to.toISOString(),
    },
    signal,
    fallback: 'Historical metrics are unavailable.',
  });
}
