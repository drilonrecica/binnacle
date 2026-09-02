<script lang="ts">
  import { onMount } from 'svelte';
  import Download from '@lucide/svelte/icons/download';
  import FileSearch from '@lucide/svelte/icons/file-search';
  import type { SettingsSnapshot } from '../api/settings';
  import {
    createDiagnosticsPreview,
    diagnosticsDownloadUrl,
    monitorHealth,
    type DiagnosticsPreview,
    type MonitorMetric,
  } from '../api/access';
  import { errorMessage, isApiError } from '../api/client';
  import { formatBytes, formatNumber } from '../i18n';
  import Button from '../ui/Button.svelte';
  import Card from '../ui/Card.svelte';
  import RelativeTime from '../ui/RelativeTime.svelte';
  import Skeleton from '../ui/Skeleton.svelte';
  import StatusPill from '../ui/StatusPill.svelte';
  import { formatClock } from '../ui/relative-time';
  import { tooltip } from '../ui/tooltip';

  let { snapshot }: { snapshot: SettingsSnapshot | null } = $props();

  let metrics = $state<MonitorMetric[]>([]);
  let at = $state('');
  let healthError = $state('');
  let preview = $state<DiagnosticsPreview | null>(null);
  let generating = $state(false);
  let diagError = $state('');

  const rank: Record<string, number> = {
    critical: 0,
    error: 0,
    warning: 1,
    degraded: 1,
    normal: 3,
    healthy: 3,
    unavailable: 4,
  };
  const sorted = $derived(
    [...metrics].sort(
      (a, b) =>
        (rank[a.status] ?? 2) - (rank[b.status] ?? 2) ||
        a.label.localeCompare(b.label),
    ),
  );

  function display(metric: MonitorMetric) {
    if (metric.value == null) return '—';
    if (metric.unit === 'bytes') return formatBytes(Number(metric.value));
    if (metric.unit === 'percent')
      return `${formatNumber(Number(metric.value))}%`;
    if (metric.unit === 'milliseconds')
      return `${formatNumber(Number(metric.value))} ms`;
    if (typeof metric.value === 'number')
      return `${formatNumber(metric.value)}${metric.unit ? ` ${metric.unit}` : ''}`;
    return `${metric.value}${metric.unit ? ` ${metric.unit}` : ''}`;
  }

  async function loadHealth() {
    try {
      const value = await monitorHealth();
      metrics = value.metrics;
      at = value.at;
      healthError = '';
    } catch (reason) {
      healthError = errorMessage(reason);
    }
  }

  async function generate() {
    generating = true;
    diagError = '';
    try {
      preview = await createDiagnosticsPreview();
    } catch (reason) {
      diagError =
        isApiError(reason) && reason.status === 429
          ? 'Diagnostics can be generated three times per 15 minutes. Try again later.'
          : errorMessage(reason);
    } finally {
      generating = false;
    }
  }

  onMount(() => {
    void loadHealth();
    const timer = window.setInterval(() => {
      if (document.visibilityState === 'visible') void loadHealth();
    }, 5000);
    return () => window.clearInterval(timer);
  });

  const deployment = [
    'http.listen_address',
    'paths.data_dir',
    'docker.socket_path',
    'paths.host_proc',
    'paths.host_sys',
  ];
  const deploymentLabels: Record<string, string> = {
    'http.listen_address': 'Listen address',
    'paths.data_dir': 'Data directory',
    'docker.socket_path': 'Docker socket',
    'paths.host_proc': 'Host /proc',
    'paths.host_sys': 'Host /sys',
  };
</script>

