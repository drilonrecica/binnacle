<script lang="ts" module>
  export type SortDirection = 'asc' | 'desc';
  export interface SortState {
    key: string;
    direction: SortDirection;
  }
  export interface Column<T> {
    key: string;
    label: string;
    sortable?: boolean;
    align?: 'left' | 'right' | 'center';
    width?: string;
    /** Hide the column when the viewport is narrower than this many pixels. */
    hideBelow?: number;
    /** Screen-reader-only header (for action columns). */
    srOnly?: boolean;
    cell?: import('svelte').Snippet<[T]>;
    /** Value used when sorting; falls back to the raw cell text when absent. */
    sortValue?: (row: T) => string | number | null | undefined;
  }
</script>

<script lang="ts" generics="T">
  import type { Snippet } from 'svelte';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import ArrowUp from '@lucide/svelte/icons/arrow-up';
  import ArrowDown from '@lucide/svelte/icons/arrow-down';
  import { SvelteSet } from 'svelte/reactivity';
  import { viewport } from './media.svelte';
  import Skeleton from './Skeleton.svelte';

  let {
    rows,
    columns,
    rowKey,
    caption,
    sort = $bindable(),
    rowHref,
    onrowclick,
    loading = false,
    skeletonRows = 5,
    empty,
    groupBy,
    groupLabel,
    groupOrder,
    collapsedGroups = $bindable(new Set<string>()),
    mobileCard,
    density = 'default',
    striped = false,
  }: {
    rows: T[];
    columns: Column<T>[];
    rowKey: (row: T) => string;
    caption: string;
    sort?: SortState;
    /** Makes the whole row a link to this path. */
    rowHref?: (row: T) => string;
    onrowclick?: (row: T) => void;
    loading?: boolean;
    skeletonRows?: number;
    empty?: Snippet;
    groupBy?: (row: T) => string;
    groupLabel?: Snippet<[{ key: string; count: number }]>;
    /** Explicit group order; unknown groups follow alphabetically. */
    groupOrder?: string[];
    collapsedGroups?: Set<string>;
    /** When the viewport is narrow, render each row with this snippet instead. */
    mobileCard?: Snippet<[T]>;
    density?: 'default' | 'compact';
    striped?: boolean;
  } = $props();

  const visibleColumns = $derived(
    columns.filter(
      (column) => !column.hideBelow || viewport.width >= column.hideBelow,
    ),
  );

  const sorted = $derived.by(() => {
    if (!sort) return rows;
    const column = columns.find((item) => item.key === sort?.key);
    if (!column) return rows;
    const value = (row: T) =>
      column.sortValue
        ? column.sortValue(row)
        : (row as Record<string, unknown>)[column.key];
    const direction = sort.direction === 'asc' ? 1 : -1;
    return [...rows].sort((left, right) => {
      const a = value(left);
      const b = value(right);
      if (a == null && b == null) return 0;
      if (a == null) return 1;
      if (b == null) return -1;
      if (typeof a === 'number' && typeof b === 'number')
        return (a - b) * direction;
      return (
        String(a).localeCompare(String(b), undefined, {
          numeric: true,
          sensitivity: 'base',
        }) * direction
      );
    });
  });

  interface Group {
    key: string;
    rows: T[];
  }

  const groups = $derived.by<Group[]>(() => {
    if (!groupBy) return [{ key: '', rows: sorted }];
    const buckets: Array<{ key: string; rows: T[] }> = [];
    for (const row of sorted) {
      const key = groupBy(row);
      const bucket = buckets.find((item) => item.key === key);
      if (bucket) bucket.rows.push(row);
      else buckets.push({ key, rows: [row] });
    }
    const keys = buckets
      .map((bucket) => bucket.key)
      .sort((a, b) => {
        const ia = groupOrder?.indexOf(a) ?? -1;
        const ib = groupOrder?.indexOf(b) ?? -1;
        if (ia >= 0 || ib >= 0)
          return (ia < 0 ? Infinity : ia) - (ib < 0 ? Infinity : ib);
        return a.localeCompare(b);
      });
    return keys.map((key) => ({
      key,
      rows: buckets.find((bucket) => bucket.key === key)?.rows ?? [],
    }));
  });

  function toggleSort(column: Column<T>) {
    if (!column.sortable) return;
    if (sort?.key === column.key) {
      sort = {
        key: column.key,
        direction: sort.direction === 'asc' ? 'desc' : 'asc',
      };
    } else {
      const numeric = rows.some(
        (row) =>
          typeof (column.sortValue
            ? column.sortValue(row)
            : (row as Record<string, unknown>)[column.key]) === 'number',
      );
      sort = { key: column.key, direction: numeric ? 'desc' : 'asc' };
    }
  }

  function toggleGroup(key: string) {
    const next = new SvelteSet(collapsedGroups);
    if (next.has(key)) next.delete(key);
    else next.add(key);
    collapsedGroups = next;
  }

  function rowActivate(event: MouseEvent, row: T) {
    const target = event.target as HTMLElement;
    if (target.closest('a, button, input, select, textarea, [role="menu"]'))
      return;
    if (rowHref) {
      const href = rowHref(row);
      const anchor = (
        event.currentTarget as HTMLElement
      ).querySelector<HTMLAnchorElement>(`a[href="${CSS.escape(href)}"]`);
      if (anchor) {
        anchor.click();
        return;
      }
    }
    onrowclick?.(row);
  }

  const interactive = $derived(Boolean(rowHref || onrowclick));
  const useCards = $derived(Boolean(mobileCard) && viewport.isNarrow);
