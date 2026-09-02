<script lang="ts">
  import { onMount, tick } from 'svelte';
  import Search from '@lucide/svelte/icons/search';
  import CornerDownLeft from '@lucide/svelte/icons/corner-down-left';
  import Kbd from './Kbd.svelte';
  import { rankItems, type PaletteItem } from './palette';
  import { uniqueId } from './focus';
  import { router } from '../router.svelte';

  let {
    open = $bindable(false),
    items,
  }: {
    open?: boolean;
    /** Called on every open so callers can provide fresh items. */
    items: () => PaletteItem[];
  } = $props();

  let dialog = $state<HTMLDialogElement | null>(null);
  let input = $state<HTMLInputElement | null>(null);
  let query = $state('');
  let activeIndex = $state(0);
  const source = $derived<PaletteItem[]>(open ? items() : []);
  const listId = uniqueId('palette-list');

  const results = $derived(rankItems(source, query, 14));
  const grouped = $derived.by(() => {
    const groups: Array<[string, PaletteItem[]]> = [];
    for (const item of results) {
      const entry = groups.find(([group]) => group === item.group);
      if (entry) entry[1].push(item);
      else groups.push([item.group, [item]]);
    }
    return groups;
  });
  const activeItem = $derived(results[activeIndex]);

  $effect(() => {
    if (!dialog) return;
    if (open && !dialog.open) {
      query = '';
      activeIndex = 0;
      dialog.showModal();
      void tick().then(() => input?.focus());
    } else if (!open && dialog.open) {
      dialog.close();
    }
  });

  $effect(() => {
    void query;
    activeIndex = 0;
  });

  function choose(item: PaletteItem | undefined) {
    if (!item) return;
    open = false;
    if (item.href) router.navigate(item.href);
    item.action?.();
  }

  function onKeydown(event: KeyboardEvent) {
    if (event.key === 'ArrowDown') {
      event.preventDefault();
      activeIndex = Math.min(results.length - 1, activeIndex + 1);
    } else if (event.key === 'ArrowUp') {
      event.preventDefault();
      activeIndex = Math.max(0, activeIndex - 1);
    } else if (event.key === 'Enter') {
      event.preventDefault();
      choose(activeItem);
    }
  }

  $effect(() => {
    const id = activeItem?.id;
    if (!id || !dialog) return;
    dialog
      .querySelector(`[data-item="${CSS.escape(id)}"]`)
      ?.scrollIntoView({ block: 'nearest' });
  });

  onMount(() => {
    const onGlobalKey = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
        event.preventDefault();
        open = !open;
      }
    };
    window.addEventListener('keydown', onGlobalKey);
    return () => window.removeEventListener('keydown', onGlobalKey);
  });
</script>

<dialog
  bind:this={dialog}
  class="palette"
  aria-label="Command palette"
  onclose={() => (open = false)}
  onclick={(event) => {
    if (event.target === dialog) dialog?.close();
  }}
>
  {#if open}
    <div class="panel">
      <div class="search">
        <Search aria-hidden="true" />
        <input
          bind:this={input}
          bind:value={query}
          type="text"
          role="combobox"
          aria-expanded="true"
          aria-controls={listId}
          aria-activedescendant={activeItem
            ? `palette-item-${activeItem.id}`
            : undefined}
          aria-autocomplete="list"
          placeholder="Jump to a page or resource…"
          autocomplete="off"
          spellcheck="false"
          onkeydown={onKeydown}
        />
        <Kbd>esc</Kbd>
      </div>
      <div class="results" id={listId} role="listbox" aria-label="Results">
        {#if !results.length}
          <p class="none">No matches for “{query}”.</p>
        {/if}
        {#each grouped as [group, entries] (group)}
          <div class="group" role="group" aria-label={group}>
            <div class="group-label" aria-hidden="true">{group}</div>
            {#each entries as item (item.id)}
              {@const index = results.indexOf(item)}
              <button
                type="button"
                id={`palette-item-${item.id}`}
                data-item={item.id}
                role="option"
                aria-selected={index === activeIndex}
                class:active={index === activeIndex}
                onmousemove={() => (activeIndex = index)}
                onclick={() => choose(item)}
              >
                <span class="label">{item.label}</span>
                {#if item.hint}<span class="hint">{item.hint}</span>{/if}
                {#if index === activeIndex}<span
                    class="enter"
                    aria-hidden="true"><CornerDownLeft /></span
                  >{/if}
              </button>
            {/each}
          </div>
        {/each}
      </div>
    </div>
  {/if}
</dialog>

<style>
  .palette {
    width: min(100vw - 2rem, 600px);
    margin-top: 12vh;
    padding: 0;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-lg);
    background: var(--surface);
    color: var(--text);
    box-shadow: var(--shadow-lg);
    overflow: hidden;
  }
  .palette::backdrop {
    background: var(--overlay);
    backdrop-filter: blur(2px);
  }
  .palette[open] {
    animation: palette-in var(--motion) var(--ease-out);
  }
  @keyframes palette-in {
    from {
      opacity: 0;
      transform: translateY(-6px);
    }
  }
  .search {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-3) var(--space-4);
    border-bottom: 1px solid var(--border);
    color: var(--text-3);
  }
  .search :global(svg) {
    width: 18px;
    height: 18px;
    flex: none;
  }
  input {
    flex: 1;
    min-width: 0;
    border: 0;
    background: none;
    color: var(--text);
    font-size: var(--text-md);
  }
  input:focus {
    outline: none;
  }
  input::placeholder {
    color: var(--text-3);
  }
  .results {
    max-height: 50vh;
    padding: var(--space-2);
    overflow-y: auto;
  }
  .none {
    padding: var(--space-6);
    color: var(--text-2);
    text-align: center;
    font-size: var(--text-sm);
  }
  .group + .group {
    margin-top: var(--space-2);
  }
  .group-label {
    padding: var(--space-2) var(--space-2) var(--space-1);
    color: var(--text-3);
    font-size: var(--text-xs);
    font-weight: 600;
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
  }
  [role='option'] {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    width: 100%;
    min-height: 38px;
    padding: 0 var(--space-3);
    border: 0;
    border-radius: var(--radius-sm);
    background: none;
    color: var(--text);
    font-size: var(--text-sm);
    text-align: left;
  }
  [role='option'].active {
    background: var(--surface-2);
  }
  .label {
    flex: none;
    font-weight: 500;
  }
  .hint {
    flex: 1;
    min-width: 0;
    color: var(--text-3);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .enter {
    display: inline-flex;
    margin-left: auto;
    color: var(--text-3);
  }
  .enter :global(svg) {
    width: 14px;
    height: 14px;
  }
</style>