<div class="stack">
  <Card
    title="Binnacle health"
    description="How the monitor itself is doing: memory, queues, database, and delivery workers. Refreshes every five seconds."
    padded={false}
  >
    {#snippet actions()}
      {#if at}<span class="stamp"
          >updated <span class="num">{formatClock(at)}</span></span
        >{/if}
    {/snippet}
    {#if healthError}
      <p class="error padded" role="alert">{healthError}</p>
    {:else if !metrics.length}
      <div class="padded"><Skeleton lines={4} height={28} /></div>
    {:else}
      <ul class="grid">
        {#each sorted as metric (metric.id)}
          <li class={`metric ${metric.status}`}>
            <div class="metric-head">
              <span class="metric-label" use:tooltip={metric.help}
                >{metric.label}</span
              >
              <StatusPill
                status={metric.status === 'normal'
                  ? 'healthy'
                  : metric.status === 'unavailable'
                    ? 'unknown'
                    : metric.status}
                size="sm"
              />
            </div>
            <span class="metric-value num">{display(metric)}</span>
            <span class="metric-help">{metric.help}</span>
          </li>
        {/each}
      </ul>
    {/if}
  </Card>

  <Card
    title="Diagnostics bundle"
    description="A reviewable archive of version, configuration, collector state, and self-metrics for support. Nothing leaves this server unless you download and share it."
  >
    {#snippet actions()}
      <Button onclick={generate} loading={generating}>
        {#snippet icon()}<FileSearch />{/snippet}
        Generate preview
      </Button>
    {/snippet}
    {#if diagError}<p class="error" role="alert">{diagError}</p>{/if}
    {#if preview}
      <div class="preview">
        <p class="stamp">
          Generated <RelativeTime value={preview.createdAt} /> · expires <RelativeTime
            value={preview.expiresAt}
          />
        </p>
        {#if preview.partialFailures?.length}
          <p class="warn">
            Some fields could not be collected: {preview.partialFailures.join(
              ', ',
            )}.
          </p>
        {/if}
        <details class="fields">
          <summary
            >Review the {Object.keys(preview.fields).length} included fields</summary
          >
          <dl>
            {#each Object.entries(preview.fields) as [key, value] (key)}
              <div>
                <dt>{key}</dt>
                <dd>
                  <pre>{typeof value === 'string'
                      ? value
                      : JSON.stringify(value, null, 2)}</pre>
                </dd>
              </div>
            {/each}
          </dl>
        </details>
        <div class="actions">
          <Button
            variant="primary"
            href={diagnosticsDownloadUrl(preview.id)}
            data-native
          >
            {#snippet icon()}<Download />{/snippet}
            Download bundle
          </Button>
        </div>
      </div>
    {/if}
  </Card>

  <Card
    title="Deployment"
    description="Values set by the container or config file. Change them in the deployment and restart."
  >
    <dl class="deployment">
      {#each deployment as key (key)}
        <div>
          <dt>{deploymentLabels[key] ?? key}</dt>
          <dd><code>{snapshot?.values[key]?.value || '—'}</code></dd>
        </div>
      {/each}
      <div>
        <dt>Feature flags</dt>
        <dd>
          <code
            >advanced_auth={String(
              Boolean(snapshot?.features.advancedAuth),
            )}</code
          >
          <code
            >portability={String(Boolean(snapshot?.features.portability))}</code
          >
        </dd>
      </div>
    </dl>
  </Card>
</div>

<style>
  .stack {
    display: grid;
    gap: var(--space-5);
  }
  .padded {
    padding: var(--space-4);
  }
  .error {
    color: var(--critical-fg);
    font-size: var(--text-sm);
  }
  .warn {
    color: var(--warn-fg);
    font-size: var(--text-sm);
  }
  .stamp {
    color: var(--text-3);
    font-size: var(--text-xs);
  }
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
    gap: 1px;
    margin: 0;
    padding: 0;
    background: var(--border);
    list-style: none;
  }
  .metric {
    display: grid;
    gap: var(--space-1);
    padding: var(--space-3) var(--space-4);
    background: var(--surface);
  }
  .metric-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
  }
  .metric-label {
    font-size: var(--text-sm);
    font-weight: 500;
  }
  .metric-value {
    font-size: var(--text-lg);
    font-weight: 500;
  }
  .metric-help {
    color: var(--text-3);
    font-size: var(--text-xs);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .preview {
    display: grid;
    gap: var(--space-3);
  }
  .fields summary {
    cursor: pointer;
    font-size: var(--text-sm);
    font-weight: 500;
  }
  .fields dl {
    display: grid;
    gap: var(--space-2);
    margin: var(--space-3) 0 0;
    max-height: 360px;
    overflow: auto;
    padding: var(--space-3);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--bg-subtle);
  }
  .fields dt {
    color: var(--text-3);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }
  .fields dd {
    margin: 0;
  }
  pre {
    margin: 0;
    font-size: var(--text-xs);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  .actions {
    display: flex;
    gap: var(--space-2);
  }
  .deployment {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--space-3);
    margin: 0;
  }
  .deployment div {
    display: grid;
    gap: 2px;
  }
  .deployment dt {
    color: var(--text-3);
    font-size: var(--text-xs);
  }
  .deployment dd {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    margin: 0;
  }
  .deployment code {
    font-size: var(--text-xs);
    overflow-wrap: anywhere;
  }
  @media (max-width: 720px) {
    .deployment {
      grid-template-columns: 1fr;
    }
  }
</style>
