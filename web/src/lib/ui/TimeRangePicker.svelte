<script lang="ts" module>
  import type { RangeKey } from '../history';
  export interface TimeRange {
    key: RangeKey;
    from: Date;
    to: Date;
  }
</script>

<script lang="ts">
  import { onMount, tick } from 'svelte';
  import CalendarRange from '@lucide/svelte/icons/calendar-range';
  import { rangeFor, validateRange } from '../history';
  import { router } from '../router.svelte';
  import { attachFloating } from './floating';
  import { clickOutside, uniqueId } from './focus';
  import Button from './Button.svelte';
  import SegmentedControl from './SegmentedControl.svelte';
  import { formatAbsolute } from './relative-time';

  let {
    value = $bindable(),
    defaultKey = '24h',
    /** Mirror the range into the URL (`range`, `from`, `to`). */
    sync = true,
    onchange,
  }: {
    value?: TimeRange;
    defaultKey?: Exclude<RangeKey, 'custom'>;
    sync?: boolean;
    onchange?: (range: TimeRange) => void;
  } = $props();

  const presets: Array<Exclude<RangeKey, 'custom'>> = [
    '1h',
    '6h',
    '24h',
    '7d',
    '30d',
  ];
  let customOpen = $state(false);
  let from = $state('');
  let to = $state('');
  let error = $state('');
  let trigger = $state<HTMLDivElement | null>(null);
  let popover = $state<HTMLDivElement | null>(null);
  let cleanup: (() => void) | null = null;
  const popoverId = uniqueId('range-popover');

  function toLocalInput(date: Date) {
    const offset = date.getTimezoneOffset() * 60_000;
    return new Date(date.getTime() - offset).toISOString().slice(0, 16);
  }

  function fromUrl(): TimeRange | null {
    if (!sync) return null;
    const key = router.param('range');
    if (key === 'custom') {
      const start = new Date(router.param('from'));
      const end = new Date(router.param('to'));
      if (!validateRange(start, end))
        return { key: 'custom', from: start, to: end };
      return null;
    }
    if ((presets as string[]).includes(key)) {
      const preset = key as Exclude<RangeKey, 'custom'>;
      return { key: preset, ...rangeFor(preset) };
    }
    return null;
  }

  function commit(next: TimeRange) {
    value = next;
    onchange?.(next);
    if (sync) {
      router.setQuery({
        range: next.key === defaultKey ? null : next.key,
        from: next.key === 'custom' ? next.from.toISOString() : null,
        to: next.key === 'custom' ? next.to.toISOString() : null,
      });
    }
  }

  function selectPreset(key: Exclude<RangeKey, 'custom'>) {
    customOpen = false;
    commit({ key, ...rangeFor(key) });
  }

  async function openCustom() {
    const current = value ?? { key: defaultKey, ...rangeFor(defaultKey) };
    from = toLocalInput(current.from);
    to = toLocalInput(current.to);
    error = '';
    customOpen = true;
    await tick();
    if (trigger && popover)
      cleanup = attachFloating(trigger, popover, {
        placement: 'bottom-end',
        gap: 8,
      });
    popover?.querySelector<HTMLInputElement>('input')?.focus();
  }

  function closeCustom() {
    customOpen = false;
    cleanup?.();
    cleanup = null;
  }

  function applyCustom() {
    const start = new Date(from);
    const end = new Date(to);
    const problem = validateRange(start, end);
    if (problem) {
      error = problem;
      return;
    }
    closeCustom();
    commit({ key: 'custom', from: start, to: end });
  }

  onMount(() => {
    if (!value) {
      value = fromUrl() ?? { key: defaultKey, ...rangeFor(defaultKey) };
      onchange?.(value);
    }
    return () => cleanup?.();
  });

  /** Refresh preset ranges so "last hour" keeps sliding forward. */
  export function refresh() {
    if (value && value.key !== 'custom')
      commit({ key: value.key, ...rangeFor(value.key) });
  }
</script>

<div class="picker" bind:this={trigger}>
  <SegmentedControl
    label="Time range"
    size="sm"
    value={value?.key === 'custom' ? undefined : value?.key}
    options={presets.map((key) => ({ value: key, label: key }))}
    onchange={selectPreset}
  />
  <button
    type="button"
    class="custom"
    class:active={value?.key === 'custom'}
    aria-haspopup="dialog"
    aria-expanded={customOpen}
    aria-controls={popoverId}
    onclick={() => (customOpen ? closeCustom() : void openCustom())}
  >
    <CalendarRange aria-hidden="true" />
    {#if value?.key === 'custom'}
      <span class="custom-range"
        >{formatAbsolute(value.from)} – {formatAbsolute(value.to)}</span
      >
    {:else}
      Custom
    {/if}
  </button>
  {#if customOpen}
    <div
      id={popoverId}
      class="popover"
      role="dialog"
      aria-label="Custom time range"
      tabindex="-1"
      bind:this={popover}
      use:clickOutside={closeCustom}
      onkeydown={(event) => {
        if (event.key === 'Escape') closeCustom();
      }}
    >
      <label>
        <span>From</span>
        <input type="datetime-local" bind:value={from} required />
      </label>
      <label>
        <span>To</span>
        <input type="datetime-local" bind:value={to} required />
      </label>
      {#if error}<p class="error" role="alert">{error}</p>{/if}
      <div class="actions">
        <Button size="sm" variant="quiet" onclick={closeCustom}>Cancel</Button>
        <Button size="sm" variant="primary" onclick={applyCustom}
          >Apply range</Button
        >
      </div>
    </div>
  {/if}
</div>

<style>
  .picker {
    display: inline-flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
  }
  .custom {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    height: var(--control-h-sm);
    padding: 0 var(--space-2);
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-control);
    background: transparent;
    color: var(--muted);
    font-size: var(--text-meta);
    font-weight: var(--weight-medium);
  }
  .custom:hover,
  .custom.active {
    color: var(--fg);
  }
  .custom.active {
    border-color: var(--accent-border);
    background: var(--accent-bg);
  }
  .custom :global(svg) {
    width: 14px;
    height: 14px;
  }
  .custom-range {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
  }
  .popover {
    z-index: 60;
    display: grid;
    gap: var(--space-3);
    width: min(280px, calc(100vw - 2 * var(--space-4)));
    padding: var(--space-4);
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-card);
    background: var(--panel);
    box-shadow: var(--shadow-overlay);
  }
  label {
    display: grid;
    gap: var(--space-1);
    color: var(--muted);
    font-size: var(--text-meta);
  }
  input {
    height: var(--control-h);
    padding: 0 var(--space-2);
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-control);
    background: var(--bg);
    color: var(--fg);
    font-family: var(--font-mono);
    font-size: var(--text-mono);
  }
  .error {
    color: var(--critical-fg);
    font-size: var(--text-meta);
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
  }
</style>
