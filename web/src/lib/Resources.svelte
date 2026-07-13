<script lang="ts">
  import { onMount } from 'svelte';
  import type { LiveStore } from './live.svelte';
  import { formatBytes, formatNumber } from './i18n';
  import ConsoleSection from './ui/ConsoleSection.svelte';
  import ConsoleState from './ui/ConsoleState.svelte';
  import {
    defaultDirection,
    loadSortPreference,
    resourceContext,
    resourceStatusLabel,
    saveSortPreference,
    sortResources,
    type ActiveSortField,
    type ArchivedSortField,
    type SortDirection,
  } from './resource-sort';
  type Archived = {
    id: string;
    name: string;
    status: string;
    category: string;
    context?: string;
    project?: string;
    environment?: string;
    archivedAt?: string;
  };
  let { live }: { live: LiveStore } = $props();
  let archived = $state<Archived[]>([]);
  let error = $state('');
  let loadingArchived = $state(false);
  let showArchived = $state(
    new URLSearchParams(location.search).get('view') === 'archived',
  );
  let resources = $derived(live.snapshot?.resources ?? []);
  let activeField = $state<ActiveSortField>('name');
  let activeDirection = $state<SortDirection>('asc');
  let archivedField = $state<ArchivedSortField>('name');
  let archivedDirection = $state<SortDirection>('asc');
  let sortedActive = $derived(
    sortResources(resources, {
      field: activeField,
      direction: activeDirection,
    }),
  );
  let sortedArchived = $derived(
    sortResources(archived, {
      field: archivedField,
      direction: archivedDirection,
    }),
  );
  let currentDirection = $derived(
    showArchived ? archivedDirection : activeDirection,
  );
  onMount(() => {
    const activePreference = loadSortPreference('active');
    activeField = activePreference.field;
    activeDirection = activePreference.direction;
    const archivedPreference = loadSortPreference('archived');
    archivedField = archivedPreference.field;
    archivedDirection = archivedPreference.direction;
    if (!showArchived) return;
    loadingArchived = true;
    void fetch('/api/v1/resources?state=archived', {
      credentials: 'same-origin',
    })
      .then((response) => {
        if (!response.ok)
          throw new Error('Archived resources are unavailable.');
        return response.json() as Promise<Archived[]>;
      })
      .then((values) => (archived = values))
      .catch((reason) => (error = String(reason)))
      .finally(() => (loadingArchived = false));
  });
  function changeField(value: string) {
    if (showArchived) {
      archivedField = value as ArchivedSortField;
      archivedDirection = defaultDirection(archivedField);
      saveSortPreference('archived', {
        field: archivedField,
        direction: archivedDirection,
      });
    } else {
      activeField = value as ActiveSortField;
      activeDirection = defaultDirection(activeField);
      saveSortPreference('active', {
        field: activeField,
        direction: activeDirection,
      });
    }
  }
  function toggleDirection() {
    if (showArchived) {
      archivedDirection = archivedDirection === 'asc' ? 'desc' : 'asc';
      saveSortPreference('archived', {
        field: archivedField,
        direction: archivedDirection,
      });
    } else {
      activeDirection = activeDirection === 'asc' ? 'desc' : 'asc';
      saveSortPreference('active', {
        field: activeField,
        direction: activeDirection,
      });
    }
  }
</script>

<section class="console-page" aria-labelledby="resources-title">
  <ConsoleSection
    code="ROSTER"
    title="Resources"
    id="resources-title"
    detail={showArchived
      ? `${archived.length} archived`
      : `${resources.length} active`}
  />
  <nav class="control-rail resource-tabs" aria-label="Resource views">
    <span>STATE</span><a
      href="/resources"
      aria-current={!showArchived ? 'page' : undefined}>Active</a
    >
    <a
      href="/resources?view=archived"
      aria-current={showArchived ? 'page' : undefined}>Archived</a
    >
  </nav>
  <div class="resource-sort-controls">
    <label for="resource-sort">Sort by</label>
    <select
      id="resource-sort"
      value={showArchived ? archivedField : activeField}
      onchange={(event) => changeField(event.currentTarget.value)}
    >
      <option value="name">Name</option>
      {#if showArchived}
        <option value="context">Context</option>
        <option value="archived">Archived date</option>
      {:else}
        <option value="state">State</option>
        <option value="cpu">CPU</option>
        <option value="memory">Memory</option>
        <option value="context">Context</option>
        <option value="components">Components</option>
      {/if}
    </select>
    <button
      type="button"
      onclick={toggleDirection}
      aria-label={`Sort ${currentDirection === 'asc' ? 'descending' : 'ascending'}`}
      >{currentDirection === 'asc' ? '↑ Ascending' : '↓ Descending'}</button
    >
  </div>
  {#if error}<p class="console-notice" role="alert">{error}</p>
  {:else if loadingArchived}<p class="console-empty" role="status">
      Loading archived resources…
    </p>
  {:else if showArchived && !archived.length}<p class="console-empty">
      No archived resources.
    </p>
  {:else if !showArchived && !resources.length}<p class="console-empty">
      No active resources. Host monitoring remains available.
    </p>
  {:else}
    <div class="table-scroll">
      <table class="console-table resource-roster">
        <thead
          ><tr
            ><th>State</th><th>Resource</th><th>Context</th><th>CPU</th><th
              >Memory</th
            ><th>Components</th>{#if showArchived}<th>Archived</th>{/if}</tr
          ></thead
        >
        <tbody
          >{#each showArchived ? sortedArchived : sortedActive as resource (resource.id)}
            <tr data-state={showArchived ? 'archived' : resource.status}>
              <td
                ><ConsoleState
                  state={showArchived ? 'unknown' : resource.status}
                  label={showArchived
                    ? 'archived'
                    : resourceStatusLabel(resource)}
                /></td
              >
              <th scope="row"
                ><a href={`/resources/${resource.id}`}>{resource.name}</a></th
              >
              <td>{resourceContext(resource)}</td>
              <td
                >{showArchived
                  ? '—'
                  : `${formatNumber('cpuHostPct' in resource ? resource.cpuHostPct : null)}%`}</td
              >
              <td
                >{showArchived
                  ? '—'
                  : formatBytes(
                      'memoryBytes' in resource ? resource.memoryBytes : null,
                    )}</td
              >
              <td
                >{showArchived
                  ? '—'
                  : 'components' in resource
                    ? (resource.components?.length ?? 0)
                    : '—'}</td
              >
              {#if showArchived}<td
                  >{'archivedAt' in resource && resource.archivedAt
                    ? new Date(resource.archivedAt).toLocaleString()
                    : '—'}</td
                >{/if}
            </tr>
          {/each}</tbody
        >
      </table>
    </div>
    {#if showArchived}<p class="console-caption">
        Archived resources are historical only. Binnacle cannot control or
        restore workloads.
      </p>{/if}
  {/if}
</section>
