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
    /**
     * Hide the column in table form below the `lg` breakpoint (1200 px).
     * Below 900 px every table is a list of cards, which show every column.
     */
    hideBelow?: 'lg';
    /** Screen-reader-only header (for action columns). */
    srOnly?: boolean;
    /**
     * Role in the card form: `title` heads the card (default: the first
     * column), `hide` leaves the column out of cards.
     */
    card?: 'title' | 'hide';
    cell?: import('svelte').Snippet<[T]>;
    /** Value used when sorting; falls back to the raw cell text when absent. */
    sortValue?: (row: T) => string | number | null | undefined;
  }
</script>

<script lang="ts" generics="T">
  import { missing } from '../i18n';
  import type { Snippet } from 'svelte';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import ArrowUp from '@lucide/svelte/icons/arrow-up';
  import ArrowDown from '@lucide/svelte/icons/arrow-down';
  import { SvelteSet } from 'svelte/reactivity';
  import { uniqueId } from './focus';
  import { breakpoints, viewport } from './media.svelte';
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
    layout = 'auto',
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
    /**
     * Custom card for one row. Without it the card form is built from the
     * columns, so every table has one.
     */
    mobileCard?: Snippet<[T]>;
    /** `auto` shows cards below 900 px and a table above. */
    layout?: 'auto' | 'table' | 'cards';
    density?: 'default' | 'compact';
    striped?: boolean;
  } = $props();

  const sortId = uniqueId('table-sort');

  const visibleColumns = $derived(
    columns.filter(
      (column) => column.hideBelow !== 'lg' || viewport.width >= breakpoints.lg,
    ),
  );

  const titleColumn = $derived(
    columns.find((column) => column.card === 'title') ??
      columns.find((column) => column.card !== 'hide' && !column.srOnly),
  );
  const cardFields = $derived(
    columns.filter(
      (column) =>
        column !== titleColumn && column.card !== 'hide' && !column.srOnly,
    ),
  );
  const cardActions = $derived(
    columns.filter((column) => column.srOnly && column.card !== 'hide'),
  );
  const sortableColumns = $derived(columns.filter((column) => column.sortable));

  function sortValue(column: Column<T>, row: T) {
    return column.sortValue
      ? column.sortValue(row)
      : (row as Record<string, unknown>)[column.key];
  }

  function sortText(value: unknown): string {
    if (typeof value === 'string') return value;
    if (typeof value === 'number' || typeof value === 'boolean')
      return String(value);
    return JSON.stringify(value) ?? '';
  }

  const sorted = $derived.by(() => {
    if (!sort) return rows;
    const column = columns.find((item) => item.key === sort?.key);
    if (!column) return rows;
    const direction = sort.direction === 'asc' ? 1 : -1;
    return [...rows].sort((left, right) => {
      const a = sortValue(column, left);
      const b = sortValue(column, right);
      if (a == null && b == null) return 0;
      if (a == null) return 1;
      if (b == null) return -1;
      if (typeof a === 'number' && typeof b === 'number')
        return (a - b) * direction;
      return (
        sortText(a).localeCompare(sortText(b), undefined, {
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
    // A scratch index rebuilt inside this derived; nothing reads it reactively.
    // eslint-disable-next-line svelte/prefer-svelte-reactivity
    const buckets = new Map<string, T[]>();
    for (const row of sorted) {
      const key = groupBy(row);
      const bucket = buckets.get(key);
      if (bucket) bucket.push(row);
      else buckets.set(key, [row]);
    }
    const keys = [...buckets.keys()].sort((a, b) => {
      const ia = groupOrder?.indexOf(a) ?? -1;
      const ib = groupOrder?.indexOf(b) ?? -1;
      if (ia >= 0 || ib >= 0)
        return (ia < 0 ? Infinity : ia) - (ib < 0 ? Infinity : ib);
      return a.localeCompare(b);
    });
    return keys.map((key) => ({ key, rows: buckets.get(key) ?? [] }));
  });

  function defaultDirection(column: Column<T>): SortDirection {
    const numeric = rows.some(
      (row) => typeof sortValue(column, row) === 'number',
    );
    return numeric ? 'desc' : 'asc';
  }

  function toggleSort(column: Column<T>) {
    if (!column.sortable) return;
    if (sort?.key === column.key) {
      sort = {
        key: column.key,
        direction: sort.direction === 'asc' ? 'desc' : 'asc',
      };
    } else {
      sort = { key: column.key, direction: defaultDirection(column) };
    }
  }

  function onSortSelect(event: Event) {
    const [key, direction] = (
      event.currentTarget as HTMLSelectElement
    ).value.split(':');
    if (!key) {
      sort = undefined;
      return;
    }
    sort = { key, direction: direction === 'asc' ? 'asc' : 'desc' };
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

  function rawValue(row: T, column: Column<T>): unknown {
    return (row as Record<string, unknown>)[column.key] ?? missing;
  }

  const interactive = $derived(Boolean(rowHref || onrowclick));
  const useCards = $derived(
    layout === 'cards' || (layout === 'auto' && viewport.isMobile),
  );
</script>

{#snippet cellContent(row: T, column: Column<T>)}
  {#if column.cell}{@render column.cell(row)}{:else}{rawValue(row, column)}{/if}
{/snippet}

{#snippet groupToggle(group: Group)}
  <button
    type="button"
    class="group-toggle"
    aria-expanded={!collapsedGroups.has(group.key)}
    onclick={() => toggleGroup(group.key)}
  >
    {#if collapsedGroups.has(group.key)}<ChevronRight
        aria-hidden="true"
      />{:else}<ChevronDown aria-hidden="true" />{/if}
    {#if groupLabel}{@render groupLabel({
        key: group.key,
        count: group.rows.length,
      })}{:else}<span>{group.key}</span>
      <span class="count">{group.rows.length}</span>{/if}
  </button>
{/snippet}

{#if loading}
  <div class="skeleton" role="status" aria-label={`Loading ${caption}`}>
    {#each Array.from({ length: skeletonRows }, (_, index) => index) as index (index)}
      <Skeleton height={density === 'compact' ? 28 : 36} />
    {/each}
  </div>
{:else if !rows.length}
  <div class="empty">{@render empty?.()}</div>
{:else if useCards}
  {#if sortableColumns.length}
    <div class="card-sort">
      <label for={sortId}>Sort</label>
      <select
        id={sortId}
        value={sort ? `${sort.key}:${sort.direction}` : ''}
        onchange={onSortSelect}
      >
        {#if !sort}<option value="">Default order</option>{/if}
        {#each sortableColumns as column (column.key)}
          <option value={`${column.key}:asc`}>{column.label}, ascending</option>
          <option value={`${column.key}:desc`}
            >{column.label}, descending</option
          >
        {/each}
      </select>
    </div>
  {/if}
  <ul class="cards" aria-label={caption}>
    {#each groups as group (group.key)}
      {#if groupBy}
        <li class="card-group">{@render groupToggle(group)}</li>
      {/if}
      {#if !collapsedGroups.has(group.key)}
        {#each group.rows as row (rowKey(row))}
          <!-- Pointer convenience only: the keyboard reaches the row through the
               link or buttons inside the card, as in the table form. -->
          <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
          <li
            class="card-row"
            class:interactive
            onclick={interactive
              ? (event) => rowActivate(event, row)
              : undefined}
          >
            {#if mobileCard}
              {@render mobileCard(row)}
            {:else}
              <div class="auto-card">
                {#if titleColumn}
                  <div class="card-title">
                    {@render cellContent(row, titleColumn)}
                  </div>
                {/if}
                {#if cardFields.length}
                  <dl class="card-fields">
                    {#each cardFields as column (column.key)}
                      <div class="card-field">
                        <dt>{column.label}</dt>
                        <dd class:num={column.align === 'right'}>
                          {@render cellContent(row, column)}
                        </dd>
                      </div>
                    {/each}
                  </dl>
                {/if}
                {#if cardActions.length}
                  <div class="card-actions">
                    {#each cardActions as column (column.key)}
                      {@render cellContent(row, column)}
                    {/each}
                  </div>
                {/if}
              </div>
            {/if}
          </li>
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
                {@render groupToggle(group)}
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
                    {@render cellContent(row, column)}
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
    font-size: var(--text-body);
  }
  th,
  td {
    height: var(--row-h);
    padding: 0 var(--space-3);
    border-bottom: 1px solid var(--line);
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
    height: var(--row-h-compact);
  }
  thead th {
    position: sticky;
    top: 0;
    z-index: 1;
    height: var(--row-h);
    background: var(--panel);
    color: var(--muted);
    font-size: var(--text-label);
    font-weight: var(--weight-semibold);
    letter-spacing: var(--tracking-label);
    text-align: left;
    text-transform: uppercase;
  }
  .right {
    text-align: right;
  }
  .center {
    text-align: center;
  }
  td.right {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    font-variant-numeric: tabular-nums;
  }
  .sort {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    padding: 0;
    border: 0;
    background: none;
    color: inherit;
    font: inherit;
    letter-spacing: inherit;
    text-transform: inherit;
  }
  .sort:hover {
    color: var(--fg);
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
    border-top: 1px solid var(--line);
  }
  .striped tbody tr:nth-child(even) td {
    background: var(--panel-2);
  }
  .interactive tbody tr:not(.group) {
    cursor: pointer;
  }
  .interactive tbody tr:not(.group):hover td {
    background: var(--panel-2);
  }
  tr.group th {
    height: var(--row-h-compact);
    padding-left: var(--space-3);
    background: var(--panel);
    text-align: left;
  }
  .group-toggle {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    padding: 2px var(--space-1) 2px 0;
    border: 0;
    border-radius: var(--radius-control);
    background: none;
    color: var(--muted);
    font-size: var(--text-label);
    font-weight: var(--weight-semibold);
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
  }
  .group-toggle:hover {
    color: var(--fg);
  }
  .group-toggle :global(svg) {
    width: 14px;
    height: 14px;
  }
  .count {
    padding: 0 var(--space-1);
    border-radius: var(--radius-control);
    background: var(--line);
    color: var(--muted);
    font-family: var(--font-mono);
    font-weight: var(--weight-medium);
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
  .card-sort {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: var(--space-2);
    padding: var(--space-3) var(--space-3) 0;
    color: var(--muted);
    font-size: var(--text-meta);
  }
  .card-sort select {
    height: var(--control-h-sm);
    max-width: 100%;
    padding: 0 var(--space-2);
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-control);
    background: var(--bg);
    color: var(--fg);
    font-size: var(--text-meta);
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
    min-width: 0;
    border: 1px solid var(--line);
    border-radius: var(--radius-card);
    background: var(--panel);
  }
  .card-row.interactive {
    cursor: pointer;
  }
  .card-row.interactive:hover {
    background: var(--panel-2);
  }
  .auto-card {
    display: grid;
    gap: var(--space-2);
    padding: var(--card-pad);
  }
  .card-title {
    min-width: 0;
    font-weight: var(--weight-semibold);
    overflow-wrap: anywhere;
  }
  .card-fields {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(min(100%, 140px), 1fr));
    gap: var(--space-2) var(--space-3);
    margin: 0;
  }
  .card-field {
    display: grid;
    gap: 2px;
    min-width: 0;
  }
  dt {
    color: var(--muted);
    font-size: var(--text-label);
    font-weight: var(--weight-semibold);
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
  }
  dd {
    min-width: 0;
    margin: 0;
    overflow-wrap: anywhere;
  }
  dd.num {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    font-variant-numeric: tabular-nums;
  }
  .card-actions {
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: var(--space-2);
  }
</style>
