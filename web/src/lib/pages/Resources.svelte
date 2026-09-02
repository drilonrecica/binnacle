<script lang="ts">
  import { onMount } from 'svelte';
  import Archive from '@lucide/svelte/icons/archive';
  import Search from '@lucide/svelte/icons/search';
  import type { LiveStore } from '../live.svelte';
  import {
    listArchivedResources,
    resourceGroup,
    type ArchivedResource,
    type LiveResource,
  } from '../api/resources';
  import { fetchSparklines, type SparklineResponse } from '../api/metrics';
  import { errorMessage } from '../api/client';
  import { router } from '../router.svelte';
  import { resourcePath } from '../router';
  import { prefs } from '../preferences.svelte';
  import { prioritizedResources, staleResource } from '../watch';
  import ResourceTable from '../resources/ResourceTable.svelte';
  import Badge from '../ui/Badge.svelte';
  import Card from '../ui/Card.svelte';
  import DataTable, {
    type Column,
    type SortState,
  } from '../ui/DataTable.svelte';
  import EmptyState from '../ui/EmptyState.svelte';
  import Field from '../ui/Field.svelte';
  import Input from '../ui/Input.svelte';
  import PageHeader from '../ui/PageHeader.svelte';
  import RelativeTime from '../ui/RelativeTime.svelte';
  import Select from '../ui/Select.svelte';
  import StatusPill from '../ui/StatusPill.svelte';
  import Switch from '../ui/Switch.svelte';
  import Tabs, { tabPanelId, tabId } from '../ui/Tabs.svelte';
  import { categoryLabel } from '../api/resources';

  let { live }: { live: LiveStore } = $props();

  const sortKey = 'binnacle.resources.sort.v2';
  const groupKey = 'binnacle.resources.grouped';

  let tab = $state('active');
  let search = $state('');
  let status = $state(
    router.param('filter') === 'attention' ? 'attention' : 'all',
  );
  let project = $state('');
  let environment = $state('');
  let grouped = $state(true);
  let sort = $state<SortState | undefined>(undefined);
  let sparklines = $state<SparklineResponse | null>(null);
  let archived = $state<ArchivedResource[]>([]);
  let archivedLoading = $state(false);
  let archivedError = $state('');
  let archivedSort = $state<SortState | undefined>({
    key: 'archived',
    direction: 'desc',
  });

  const snapshot = $derived(live.snapshot);
  const all = $derived(
    snapshot
      ? prioritizedResources(
          snapshot.resources.filter(
            (resource) => resource.status !== 'archived',
          ),
          prefs.value.pinnedResources,
        )
      : [],
  );
  const projects = $derived(
    [
      ...new Set(all.map((resource) => resource.project).filter(Boolean)),
    ].sort() as string[],
  );
  const environments = $derived(
    [
      ...new Set(all.map((resource) => resource.environment).filter(Boolean)),
    ].sort() as string[],
  );

  const counts = $derived.by(() => {
    const out = { healthy: 0, degraded: 0, down: 0, other: 0 };
    for (const resource of all) {
      if (resource.status === 'healthy') out.healthy++;
      else if (resource.status === 'degraded') out.degraded++;
      else if (resource.status === 'down') out.down++;
      else out.other++;
    }
    return out;
  });

  function matchesStatus(resource: LiveResource) {
    if (!snapshot) return true;
    const stale = staleResource(resource, snapshot.ts);
    switch (status) {
      case 'attention':
        return resource.status !== 'healthy' || stale;
      case 'stale':
        return stale;
      case 'all':
        return true;
      default:
        return resource.status === status && !stale;
    }
  }

  const filtered = $derived.by(() => {
    const needle = search.trim().toLowerCase();
    return all.filter(
      (resource) =>
        matchesStatus(resource) &&
        (!project || resource.project === project) &&
        (!environment || resource.environment === environment) &&
        (!needle ||
          [
            resource.name,
            resource.project,
            resource.environment,
            resource.context,
            resource.category,
            resource.id,
          ]
            .filter(Boolean)
            .some((value) => String(value).toLowerCase().includes(needle))),
    );
  });

  const archivedFiltered = $derived.by(() => {
    const needle = search.trim().toLowerCase();
    if (!needle) return archived;
    return archived.filter((resource) =>
      [
        resource.name,
        resource.project,
        resource.environment,
        resource.context,
        resource.category,
      ]
        .filter(Boolean)
        .some((value) => String(value).toLowerCase().includes(needle)),
    );
  });

  const archivedColumns: Column<ArchivedResource>[] = [
    {
      key: 'name',
      label: 'Resource',
      sortable: true,
      sortValue: (row) => row.name,
      cell: archivedName,
    },
    {
      key: 'group',
      label: 'Project',
      sortable: true,
      sortValue: (row) => resourceGroup(row),
      hideBelow: 720,
    },
    {
      key: 'category',
      label: 'Category',
      sortable: true,
      sortValue: (row) => row.category,
      cell: archivedCategory,
      hideBelow: 900,
    },
    {
      key: 'archived',
      label: 'Archived',
      align: 'right',
      sortable: true,
      sortValue: (row) => (row.archivedAt ? Date.parse(row.archivedAt) : null),
      cell: archivedAt,
    },
  ];

  async function loadArchived() {
    archivedLoading = true;
    archivedError = '';
    try {
      archived = await listArchivedResources();
    } catch (reason) {
      archivedError = errorMessage(reason);
    } finally {
      archivedLoading = false;
    }
  }

  async function loadSparklines() {
    if (document.visibilityState !== 'visible') return;
    try {
      sparklines = await fetchSparklines(['cpu', 'memory'], '1h');
    } catch {
      /* Rows fall back to live samples. */
    }
  }

  $effect(() => {
    if (tab === 'archived' && !archived.length && !archivedLoading)
      void loadArchived();
  });

  $effect(() => {
    try {
      if (sort) localStorage.setItem(sortKey, JSON.stringify(sort));
      else localStorage.removeItem(sortKey);
      localStorage.setItem(groupKey, grouped ? 'yes' : 'no');
    } catch {
      /* Preferences are a convenience. */
    }
  });

  onMount(() => {
    try {
      const saved = JSON.parse(
        localStorage.getItem(sortKey) ?? 'null',
      ) as SortState | null;
      if (
        saved?.key &&
        (saved.direction === 'asc' || saved.direction === 'desc')
      )
        sort = saved;
      grouped = localStorage.getItem(groupKey) !== 'no';
    } catch {
      /* ignore */
    }
    void loadSparklines();
    const timer = window.setInterval(() => void loadSparklines(), 60_000);
    return () => window.clearInterval(timer);
  });
