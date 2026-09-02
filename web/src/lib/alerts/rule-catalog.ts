/** Human descriptions for the built-in alert families. */

export interface RuleInfo {
  title: string;
  description: string;
  /** Unit for the threshold, when the family has one. */
  unit?: '%' | 'events';
  scope: string;
  editable: Array<
    | 'threshold'
    | 'recoveryThreshold'
    | 'triggerSeconds'
    | 'recoverySeconds'
    | 'severity'
  >;
}

export const ruleCatalog: Record<string, RuleInfo> = {
  host_cpu_warning: {
    title: 'Host CPU',
    description:
      'Host CPU busy time stays above the threshold for the trigger duration.',
    unit: '%',
    scope: 'Host',
    editable: [
      'threshold',
      'recoveryThreshold',
      'triggerSeconds',
      'recoverySeconds',
      'severity',
    ],
  },
  host_memory_warning: {
    title: 'Host memory',
    description:
      'Used host memory stays above the threshold for the trigger duration.',
    unit: '%',
    scope: 'Host',
    editable: [
      'threshold',
      'recoveryThreshold',
      'triggerSeconds',
      'recoverySeconds',
      'severity',
    ],
  },
  filesystem_warning: {
    title: 'Filesystem usage',
    description: 'A mounted filesystem is filling up.',
    unit: '%',
    scope: 'Every mount',
    editable: [
      'threshold',
      'recoveryThreshold',
      'triggerSeconds',
      'recoverySeconds',
      'severity',
    ],
  },
  filesystem_critical: {
    title: 'Filesystem nearly full',
    description: 'A mounted filesystem is about to run out of space.',
    unit: '%',
    scope: 'Every mount',
    editable: [
      'threshold',
      'recoveryThreshold',
      'triggerSeconds',
      'recoverySeconds',
      'severity',
    ],
  },
  inode_warning: {
    title: 'Inode usage',
    description: 'A filesystem is running low on inodes even if bytes remain.',
    unit: '%',
    scope: 'Every mount',
    editable: [
      'threshold',
      'recoveryThreshold',
      'triggerSeconds',
      'recoverySeconds',
      'severity',
    ],
  },
  inode_critical: {
    title: 'Inodes nearly exhausted',
    description: 'A filesystem is about to run out of inodes.',
    unit: '%',
    scope: 'Every mount',
    editable: [
      'threshold',
      'recoveryThreshold',
      'triggerSeconds',
      'recoverySeconds',
      'severity',
    ],
  },
  restart_storm: {
    title: 'Restart storm',
    description:
      'A resource restarts repeatedly within a short window. Paused during deployments.',
    unit: 'events',
    scope: 'Every resource',
    editable: ['threshold', 'recoverySeconds', 'severity'],
  },
  oom_loop: {
    title: 'Out-of-memory loop',
    description:
      'Containers of a resource are killed by the kernel OOM handler repeatedly.',
    unit: 'events',
    scope: 'Every resource',
    editable: ['threshold', 'recoverySeconds', 'severity'],
  },
  required_check_failure: {
    title: 'Required check failing',
    description:
      'A required HTTP check keeps failing. The resource is reported as down.',
    scope: 'Required checks',
    editable: ['triggerSeconds', 'recoverySeconds', 'severity'],
  },
  optional_check_failure: {
    title: 'Optional check failing',
    description:
      'An optional HTTP check keeps failing. The resource is reported as degraded.',
    scope: 'Optional checks',
    editable: ['triggerSeconds', 'recoverySeconds', 'severity'],
  },
  docker_collector_down: {
    title: 'Docker collector down',
    description:
      'Binnacle cannot reach the Docker API, so container data is stale.',
    scope: 'Docker collector',
    editable: ['triggerSeconds', 'recoverySeconds', 'severity'],
  },
  persistence_failure: {
    title: 'Storage failure',
    description: 'History cannot be written to the local database.',
    scope: 'Binnacle storage',
    editable: ['recoverySeconds', 'severity'],
  },
};

export function ruleInfo(family: string, fallbackName: string): RuleInfo {
  return (
    ruleCatalog[family] ?? {
      title: fallbackName,
      description: '',
      scope: 'Custom',
      editable: [
        'threshold',
        'recoveryThreshold',
        'triggerSeconds',
        'recoverySeconds',
        'severity',
      ],
    }
  );
}

export function formatSeconds(seconds: number | undefined): string {
  if (!seconds) return 'instant';
  if (seconds % 3600 === 0) return `${seconds / 3600}h`;
  if (seconds % 60 === 0) return `${seconds / 60}m`;
  return `${seconds}s`;
}