</script>

{#if loading}
  <div class="skeleton" role="status" aria-label={`Loading ${caption}`}>
    {#each Array.from({ length: skeletonRows }, (_, index) => index) as index (index)}
      <Skeleton height={density === 'compact' ? 28 : 36} />
    {/each}
  </div>
{:else if !rows.length}
  <div class="empty">{@render empty?.()}</div>
{:else if useCards}
  <ul class="cards" aria-label={caption}>
    {#each groups as group (group.key)}
      {#if groupBy}
        <li class="card-group">
          <button
            type="button"
            class="group-toggle"
            aria-expanded={!collapsedGroups.has(group.key)}
            onclick={() => toggleGroup(group.key)}
          >
            {#if collapsedGroups.has(group.key)}<ChevronRight
              />{:else}<ChevronDown />{/if}
            {#if groupLabel}{@render groupLabel({
                key: group.key,
                count: group.rows.length,
              })}{:else}<span>{group.key}</span>
              <span class="count">{group.rows.length}</span>{/if}
          </button>
        </li>
      {/if}
      {#if !collapsedGroups.has(group.key)}
        {#each group.rows as row (rowKey(row))}
          <li class="card-row">{@render mobileCard?.(row)}</li>
        {/each}
      {/if}
    {/each}
  </ul>
{:else}
  <div class="scroll">
    <table class={`table ${density}`} class:striped class:interactive>
      <caption class="sr-only">{caption}</caption>
      <thead>
        <tr>
          {#each visibleColumns as column (column.key)}
            <th
              scope="col"
              class={column.align ?? 'left'}
              style:width={column.width}
              aria-sort={sort?.key === column.key
                ? sort.direction === 'asc'
                  ? 'ascending'
                  : 'descending'
                : column.sortable
                  ? 'none'
                  : undefined}
            >
              {#if column.sortable}
                <button
                  type="button"
                  class="sort"
                  onclick={() => toggleSort(column)}
                >
                  <span class:sr-only={column.srOnly}>{column.label}</span>
                  <span class="sort-icon" aria-hidden="true">
                    {#if sort?.key === column.key}
                      {#if sort.direction === 'asc'}<ArrowUp />{:else}<ArrowDown
                        />{/if}
                    {/if}
                  </span>
                </button>
              {:else}
                <span class:sr-only={column.srOnly}>{column.label}</span>
              {/if}
            </th>
          {/each}
        </tr>
      </thead>
      {#each groups as group (group.key)}
        <tbody>
          {#if groupBy}
            <tr class="group">
              <th scope="colgroup" colspan={visibleColumns.length}>
                <button
                  type="button"
                  class="group-toggle"
                  aria-expanded={!collapsedGroups.has(group.key)}
                  onclick={() => toggleGroup(group.key)}
                >
                  {#if collapsedGroups.has(group.key)}<ChevronRight
                    />{:else}<ChevronDown />{/if}
                  {#if groupLabel}{@render groupLabel({
                      key: group.key,
                      count: group.rows.length,
                    })}{:else}<span>{group.key}</span>
                    <span class="count">{group.rows.length}</span>{/if}
                </button>
              </th>
            </tr>
          {/if}
          {#if !collapsedGroups.has(group.key)}
            {#each group.rows as row (rowKey(row))}
              <tr
                onclick={interactive
                  ? (event) => rowActivate(event, row)
                  : undefined}
              >
                {#each visibleColumns as column (column.key)}
                  <td class={column.align ?? 'left'}>
                    {#if column.cell}{@render column.cell(row)}{:else}{(
                        row as Record<string, unknown>
                      )[column.key] ?? '—'}{/if}
                  </td>
                {/each}
              </tr>
            {/each}
          {/if}
        </tbody>
      {/each}
    </table>
  </div>
{/if}

<style>
  .scroll {
    width: 100%;
    overflow-x: auto;
  }
  .table {
    width: 100%;
    font-size: var(--text-sm);
  }
  th,
  td {
    padding: 0 var(--space-3);
    height: var(--row-h);
    border-bottom: 1px solid var(--border);
    vertical-align: middle;
    white-space: nowrap;
  }
  td:first-child,
  th:first-child {
    padding-left: var(--space-4);
  }
  td:last-child,
  th:last-child {
    padding-right: var(--space-4);
  }
  .compact th,
  .compact td {
    height: calc(var(--row-h) - 8px);
  }
  thead th {
    position: sticky;
    top: 0;
    z-index: 1;
    height: 36px;
    background: var(--bg-subtle);
    color: var(--text-2);
    font-size: var(--text-xs);
    font-weight: 500;
    letter-spacing: var(--tracking-label);
    text-align: left;
  }
  .right {
    text-align: right;
  }
  .center {
    text-align: center;
  }
  td.right {
    font-family: var(--font-mono);
    font-variant-numeric: tabular-nums;
  }
  .sort {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 0;
    border: 0;
    background: none;
    color: inherit;
    font: inherit;
    letter-spacing: inherit;
  }
  .sort:hover {
    color: var(--text);
  }
  .sort-icon {
    display: inline-flex;
    width: 12px;
  }
  .sort-icon :global(svg) {
    width: 12px;
    height: 12px;
  }
  .right .sort {
    flex-direction: row-reverse;
  }
  tbody tr:last-child td {
    border-bottom-color: transparent;
  }
  tbody + tbody tr:first-child th,
  tbody + tbody tr:first-child td {
    border-top: 1px solid var(--border);
  }
  .striped tbody tr:nth-child(even) td {
    background: var(--bg-subtle);
  }
  .interactive tbody tr:not(.group) {
    cursor: pointer;
  }
  .interactive tbody tr:not(.group):hover td {
    background: var(--surface-2);
  }
  tr.group th {
    height: 32px;
    padding-left: var(--space-3);
    background: var(--surface);
    text-align: left;
  }
  .group-toggle {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    padding: 2px 4px 2px 0;
    border: 0;
    border-radius: var(--radius-sm);
    background: none;
    color: var(--text-2);
    font-size: var(--text-xs);
    font-weight: 600;
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
  }
  .group-toggle:hover {
    color: var(--text);
  }
  .group-toggle :global(svg) {
    width: 14px;
    height: 14px;
  }
  .count {
    padding: 0 6px;
    border-radius: var(--radius-full);
    background: var(--surface-3);
    color: var(--text-2);
    font-family: var(--font-mono);
    font-weight: 500;
    text-transform: none;
  }
  .skeleton {
    display: grid;
    gap: var(--space-2);
    padding: var(--space-4);
  }
  .empty {
    padding: var(--space-6) var(--space-4);
  }
  .cards {
    display: grid;
    gap: var(--space-2);
    margin: 0;
    padding: var(--space-3);
    list-style: none;
  }
  .card-group {
    padding-top: var(--space-2);
  }
  .card-row {
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface);
  }
</style>
