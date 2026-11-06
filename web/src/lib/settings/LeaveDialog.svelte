<script lang="ts">
  import { leaveGuard } from '../leave-guard.svelte';
  import { router } from '../router.svelte';
  import ConfirmDialog from '../ui/ConfirmDialog.svelte';

  /**
   * Asks before leaving Settings with unsaved edits (UI-21). Opens whenever
   * the router held a navigation back; staying keeps the edits, leaving
   * discards them.
   */
  function isOpen() {
    return leaveGuard.pending !== null;
  }

  // Closed by Stay, Escape or the backdrop: drop the request, keep the edits.
  function setOpen(next: boolean) {
    if (!next) leaveGuard.settle();
  }

  function leave() {
    const target = leaveGuard.settle();
    if (target) router.navigate(target, { force: true });
  }
</script>

<ConfirmDialog
  bind:open={isOpen, setOpen}
  title="Leave without saving?"
  description="A settings card has changes that are not saved. Leaving discards them."
  confirmLabel="Discard and leave"
  cancelLabel="Stay"
  onconfirm={leave}
/>
