<script lang="ts">
  import { missing } from '../i18n';
  import { tick } from 'svelte';
  import ArrowDown from '@lucide/svelte/icons/arrow-down';
  import Copy from '@lucide/svelte/icons/copy';
  import ScrollText from '@lucide/svelte/icons/scroll-text';
  import type { LogEntry } from '../api/logs';
  import type { LogLine } from './ring';
  import Button from '../ui/Button.svelte';
  import EmptyState from '../ui/EmptyState.svelte';
  import { toasts } from '../ui/toast.svelte';
  import { formatClock } from '../ui/relative-time';

  let {
    lines,
    hiddenBefore = 0,
    onshowearlier,
    following = false,
    wrap = true,
    colors = true,
    search = '',
    loading = false,
    componentNames = new Map<string, string>(),
    emptyDescription = 'Load a range, or start following to stream new lines.',
  }: {
    /** The rendered window, oldest first; the ring holds the rest (UI-15). */
    lines: LogLine[];
    /** Lines held in memory before the window. */
    hiddenBefore?: number;
    onshowearlier?: () => void;
    following?: boolean;
    wrap?: boolean;
    colors?: boolean;
    search?: string;
    loading?: boolean;
    componentNames?: Map<string, string>;
    emptyDescription?: string;
  } = $props();

  let scroller = $state<HTMLDivElement | null>(null);
  let pinned = $state(true);
  let unseen = $state(0);
  let lastSeq = -1;

  const palette = [
    'var(--chart-1)',
    'var(--chart-2)',
    'var(--chart-3)',
    'var(--chart-4)',
    'var(--chart-5)',
    'var(--chart-6)',
  ];
  function componentColor(id: string) {
    let hash = 0;
    for (const char of id) hash = (hash * 31 + char.charCodeAt(0)) >>> 0;
    return palette[hash % palette.length];
  }

  function severityClass(severity: string) {
    switch (severity) {
      case 'error':
      case 'critical':
      case 'fatal':
        return 'critical';
      case 'warn':
      case 'warning':
        return 'warn';
      case 'debug':
      case 'trace':
        return 'muted';
      default:
        return '';
    }
  }

  function parts(message: string): Array<{ text: string; hit: boolean }> {
    const needle = search.trim();
    if (!needle) return [{ text: message, hit: false }];
    const out: Array<{ text: string; hit: boolean }> = [];
    let rest = message;
    const lower = needle.toLowerCase();
    while (rest.length) {
      const index = rest.toLowerCase().indexOf(lower);
      if (index < 0) {
        out.push({ text: rest, hit: false });
        break;
      }
      if (index > 0) out.push({ text: rest.slice(0, index), hit: false });
      out.push({ text: rest.slice(index, index + needle.length), hit: true });
      rest = rest.slice(index + needle.length);
    }
    return out;
  }

  function onScroll() {
    if (!scroller) return;
    const distance =
      scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight;
    pinned = distance < 40;
    if (pinned) unseen = 0;
  }

  async function jump() {
    await tick();
    if (!scroller) return;
    scroller.scrollTop = scroller.scrollHeight;
    pinned = true;
    unseen = 0;
  }

  $effect(() => {
    const newest = lines.at(-1)?.seq ?? -1;
    if (newest > lastSeq && following) {
      if (pinned) void jump();
      else if (lastSeq >= 0) unseen += newest - lastSeq;
    }
    lastSeq = newest;
  });

  async function copyLine(entry: LogEntry) {
    try {
      await navigator.clipboard.writeText(
        `${entry.timestamp} ${entry.component} ${entry.stream} ${entry.message}`,
      );
      toasts.success('Line copied');
    } catch {
      toasts.error('Could not copy the line');
    }
  }
</script>

