<script lang="ts">
  import type { Snippet } from 'svelte';
  import Toaster from '../ui/Toaster.svelte';
  import BrandMark from './BrandMark.svelte';

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
        <BrandMark size={32} />
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
    gap: var(--space-4);
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
    gap: var(--space-4);
    width: min(100%, 440px);
    padding: var(--space-8) var(--space-6);
    border: 1px solid var(--line);
    border-radius: var(--radius-card);
    background: var(--panel);
    box-shadow: var(--shadow-overlay);
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
    gap: var(--space-2);
    margin-bottom: var(--space-4);
  }
  /* Lockup (DESIGN §Logo): Barlow Condensed 600, gap 0.5x cap height. */
  .wordmark {
    font-family: var(--font-display);
    font-size: 1.5rem;
    font-weight: var(--weight-semibold);
    letter-spacing: var(--tracking-display);
    line-height: 1;
  }
  .eyebrow {
    color: var(--accent-text);
    font-size: var(--text-label);
    font-weight: var(--weight-semibold);
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
  }
  h1 {
    font-family: var(--font-display);
    font-size: var(--text-h1);
    letter-spacing: var(--tracking-display);
  }
  .description {
    color: var(--muted);
    font-size: var(--text-meta);
  }
  .body {
    display: grid;
    gap: var(--space-4);
  }
  .aside {
    padding-top: var(--space-4);
    border-top: 1px solid var(--line);
    color: var(--muted);
    font-size: var(--text-mono);
  }
  .foot {
    position: relative;
    color: var(--muted);
    font-size: var(--text-meta);
  }
  @media (max-width: 560px) {
    .card {
      padding: var(--space-6) var(--space-4);
    }
  }
</style>
