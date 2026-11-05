<script lang="ts">
  let {
    checked = $bindable(false),
    disabled = false,
    busy = false,
    id,
    label,
    onchange,
  }: {
    checked?: boolean;
    disabled?: boolean;
    busy?: boolean;
    id?: string;
    /** Accessible name when no visible label references the switch. */
    label?: string;
    onchange?: (checked: boolean) => void;
  } = $props();

  function toggle() {
    if (disabled || busy) return;
    checked = !checked;
    onchange?.(checked);
  }
</script>

<button
  type="button"
  role="switch"
  {id}
  class="switch"
  class:busy
  aria-checked={checked}
  aria-label={label}
  aria-busy={busy ? 'true' : undefined}
  {disabled}
  onclick={toggle}
>
  <span class="thumb"></span>
</button>

<style>
  .switch {
    position: relative;
    flex: none;
    width: 36px;
    height: 20px;
    padding: 0;
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-control);
    background: var(--line);
    transition:
      background var(--motion) var(--ease-out),
      border-color var(--motion) var(--ease-out);
  }
  .switch[aria-checked='true'] {
    border-color: var(--accent);
    background: var(--accent);
  }
  .thumb {
    position: absolute;
    top: 2px;
    left: 2px;
    width: 14px;
    height: 14px;
    border-radius: 2px;
    background: var(--fg);
    transition: transform var(--motion) var(--ease-out);
  }
  .switch[aria-checked='true'] .thumb {
    background: var(--accent-contrast);
    transform: translateX(16px);
  }
  .switch:disabled {
    opacity: 0.5;
  }
  .busy {
    opacity: 0.7;
  }
</style>