<div class="viewer" class:wrap>
  {#if !lines.length && !loading}
    <EmptyState title="No log lines" description={emptyDescription}>
      {#snippet icon()}<ScrollText />{/snippet}
    </EmptyState>
  {:else}
    <!-- svelte-ignore a11y_no_noninteractive_tabindex (the scroll container must be keyboard-reachable) -->
    <div
      class="scroller"
      bind:this={scroller}
      onscroll={onScroll}
      tabindex="0"
      role="region"
      aria-label="Log lines"
    >
      {#if hiddenBefore > 0 && onshowearlier}
        <div class="earlier">
          <Button size="sm" variant="quiet" onclick={onshowearlier}>
            Show earlier lines ({hiddenBefore} more)
          </Button>
        </div>
      {/if}
      <ol class="lines">
        {#each lines as { seq, entry } (seq)}
          <li
            class={`line ${colors ? severityClass(entry.severity) : ''}`}
            class:stderr={entry.stream === 'stderr'}
          >
            <span class="time num" title={entry.timestamp}
              >{entry.timestamp ? formatClock(entry.timestamp) : missing}</span
            >
            <span
              class="component"
              style:--component-color={componentColor(entry.component)}
              title={entry.component}
              >{componentNames.get(entry.component) ??
                entry.component.slice(0, 12)}</span
            >
            <span class="stream" aria-label={entry.stream}
              >{entry.stream === 'stderr' ? 'E' : ''}</span
            >
            <span class="message">
              {#each parts(entry.message) as part, partIndex (partIndex)}
                {#if part.hit}<mark>{part.text}</mark>{:else}{part.text}{/if}
              {/each}
            </span>
            <button
              type="button"
              class="copy"
              aria-label="Copy line"
              onclick={() => copyLine(entry)}><Copy /></button
            >
          </li>
        {/each}
      </ol>
    </div>
    {#if following && unseen > 0}
      <div class="jump">
        <Button size="sm" variant="primary" onclick={jump}>
          {#snippet icon()}<ArrowDown />{/snippet}
          {unseen} new {unseen === 1 ? 'line' : 'lines'}
        </Button>
      </div>
    {/if}
  {/if}
</div>

<style>
  .viewer {
    position: relative;
    min-height: 320px;
  }
  .scroller {
    max-height: calc(100vh - 300px);
    min-height: 320px;
    overflow: auto;
    background: var(--bg);
    border-radius: 0 0 var(--radius-card) var(--radius-card);
  }
  .scroller:focus-visible {
    outline-offset: -2px;
  }
  .lines {
    margin: 0;
    padding: var(--space-2) 0;
    list-style: none;
    font-family: var(--font-mono);
    font-size: 12px;
    line-height: 1.55;
  }
  .line {
    position: relative;
    display: grid;
    grid-template-columns: 82px 120px 12px minmax(0, 1fr) 28px;
    gap: var(--space-2);
    align-items: start;
    padding: 1px var(--space-3) 1px var(--space-2);
    border-left: 2px solid transparent;
    content-visibility: auto;
    contain-intrinsic-size: auto 22px;
  }
  .line:hover {
    background: var(--panel-2);
  }
  .line.warn {
    border-left-color: var(--warn-solid);
  }
  .line.critical {
    border-left-color: var(--critical-solid);
    background: color-mix(in srgb, var(--critical-bg) 60%, transparent);
  }
  .line.muted .message {
    color: var(--muted);
  }
  .time {
    color: var(--muted);
    white-space: nowrap;
  }
  .component {
    color: var(--component-color);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .stream {
    color: var(--warn-fg);
    font-weight: 700;
    text-align: center;
  }
  .message {
    white-space: pre;
    overflow-wrap: anywhere;
    color: var(--fg);
  }
  .wrap .message {
    white-space: pre-wrap;
  }
  mark {
    padding: 0 1px;
    border-radius: 2px;
    /* Tint plus an underline: the text keeps body contrast (ADR 027 pairs). */
    background: var(--warn-bg);
    box-shadow: inset 0 -2px 0 var(--warn-solid);
    color: var(--fg);
  }
  .copy {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 22px;
    height: 20px;
    border: 0;
    border-radius: 4px;
    background: none;
    color: var(--muted);
    opacity: 0;
  }
  .line:hover .copy,
  .copy:focus-visible {
    opacity: 1;
  }
  .copy :global(svg) {
    width: 12px;
    height: 12px;
  }
  .earlier {
    display: flex;
    justify-content: center;
    padding: var(--space-2) 0 0;
  }
  .jump {
    position: absolute;
    bottom: var(--space-3);
    left: 50%;
    transform: translateX(-50%);
  }
  @media (max-width: 720px) {
    .line {
      grid-template-columns: 70px 12px minmax(0, 1fr) 28px;
    }
    .component {
      display: none;
    }
  }
</style>
