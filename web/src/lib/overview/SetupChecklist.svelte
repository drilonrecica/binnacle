<script lang="ts">
  import { onMount } from 'svelte';
  import CircleCheck from '@lucide/svelte/icons/circle-check';
  import CircleAlert from '@lucide/svelte/icons/circle-alert';
  import Circle from '@lucide/svelte/icons/circle';
  import {
    dismissChecklist,
    onboardingState,
    type OnboardingState,
  } from '../onboarding';
  import { errorMessage } from '../api/client';
  import Button from '../ui/Button.svelte';
  import Card from '../ui/Card.svelte';
  import { toasts } from '../ui/toast.svelte';

  let onboarding = $state<OnboardingState | null>(null);
  let dismissed = $state(false);

  const visible = $derived(
    Boolean(onboarding?.completedAt) &&
      !onboarding?.checklistDismissed &&
      !dismissed,
  );

  function diagnostic(id: string) {
    return onboarding?.diagnostics?.find((item) => item.id === id);
  }

  const items = $derived.by(() => {
    const docker = diagnostic('docker_api');
    const metadata = diagnostic('deployment_metadata');
    return [
      {
        id: 'docker',
        label: 'Docker monitoring',
        status: docker?.status === 'passed' ? 'done' : 'attention',
        detail:
          docker?.status === 'passed'
            ? 'Containers are being observed.'
            : (docker?.suggestedFix ??
              'Mount the Docker socket or configure a socket proxy.'),
        href: '/host?tab=collectors',
      },
      {
        id: 'metadata',
        label: 'Compose or Coolify grouping',
        status: metadata?.status === 'passed' ? 'done' : 'optional',
        detail:
          metadata?.status === 'passed'
            ? 'Resources are grouped by project.'
            : 'Connect Coolify to enrich names and grouping.',
        href: '/settings/integrations',
      },
      {
        id: 'channel',
        label: 'Add a notification channel',
        status: 'optional',
        detail: 'Get incidents by webhook or email.',
        href: '/settings/notifications',
      },
      {
        id: 'check',
        label: 'Create a health check',
        status: 'optional',
        detail: 'Probe an HTTP endpoint on a schedule.',
        href: '/alerts?tab=checks',
      },
    ] as const;
  });

  onMount(() => {
    void onboardingState()
      .then((value) => (onboarding = value))
      .catch(() => {
        /* The checklist is optional guidance. */
      });
  });

  async function dismiss() {
    dismissed = true;
    try {
      await dismissChecklist();
    } catch (reason) {
      dismissed = false;
      toasts.error('Could not dismiss the checklist', {
        description: errorMessage(reason),
      });
    }
  }
</script>

{#if visible}
  <Card title="Finish setting up" id="checklist-title" padded={false}>
    {#snippet actions()}
      <Button size="sm" variant="ghost" onclick={dismiss}>Dismiss</Button>
    {/snippet}
    <ul class="list">
      {#each items as item (item.id)}
        <li class={item.status}>
          <span class="icon" aria-hidden="true">
            {#if item.status === 'done'}<CircleCheck
              />{:else if item.status === 'attention'}<CircleAlert
              />{:else}<Circle />{/if}
          </span>
          <div class="text">
            <a href={item.href}>{item.label}</a>
            <span class="detail">{item.detail}</span>
          </div>
          <span class="sr-only"
            >{item.status === 'done'
              ? 'done'
              : item.status === 'attention'
                ? 'needs attention'
                : 'optional'}</span
          >
        </li>
      {/each}
    </ul>
  </Card>
{/if}

<style>
  .list {
    margin: 0;
    padding: var(--space-2) 0;
    list-style: none;
  }
  li {
    display: flex;
    gap: var(--space-3);
    padding: var(--space-2) var(--space-4);
  }
  .icon {
    display: inline-flex;
    margin-top: 2px;
    color: var(--text-3);
  }
  .icon :global(svg) {
    width: 16px;
    height: 16px;
  }
  li.done .icon {
    color: var(--ok-fg);
  }
  li.attention .icon {
    color: var(--warn-fg);
  }
  .text {
    display: grid;
    gap: 1px;
    font-size: var(--text-sm);
  }
  .text a {
    font-weight: 500;
  }
  li.done .text a {
    color: var(--text-2);
    text-decoration: line-through;
  }
  .detail {
    color: var(--text-3);
    font-size: var(--text-xs);
  }
</style>
