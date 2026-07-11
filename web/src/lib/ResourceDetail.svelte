<script lang="ts">
  import { onMount } from 'svelte';
  import type { LiveStore } from './live.svelte';
  import Badge from './ui/Badge.svelte';
  import { formatBytes, formatNumber } from './i18n';
  import HistoryCharts from './HistoryCharts.svelte';
  import HistoryDeletion from './HistoryDeletion.svelte';
  let { live, id }: { live: LiveStore; id: string } = $props();
  let current = $derived(
    live.snapshot?.resources.find((value) => value.id === id),
  );
  let archived = $state<{
    id: string;
    name: string;
    status: string;
    category: string;
    project?: string;
    environment?: string;
    archivedAt?: string;
  } | null>(null);
  let error = $state('');
  onMount(() => {
    if (current) return;
    void fetch(`/api/v1/resources/${encodeURIComponent(id)}`, {
      credentials: 'same-origin',
    })
      .then((response) => {
        if (!response.ok) throw new Error('Resource unavailable.');
        return response.json();
      })
      .then((value) => (archived = value))
      .catch((reason) => (error = String(reason)));
  });
</script>

{#if current}
  <section class="card">
    <h2>{current.name}</h2>
    <Badge state={current.status}>{current.status}</Badge>
    <p>
      {current.category}{current.project
        ? ` · ${current.project}`
        : ''}{current.environment ? ` / ${current.environment}` : ''}
    </p>
    <p>CPU: {formatNumber(current.cpuHostPct)}% of host</p>
    <p>Memory: {formatBytes(current.memoryBytes)}</p>
    {#if current.components?.length}<details>
        <summary>{current.components.length} components</summary>
        <ul>
          {#each current.components as component (component.id)}<li>
              {component.name} — {component.status}
            </li>{/each}
        </ul>
      </details>{/if}
    <details>
      <summary>Technical details</summary><code>{current.id}</code>
    </details>
  </section>
{:else if archived}
  <section class="card archived-detail">
    <h2>{archived.name}</h2>
    <Badge state="archived">archived</Badge>
    <p>
      This workload is no longer active. Historical telemetry remains available
      until explicitly purged.
    </p>
    <p>
      {archived.category}{archived.project
        ? ` · ${archived.project}`
        : ''}{archived.environment ? ` / ${archived.environment}` : ''}
    </p>
    {#if archived.archivedAt}<p>
        Archived {new Date(archived.archivedAt).toLocaleString()}
      </p>{/if}
  </section>
{:else if error}<p role="alert">{error}</p>{:else}<p role="status">
    Loading resource…
  </p>{/if}

{#if current || archived}<HistoryCharts
    scope="resource"
    {id}
    metrics={[
      'cpu',
      'memory',
      'network_rx',
      'network_tx',
      'block_read',
      'block_write',
    ]}
  />{/if}
{#if archived}<HistoryDeletion archivedResourceId={id} />{/if}
