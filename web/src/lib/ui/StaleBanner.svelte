<script lang="ts">
  import type { Snippet } from 'svelte';
  import StatusMarker from './StatusMarker.svelte';
  import { formatAbsolute } from './relative-time';

  let {
    since,
    message = 'Live data paused',
    tone = 'warn',
    children,
  }: {
    /** Time of the last good data; shown as an absolute time. */
    since?: string | number | Date | null;
    /** What is stale, e.g. "Status unknown". */
    message?: string;
    /** `warn` while reconnecting, `critical` once the stream is down. */
    tone?: 'warn' | 'critical';
    /** Actions such as "Reconnect". */
    children?: Snippet;
  } = $props();

  const at = $derived(since == null ? '' : formatAbsolute(since));
</script>

<!-- Not a live region: the shell's connection status announces changes;
     this banner keeps the stale state visible where the data is shown. -->
<div class={`stale ${tone}`} data-tone={tone}>
  <StatusMarker {tone} size={8} />
  <p>
    <strong>{message}</strong>{#if at}<span class="since">
        since <time datetime={new Date(since ?? 0).toISOString()}>{at}</time
        ></span
      >{/if}
  </p>
  {#if children}<div class="actions">{@render children()}</div>{/if}
</div>

<style>
  .stale {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2) var(--space-3);
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--stale-border);
    border-radius: var(--radius-card);
    background: var(--stale-bg);
    color: var(--fg);
    font-size: var(--text-meta);
  }
  .warn {
    --stale-bg: var(--warn-bg);
    --stale-border: var(--warn-border);
  }
  .critical {
    --stale-bg: var(--critical-bg);
    --stale-border: var(--critical-border);
  }
  p {
    flex: 1 1 auto;
    min-width: 0;
  }
  .since {
    color: var(--muted);
  }
  .actions {
    display: flex;
    gap: var(--space-2);
  }
</style>
