<script lang="ts">
  import type { Snippet } from 'svelte';
  import Toaster from '../ui/Toaster.svelte';

  let {
    title,
    description,
    eyebrow = '',
    wide = false,
    children,
    aside,
  }: {
    title: string;
    description?: string;
    eyebrow?: string;
    wide?: boolean;
    children: Snippet;
    aside?: Snippet;
  } = $props();
</script>

<div class="backdrop">
  <div class="glow" aria-hidden="true"></div>
  <div class="card" class:wide>
    <header class="head">
      <span class="brand">
        <img
          class="mark dark"
          src="/brand/binnacle-mark-dark.png"
          alt=""
          width="36"
          height="36"
        />
        <img
          class="mark light"
          src="/brand/binnacle-mark.png"
          alt=""
          width="36"
          height="36"
        />
        <span class="wordmark">Binnacle</span>
      </span>
      {#if eyebrow}<span class="eyebrow">{eyebrow}</span>{/if}
      <h1 id="auth-title">{title}</h1>
      {#if description}<p class="description">{description}</p>{/if}
    </header>
    <div class="body">{@render children()}</div>
    {#if aside}<aside class="aside">{@render aside()}</aside>{/if}
  </div>
  <p class="foot">Self-hosted · local metrics · no telemetry</p>
</div>
<Toaster />

<style>
  .backdrop {
    position: relative;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: var(--space-5);
    min-height: 100vh;
    min-height: 100dvh;
    padding: var(--space-6) var(--space-4);
    background: var(--bg);
    overflow: hidden;
  }
  .glow {
    position: absolute;
    top: -30vh;
    left: 50%;
    width: 80vw;
    max-width: 900px;
    height: 60vh;
    transform: translateX(-50%);
    background: radial-gradient(
      ellipse at center,
      var(--accent-bg) 0%,
      transparent 65%
    );
    pointer-events: none;
  }
  .card {
    position: relative;
    display: grid;
    gap: var(--space-5);
    width: min(100%, 440px);
    padding: var(--space-8) var(--space-6);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    background: var(--surface);
    box-shadow: var(--shadow-lg);
  }
  .card.wide {
    width: min(100%, 720px);
  }
  .head {
    display: grid;
    gap: var(--space-2);
  }
  .brand {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    margin-bottom: var(--space-3);
  }
  .mark.light {
    display: none;
  }
  :global(html[data-theme='light']) .mark.light {
    display: block;
  }
  :global(html[data-theme='light']) .mark.dark {
    display: none;
  }
  .wordmark {
    font-size: var(--text-md);
    font-weight: 700;
    letter-spacing: -0.02em;
  }
  .eyebrow {
    color: var(--accent-text);
    font-size: var(--text-xs);
    font-weight: 600;
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
  }
  h1 {
    font-size: var(--text-2xl);
  }
  .description {
    color: var(--text-2);
    font-size: var(--text-sm);
  }
  .body {
    display: grid;
    gap: var(--space-4);
  }
  .aside {
    padding-top: var(--space-4);
    border-top: 1px solid var(--border);
    color: var(--text-3);
    font-size: var(--text-xs);
  }
  .foot {
    color: var(--text-3);
    font-size: var(--text-xs);
    letter-spacing: var(--tracking-label);
  }
</style>
