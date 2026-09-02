<script lang="ts">
  import Pencil from '@lucide/svelte/icons/pencil';
  import { SvelteMap } from 'svelte/reactivity';
  import { updateRule, type Rule } from '../api/alerts';
  import { errorMessage } from '../api/client';
  import Badge from '../ui/Badge.svelte';
  import IconButton from '../ui/IconButton.svelte';
  import Skeleton from '../ui/Skeleton.svelte';
  import Switch from '../ui/Switch.svelte';
  import { toasts } from '../ui/toast.svelte';
  import { severityLabel, severityTone } from '../ui/status';
  import RuleDialog from './RuleDialog.svelte';
  import { formatSeconds, ruleInfo } from './rule-catalog';

  let {
    rules,
    loading = false,
    onchanged,
  }: {
    rules: Rule[];
    loading?: boolean;
    onchanged?: (rule: Rule) => void;
  } = $props();

  let editing = $state<Rule | null>(null);
  let dialogOpen = $state(false);
  let busy = $state<string | null>(null);

  const groups = $derived.by(() => {
    const order = [
      'Host',
      'Every mount',
      'Every resource',
      'Required checks',
      'Optional checks',
      'Docker collector',
      'Binnacle storage',
      'Custom',
    ];
    const map = new SvelteMap<string, Rule[]>();
    for (const rule of rules) {
      const scope = ruleInfo(rule.family, rule.name).scope;
      map.set(scope, [...(map.get(scope) ?? []), rule]);
    }
    return order
      .filter((key) => map.has(key))
      .map((key) => ({ key, rules: map.get(key)! }));
  });

  async function toggle(rule: Rule, enabled: boolean) {
    busy = rule.id;
    try {
      const saved = await updateRule(rule.id, { enabled });
      toasts.success(enabled ? 'Rule enabled' : 'Rule disabled', {
        description: ruleInfo(rule.family, rule.name).title,
      });
      onchanged?.(saved);
    } catch (reason) {
      toasts.error('Rule could not be updated', {
        description: errorMessage(reason),
      });
    } finally {
      busy = null;
    }
  }

  function thresholdText(rule: Rule) {
    const info = ruleInfo(rule.family, rule.name);
    if (rule.threshold == null) return null;
    if (info.unit === 'events')
      return `${rule.threshold} in ${formatSeconds(rule.windowSeconds)}`;
    const recovery =
      rule.recoveryThreshold != null &&
      rule.recoveryThreshold !== rule.threshold
        ? ` → clears below ${rule.recoveryThreshold}%`
        : '';
    return `above ${rule.threshold}%${recovery}`;
  }
</script>

{#if loading && !rules.length}
  <div class="loading"><Skeleton lines={5} height={40} /></div>
{:else}
  <div class="groups">
    {#each groups as group (group.key)}
      <section
        class="group"
        aria-labelledby={`rules-${group.key.replaceAll(' ', '-')}`}
      >
        <h3 id={`rules-${group.key.replaceAll(' ', '-')}`}>{group.key}</h3>
        <ul>
          {#each group.rules as rule (rule.id)}
            {@const info = ruleInfo(rule.family, rule.name)}
            {@const threshold = thresholdText(rule)}
            <li class:disabled={!rule.enabled}>
              <Switch
                label={`${info.title} enabled`}
                checked={rule.enabled}
                busy={busy === rule.id}
                onchange={(next) => toggle(rule, next)}
              />
              <div class="text">
                <div class="title-row">
                  <span class="title">{info.title}</span>
                  <Badge tone={severityTone(rule.severity)}
                    >{severityLabel(rule.severity)}</Badge
                  >
                  {#if rule.suppressDuringDeployment}<Badge tone="neutral"
                      >paused during deploys</Badge
                    >{/if}
                </div>
                <p class="description">{info.description}</p>
                <p class="conditions num">
                  {#if threshold}<span>{threshold}</span>{/if}
                  {#if rule.triggerSeconds}<span
                      >for {formatSeconds(rule.triggerSeconds)}</span
                    >{/if}
                  {#if rule.recoverySeconds}<span
                      >recovers after {formatSeconds(
                        rule.recoverySeconds,
                      )}</span
                    >{/if}
                </p>
              </div>
              <IconButton
                label={`Edit ${info.title}`}
                size="sm"
                onclick={() => {
                  editing = rule;
                  dialogOpen = true;
                }}><Pencil /></IconButton
              >
            </li>
          {/each}
        </ul>
      </section>
    {/each}
  </div>
{/if}

<RuleDialog
  bind:open={dialogOpen}
  rule={editing}
  onsaved={(saved) => onchanged?.(saved)}
/>

<style>
  .loading {
    padding: var(--space-4);
  }
  .groups {
    display: grid;
  }
  .group + .group {
    border-top: 1px solid var(--border);
  }
  h3 {
    padding: var(--space-3) var(--space-4) var(--space-1);
    color: var(--text-3);
    font-size: var(--text-xs);
    font-weight: 600;
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
  }
  ul {
    margin: 0;
    padding: 0 0 var(--space-2);
    list-style: none;
  }
  li {
    display: flex;
    align-items: flex-start;
    gap: var(--space-4);
    padding: var(--space-3) var(--space-4);
  }
  li.disabled .text {
    opacity: 0.6;
  }
  li > :global(.switch) {
    margin-top: 3px;
  }
  .text {
    display: grid;
    flex: 1;
    gap: 2px;
    min-width: 0;
  }
  .title-row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
  }
  .title {
    font-size: var(--text-sm);
    font-weight: 600;
  }
  .description {
    color: var(--text-2);
    font-size: var(--text-sm);
  }
  .conditions {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
    color: var(--text-3);
    font-size: var(--text-xs);
  }
</style>
