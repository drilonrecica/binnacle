<script lang="ts" module>
  // One shared ticker keeps every relative timestamp in step.
  let now = $state(Date.now());
  let subscribers = 0;
  let timer: number | undefined;
  function subscribe() {
    now = Date.now();
    subscribers += 1;
    if (subscribers === 1) {
      timer = window.setInterval(() => (now = Date.now()), 30_000);
    }
    return () => {
      subscribers -= 1;
      if (subscribers === 0 && timer) {
        window.clearInterval(timer);
        timer = undefined;
      }
    };
  }
</script>

<script lang="ts">
  import { onMount } from 'svelte';
  import { formatAbsolute, formatRelative } from './relative-time';
  import { tooltip } from './tooltip';

  let {
    value,
    prefix = '',
  }: {
    value: string | number | Date | null | undefined;
    prefix?: string;
  } = $props();

  onMount(subscribe);

  const iso = $derived(value == null ? '' : new Date(value).toISOString());
  const relative = $derived(value == null ? '—' : formatRelative(value, now));
  const absolute = $derived(value == null ? '' : formatAbsolute(value));
</script>

{#if value == null}
  <span class="rt muted">—</span>
{:else}
  <time class="rt" datetime={iso} use:tooltip={absolute}
    >{prefix}{relative}</time
  >
{/if}

<style>
  .rt {
    font-family: var(--font-mono);
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }
  .muted {
    color: var(--text-3);
  }
</style>
