<script lang="ts" module>
  export interface TabItem {
    id: string;
    label: string;
    count?: number | null;
  }
  export function tabPanelId(prefix: string, id: string) {
    return `${prefix}-panel-${id}`;
  }
  export function tabId(prefix: string, id: string) {
    return `${prefix}-tab-${id}`;
  }
</script>

<script lang="ts">
  import { router } from '../router.svelte';

  let {
    tabs,
    active = $bindable(),
    prefix,
    label,
    /** When set, the active tab is mirrored to this query parameter. */
    param,
    onchange,
  }: {
    tabs: TabItem[];
    active?: string;
    prefix: string;
    label: string;
    param?: string;
    onchange?: (id: string) => void;
  } = $props();

  $effect(() => {
    if (!param) return;
    const fromUrl = router.param(param);
    const valid = tabs.some((tab) => tab.id === fromUrl);
    const next = valid ? fromUrl : tabs[0]?.id;
    if (next && next !== active) active = next;
  });

  function select(id: string) {
    if (id === active) return;
    active = id;
    onchange?.(id);
    if (param) router.setQuery({ [param]: id === tabs[0]?.id ? null : id });
  }

  function onKeydown(event: KeyboardEvent, index: number) {
    let next: number;
    if (event.key === 'ArrowRight') next = (index + 1) % tabs.length;
    else if (event.key === 'ArrowLeft')
      next = (index - 1 + tabs.length) % tabs.length;
    else if (event.key === 'Home') next = 0;
    else if (event.key === 'End') next = tabs.length - 1;
    else return;
    event.preventDefault();
    const target = tabs[next];
    select(target.id);
    (event.currentTarget as HTMLElement).parentElement
      ?.querySelector<HTMLElement>(`#${CSS.escape(tabId(prefix, target.id))}`)
      ?.focus();
  }
</script>

<div class="tabs" role="tablist" aria-label={label}>
  {#each tabs as tab, index (tab.id)}
    <button
      type="button"
      role="tab"
      id={tabId(prefix, tab.id)}
      aria-selected={active === tab.id}
      aria-controls={tabPanelId(prefix, tab.id)}
      tabindex={active === tab.id ? 0 : -1}
      class:active={active === tab.id}
      onclick={() => select(tab.id)}
      onkeydown={(event) => onKeydown(event, index)}
    >
      {tab.label}
      {#if tab.count != null}<span class="count">{tab.count}</span>{/if}
    </button>
  {/each}
</div>

<style>
  .tabs {
    display: flex;
    gap: var(--space-1);
    border-bottom: 1px solid var(--line);
    overflow-x: auto;
    scrollbar-width: none;
  }
  .tabs::-webkit-scrollbar {
    display: none;
  }
  button {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    flex: none;
    height: 40px;
    margin-bottom: -1px;
    padding: 0 var(--space-3);
    border: 0;
    border-bottom: 2px solid transparent;
    background: none;
    color: var(--muted);
    font-size: var(--text-body);
    font-weight: var(--weight-medium);
    white-space: nowrap;
    transition:
      color var(--motion) var(--ease-out),
      border-color var(--motion) var(--ease-out);
  }
  button:hover {
    color: var(--fg);
  }
  button.active {
    border-bottom-color: var(--accent);
    color: var(--fg);
  }
  button:focus-visible {
    outline-offset: -2px;
    border-radius: var(--radius-control);
  }
  .count {
    padding: 1px var(--space-1);
    border-radius: var(--radius-control);
    background: var(--line);
    color: var(--muted);
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    font-variant-numeric: tabular-nums;
  }
  button.active .count {
    background: var(--accent-bg);
    color: var(--accent-text);
  }
</style>
