<script lang="ts">
  import type { LiveStore } from './live.svelte';
  import Badge from './ui/Badge.svelte';
  import { formatBytes, formatNumber } from './i18n';
  import PostSetupChecklist from './PostSetupChecklist.svelte';
  import { prioritizedResources, staleResource } from './overview';
  let { live }: { live: LiveStore } = $props();
  let snapshot = $derived(live.snapshot);
  let resources = $derived(
    snapshot
      ? prioritizedResources(
          snapshot.resources.filter((resource) => !resource.infrastructure),
        )
      : [],
  );
  let infrastructure = $derived(
    snapshot?.resources.filter((resource) => resource.infrastructure) ?? [],
  );
  let unhealthy = $derived(
    resources.filter((resource) => resource.status !== 'healthy'),
  );
</script>

<PostSetupChecklist />
{#if !snapshot}
  <p role="status">Loading current telemetry…</p>
{:else}
  <section class="health-strip" aria-label="Server health summary">
    <article>
      <h2>Server</h2>
      <Badge state={live.state === 'connected' ? 'healthy' : 'unknown'}
        >{live.state}</Badge
      >
    </article>
    <article>
      <h2>CPU</h2>
      <strong>{formatNumber(snapshot.host.cpuPct)}%</strong>
    </article>
    <article>
      <h2>RAM</h2>
      <strong>{formatBytes(snapshot.host.memoryUsedBytes)}</strong><small>
        of {formatBytes(snapshot.host.memoryTotalBytes)}</small
      >
    </article>
    <article>
      <h2>Disk</h2>
      <strong>{formatBytes(snapshot.host.diskUsedBytes)}</strong><small>
        of {formatBytes(snapshot.host.diskTotalBytes)}</small
      >
    </article>
  </section>

  {#if live.state !== 'connected'}<p class="warning" role="status">
      Live updates are disconnected. Displayed values may be stale.
    </p>{/if}
  {#each Object.entries(snapshot.collectors).filter(([, collector]) => collector.state !== 'healthy') as [name, collector]}
    <p class="warning" role="status">
      {name} collector {collector.state}: {collector.reason ??
        'no current data'}
    </p>
  {/each}

  <div class="overview">
    <section class="card attention" aria-labelledby="attention-title">
      <h2 id="attention-title">Needs attention</h2>
      {#if unhealthy.length}
        {#each unhealthy as resource (resource.id)}
          <article>
            <a href={`/resources/${resource.id}`}
              ><strong>{resource.name}</strong></a
            >
            <Badge state={resource.status}>{resource.status}</Badge>
          </article>
        {/each}
      {:else}<p>No unhealthy active resources.</p>{/if}
    </section>

    <section
      class="card resources-card"
      aria-labelledby="active-resources-title"
    >
      <h2 id="active-resources-title">Applications and services</h2>
      {#if resources.length}
        {#each resources as resource (resource.id)}
          {@const stale = staleResource(resource, snapshot.ts)}
          <article class:stale>
            <h3><a href={`/resources/${resource.id}`}>{resource.name}</a></h3>
            <Badge state={stale ? 'unknown' : resource.status}
              >{stale ? 'stale' : resource.status}</Badge
            >
            {#if resource.project}<p>
                {resource.project}{resource.environment
                  ? ` / ${resource.environment}`
                  : ''}
              </p>{/if}
            <p>
              CPU {stale ? 'Stale' : `${formatNumber(resource.cpuHostPct)}%`} · Memory
              {stale ? 'Stale' : formatBytes(resource.memoryBytes)}
            </p>
          </article>
        {/each}
      {:else}<p>No active resources. Host monitoring remains available.</p>{/if}
    </section>

    <section class="card" aria-labelledby="infrastructure-title">
      <h2 id="infrastructure-title">Infrastructure</h2>
      {#if infrastructure.length}{#each infrastructure as resource (resource.id)}<p
          >
            <a href={`/resources/${resource.id}`}>{resource.name}</a>
            <Badge state={resource.status}>{resource.status}</Badge>
          </p>{/each}
      {:else}<p>No infrastructure resources detected.</p>{/if}
    </section>

    <section class="card" aria-labelledby="recent-events-title">
      <h2 id="recent-events-title">Recent events</h2>
      {#if live.events.length}{#each live.events
          .slice(-6)
          .reverse() as event (event.id)}<p>
            <a
              href={event.resourceId
                ? `/resources/${event.resourceId}`
                : '/events'}>{event.message}</a
            >
          </p>{/each}
      {:else}<p>No recent events.</p>{/if}
    </section>
  </div>
{/if}