</script>

{#snippet archivedName(row: ArchivedResource)}
  <div class="archived-name">
    <a href={resourcePath(row.id)}>{row.name}</a>
    <span class="archived-group">{resourceGroup(row)}</span>
  </div>
{/snippet}
{#snippet archivedCategory(row: ArchivedResource)}
  <Badge tone="neutral">{categoryLabel(row.category)}</Badge>
{/snippet}
{#snippet archivedAt(row: ArchivedResource)}
  <RelativeTime value={row.archivedAt} />
{/snippet}
{#snippet archivedEmpty()}
  <EmptyState
    title="No archived resources"
    description="Resources that disappear for more than five minutes are archived here with their history."
  >
    {#snippet icon()}<Archive />{/snippet}
  </EmptyState>
{/snippet}

<PageHeader
  title="Resources"
  description="Logical services and their containers, grouped by project when Compose or Coolify metadata is available."
>
  {#snippet meta()}
    <StatusPill
      status="healthy"
      label={`${counts.healthy} healthy`}
      size="sm"
    />
    {#if counts.degraded}<StatusPill
        status="degraded"
        label={`${counts.degraded} degraded`}
        size="sm"
      />{/if}
    {#if counts.down}<StatusPill
        status="down"
        label={`${counts.down} down`}
        size="sm"
      />{/if}
    {#if counts.other}<StatusPill
        status="unknown"
        label={`${counts.other} other`}
        size="sm"
      />{/if}
  {/snippet}
  <Tabs
    prefix="resources"
    label="Resource views"
    param="tab"
    bind:active={tab}
    tabs={[
      { id: 'active', label: 'Active', count: all.length },
      {
        id: 'archived',
        label: 'Archived',
        count: tab === 'archived' ? archived.length : null,
      },
    ]}
  />
</PageHeader>

<div class="toolbar">
  <div class="search">
    <Input
      type="search"
      placeholder={tab === 'archived'
        ? 'Filter archived resources'
        : 'Filter by name, project, or environment'}
      bind:value={search}
      aria-label="Filter resources"
    >
      {#snippet leading()}<Search />{/snippet}
    </Input>
  </div>
  {#if tab === 'active'}
    <Select size="sm" bind:value={status} aria-label="Status filter">
      <option value="all">All statuses</option>
      <option value="attention">Needs attention</option>
      <option value="healthy">Healthy</option>
      <option value="degraded">Degraded</option>
      <option value="down">Down</option>
      <option value="unknown">Unknown</option>
      <option value="stale">Stale</option>
    </Select>
    {#if projects.length}
      <Select size="sm" bind:value={project} aria-label="Project filter">
        <option value="">All projects</option>
        {#each projects as item (item)}<option value={item}>{item}</option
          >{/each}
      </Select>
    {/if}
    {#if environments.length > 1}
      <Select
        size="sm"
        bind:value={environment}
        aria-label="Environment filter"
      >
        <option value="">All environments</option>
        {#each environments as item (item)}<option value={item}>{item}</option
          >{/each}
      </Select>
    {/if}
    {#if projects.length}
      <Field label="Group by project" inline id="group-toggle">
        {#snippet children({ id })}
          <Switch {id} bind:checked={grouped} />
        {/snippet}
      </Field>
    {/if}
  {/if}
</div>

<div
  id={tabPanelId('resources', 'active')}
  role="tabpanel"
  aria-labelledby={tabId('resources', 'active')}
  hidden={tab !== 'active'}
>
  {#if tab === 'active'}
    <Card padded={false}>
      {#if snapshot}
        <ResourceTable
          resources={filtered}
          snapshotTs={snapshot.ts}
          {sparklines}
          {grouped}
          bind:sort
          emptyTitle={all.length ? 'No matching resources' : 'No resources yet'}
          emptyDescription={all.length
            ? 'Try a different filter or clear the search.'
            : 'Containers appear here as soon as the Docker collector sees them.'}
        />
      {:else}
        <div class="waiting" role="status">
          Waiting for the first live sample…
        </div>
      {/if}
    </Card>
  {/if}
</div>

<div
  id={tabPanelId('resources', 'archived')}
  role="tabpanel"
  aria-labelledby={tabId('resources', 'archived')}
  hidden={tab !== 'archived'}
>
  {#if tab === 'archived'}
    <Card padded={false}>
      {#if archivedError}
        <p class="error" role="alert">{archivedError}</p>
      {:else}
        <DataTable
          rows={archivedFiltered}
          columns={archivedColumns}
          rowKey={(row) => row.id}
          caption="Archived resources"
          bind:sort={archivedSort}
          rowHref={(row) => resourcePath(row.id)}
          loading={archivedLoading}
          empty={archivedEmpty}
        />
      {/if}
      {#snippet footer()}
        <p class="note">
          Archived resources keep their history until it is deleted. Binnacle
          never restores or controls workloads.
        </p>
      {/snippet}
    </Card>
  {/if}
</div>

<style>
  .toolbar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2) var(--space-3);
    margin-bottom: var(--space-4);
  }
  .search {
    width: min(100%, 320px);
  }
  .toolbar :global(.select-wrap) {
    width: auto;
  }
  .waiting {
    padding: var(--space-8);
    color: var(--text-2);
    text-align: center;
  }
  .error {
    padding: var(--space-4);
    color: var(--critical-fg);
    font-size: var(--text-sm);
  }
  .archived-name {
    display: grid;
    gap: 2px;
  }
  .archived-name a {
    font-weight: 600;
  }
  .archived-group {
    color: var(--text-3);
    font-size: var(--text-xs);
  }
  .note {
    margin-right: auto;
    color: var(--text-3);
    font-size: var(--text-xs);
  }
</style>
