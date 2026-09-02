<script lang="ts" module>
  export interface MenuContext {
    close: () => void;
  }
  export const menuContextKey = Symbol('menu');
</script>

<script lang="ts">
  import { onDestroy, setContext, tick, type Snippet } from 'svelte';
  import type { Placement } from '@floating-ui/dom';
  import { attachFloating } from './floating';
  import { clickOutside, uniqueId } from './focus';

  let {
    label,
    placement = 'bottom-end',
    trigger,
    children,
  }: {
    /** Accessible name for the menu. */
    label: string;
    placement?: Placement;
    /** Renders the trigger; spread `props` onto the button. */
    trigger: Snippet<
      [
        {
          'aria-haspopup': 'menu';
          'aria-expanded': boolean;
          'aria-controls': string;
          onclick: (event: MouseEvent) => void;
          onkeydown: (event: KeyboardEvent) => void;
        },
      ]
    >;
    children: Snippet;
  } = $props();

  const id = uniqueId('menu');
  let open = $state(false);
  let wrapper = $state<HTMLDivElement | null>(null);
  let popup = $state<HTMLDivElement | null>(null);
  let cleanup: (() => void) | null = null;
  let opener: HTMLElement | null = null;

  setContext<MenuContext>(menuContextKey, { close });

  function items(): HTMLElement[] {
    return popup
      ? [
          ...popup.querySelectorAll<HTMLElement>(
            '[role="menuitem"]:not([disabled])',
          ),
        ]
      : [];
  }

  async function show(focusIndex = 0) {
    opener = document.activeElement as HTMLElement | null;
    open = true;
    await tick();
    const reference = wrapper?.querySelector<HTMLElement>(
      '[aria-haspopup="menu"]',
    );
    if (reference && popup)
      cleanup = attachFloating(reference, popup, { placement });
    const list = items();
    list.at(focusIndex)?.focus();
  }

  function close(restore = true) {
    if (!open) return;
    open = false;
    cleanup?.();
    cleanup = null;
    if (restore) opener?.focus();
  }

  function toggle() {
    if (open) close();
    else void show();
  }

  function onTriggerKeydown(event: KeyboardEvent) {
    if (
      event.key === 'ArrowDown' ||
      event.key === 'Enter' ||
      event.key === ' '
    ) {
      event.preventDefault();
      void show(0);
    } else if (event.key === 'ArrowUp') {
      event.preventDefault();
      void show(-1);
    }
  }

  function onPopupKeydown(event: KeyboardEvent) {
    const list = items();
    const index = list.indexOf(document.activeElement as HTMLElement);
    switch (event.key) {
      case 'Escape':
        event.preventDefault();
        close();
        break;
      case 'ArrowDown':
        event.preventDefault();
        list[(index + 1) % list.length]?.focus();
        break;
      case 'ArrowUp':
        event.preventDefault();
        list[(index - 1 + list.length) % list.length]?.focus();
        break;
      case 'Home':
        event.preventDefault();
        list[0]?.focus();
        break;
      case 'End':
        event.preventDefault();
        list.at(-1)?.focus();
        break;
      case 'Tab':
        close(false);
        break;
    }
  }

  onDestroy(() => cleanup?.());
</script>

<div class="menu-wrapper" bind:this={wrapper}>
  {@render trigger({
    'aria-haspopup': 'menu',
    'aria-expanded': open,
    'aria-controls': id,
    onclick: toggle,
    onkeydown: onTriggerKeydown,
  })}
  {#if open}
    <div
      {id}
      class="menu"
      role="menu"
      aria-label={label}
      tabindex="-1"
      bind:this={popup}
      onkeydown={onPopupKeydown}
      use:clickOutside={() => close(false)}
    >
      {@render children()}
    </div>
  {/if}
</div>

<style>
  .menu-wrapper {
    display: inline-flex;
  }
  .menu {
    z-index: 60;
    min-width: 180px;
    padding: var(--space-1);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius);
    background: var(--surface);
    box-shadow: var(--shadow-lg);
    animation: menu-in var(--motion-fast) var(--ease-out);
  }
  @keyframes menu-in {
    from {
      opacity: 0;
      translate: 0 -4px;
    }
  }
</style>
