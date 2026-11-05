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
    type MonitorFailure,
    type MonitorLatency,
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
  let failures = $state<MonitorFailure[]>([]);
  let latency = $state<MonitorLatency[]>([]);
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

  function pillStatus(status: string) {
    if (status === 'normal') return 'healthy';
    if (status === 'unavailable') return 'unknown';
    return status;
  }

  function milliseconds(value: number | null) {
    return value == null ? '—' : `${formatNumber(value)} ms`;
  }

  async function loadHealth() {
    try {
      const value = await monitorHealth();
      metrics = value.metrics;
      failures = value.failures ?? [];
      latency = value.latency ?? [];
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
    'paths.host_root',
  ];
  const deploymentLabels: Record<string, string> = {
    'http.listen_address': 'Listen address',
    'paths.data_dir': 'Data directory',
    'docker.socket_path': 'Docker socket',
    'paths.host_proc': 'Host /proc',
    'paths.host_sys': 'Host /sys',
    'paths.host_root': 'Host root',
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

  {#if failures.length || latency.length}
    <Card
      title="Background workers"
      description="Failures since Binnacle started and how long the latest runs took. Every failure is in the log; repeated messages are logged once a minute."
      padded={false}
    >
      {#if failures.length}
        <ul class="grid" aria-label="Failures since start">
          {#each failures as failure (failure.id)}
            <li class={`metric ${failure.status}`}>
              <div class="metric-head">
                <span class="metric-label" use:tooltip={failure.help}
                  >{failure.label}</span
                >
                <StatusPill status={pillStatus(failure.status)} size="sm" />
              </div>
              <span class="metric-value num">{formatNumber(failure.count)}</span
              >
              <span class="metric-help">{failure.help}</span>
            </li>
          {/each}
        </ul>
      {/if}
      {#if latency.length}
        <div class="latency">
          <table>
            <caption
              >Latency of the latest runs (up to 256 per operation)</caption
            >
            <thead>
              <tr>
                <th scope="col">Operation</th>
                <th scope="col" class="n">p50</th>
                <th scope="col" class="n">p95</th>
                <th scope="col" class="n">Max</th>
                <th scope="col" class="n runs">Runs</th>
                <th scope="col"><span class="sr-only">Status</span></th>
              </tr>
            </thead>
            <tbody>
              {#each latency as row (row.id)}
                <tr>
                  <th scope="row">
                    <span use:tooltip={row.help}>{row.label}</span>
                  </th>
                  <td class="n num">{milliseconds(row.p50)}</td>
                  <td class="n num">{milliseconds(row.p95)}</td>
                  <td class="n num">{milliseconds(row.max)}</td>
                  <td class="n num runs">{formatNumber(row.count)}</td>
                  <td
                    ><StatusPill
                      status={pillStatus(row.status)}
                      size="sm"
                    /></td
                  >
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </Card>
  {/if}

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
    gap: var(--space-4);
  }
  .padded {
    padding: var(--space-4);
  }
  .error {
    color: var(--critical-fg);
    font-size: var(--text-meta);
  }
  .warn {
    color: var(--warn-fg);
    font-size: var(--text-meta);
  }
  .stamp {
    color: var(--muted);
    font-size: var(--text-mono);
  }
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
    gap: 1px;
    margin: 0;
    padding: 0;
    background: var(--line);
    list-style: none;
  }
  .metric {
    display: grid;
    gap: var(--space-1);
    padding: var(--space-3) var(--space-4);
    background: var(--panel);
  }
  .metric-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
  }
  .metric-label {
    font-size: var(--text-meta);
    font-weight: 500;
  }
  .metric-value {
    font-size: var(--text-h2);
    font-weight: 500;
  }
  .metric-help {
    color: var(--muted);
    font-size: var(--text-mono);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .latency {
    padding: var(--space-3) var(--space-4) var(--space-4);
  }
  .latency table {
    width: 100%;
    border-collapse: collapse;
    font-size: var(--text-meta);
  }
  .latency caption {
    padding-bottom: var(--space-2);
    color: var(--muted);
    font-size: var(--text-mono);
    text-align: left;
  }
  .latency th,
  .latency td {
    padding: var(--space-2) var(--space-2);
    border-bottom: 1px solid var(--line);
    text-align: left;
    white-space: nowrap;
  }
  .latency thead th {
    color: var(--muted);
    font-size: var(--text-mono);
    font-weight: 500;
  }
  .latency tbody th {
    font-weight: 500;
    white-space: normal;
  }
  .latency .n {
    text-align: right;
  }
  @media (max-width: 480px) {
    .latency .runs {
      display: none;
    }
  }
  .preview {
    display: grid;
    gap: var(--space-3);
  }
  .fields summary {
    cursor: pointer;
    font-size: var(--text-meta);
    font-weight: 500;
  }
  .fields dl {
    display: grid;
    gap: var(--space-2);
    margin: var(--space-3) 0 0;
    max-height: 360px;
    overflow: auto;
    padding: var(--space-3);
    border: 1px solid var(--line);
    border-radius: var(--radius-control);
    background: var(--bg);
  }
  .fields dt {
    color: var(--muted);
    font-family: var(--font-mono);
    font-size: var(--text-mono);
  }
  .fields dd {
    margin: 0;
  }
  pre {
    margin: 0;
    font-size: var(--text-mono);
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
    color: var(--muted);
    font-size: var(--text-mono);
  }
  .deployment dd {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    margin: 0;
  }
  .deployment code {
    font-size: var(--text-mono);
    overflow-wrap: anywhere;
  }
  @media (max-width: 720px) {
    .deployment {
      grid-template-columns: 1fr;
    }
  }
</style>
