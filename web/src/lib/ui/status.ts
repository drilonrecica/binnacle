/** Shared vocabulary for status colors and labels across the UI. */

export type Tone = 'ok' | 'warn' | 'critical' | 'info' | 'neutral' | 'accent';

export type NormalizedStatus =
  | 'healthy'
  | 'degraded'
  | 'down'
  | 'unknown'
  | 'paused'
  | 'archived'
  | 'stale'
  | 'starting';

const healthyWords = new Set([
  'healthy',
  'normal',
  'passed',
  'ok',
  'succeeded',
  'resolved',
  'connected',
]);
const degradedWords = new Set([
  'warning',
  'degraded',
  'recovering',
  'pending',
  'in_progress',
  'partial',
]);
const downWords = new Set([
  'down',
  'error',
  'critical',
  'failed',
  'firing',
  'permanent_failure',
  'disconnected',
]);

export function normalizeStatus(
  raw: string | null | undefined,
): NormalizedStatus {
  const value = (raw ?? '').toLowerCase();
  if (value === 'paused') return 'paused';
  if (value === 'archived') return 'archived';
  if (value === 'stale') return 'stale';
  if (value === 'starting') return 'starting';
  if (healthyWords.has(value)) return 'healthy';
  if (degradedWords.has(value)) return 'degraded';
  if (downWords.has(value)) return 'down';
  return 'unknown';
}

export function statusTone(status: NormalizedStatus): Tone {
  switch (status) {
    case 'healthy':
      return 'ok';
    case 'degraded':
    case 'starting':
      return 'warn';
    case 'down':
      return 'critical';
    case 'paused':
      return 'info';
    default:
      return 'neutral';
  }
}

export function statusLabel(status: NormalizedStatus): string {
  switch (status) {
    case 'healthy':
      return 'Healthy';
    case 'degraded':
      return 'Degraded';
    case 'down':
      return 'Down';
    case 'paused':
      return 'Paused';
    case 'archived':
      return 'Archived';
    case 'stale':
      return 'Stale';
    case 'starting':
      return 'Starting';
    default:
      return 'Unknown';
  }
}

export type Severity = 'info' | 'warning' | 'critical';

export function severityTone(severity: string | null | undefined): Tone {
  switch ((severity ?? '').toLowerCase()) {
    case 'critical':
      return 'critical';
    case 'warning':
      return 'warn';
    case 'info':
      return 'info';
    default:
      return 'neutral';
  }
}

export function severityLabel(severity: string | null | undefined): string {
  switch ((severity ?? '').toLowerCase()) {
    case 'critical':
      return 'Critical';
    case 'warning':
      return 'Warning';
    case 'info':
      return 'Info';
    default:
      return 'Unknown';
  }
}

/** Tone for a utilization percentage. */
export function utilizationTone(
  percent: number | null | undefined,
  warn = 80,
  critical = 95,
): Tone {
  if (percent == null || !Number.isFinite(percent)) return 'neutral';
  if (percent >= critical) return 'critical';
  if (percent >= warn) return 'warn';
  return 'ok';
}
