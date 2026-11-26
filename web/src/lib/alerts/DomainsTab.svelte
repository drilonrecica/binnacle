<script lang="ts">
  import EllipsisVertical from '@lucide/svelte/icons/ellipsis-vertical';
  import Globe from '@lucide/svelte/icons/globe';
  import Pause from '@lucide/svelte/icons/pause';
  import Play from '@lucide/svelte/icons/play';
  import Plus from '@lucide/svelte/icons/plus';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import { failureLabel } from '../api/checks';
  import { errorMessage } from '../api/client';
  import {
    certExpiringDays,
    certLabel,
    createDomain,
    deleteDomain,
    domainStatus,
    httpLabel,
    runDomain,
    unreachable,
    updateDomain,
    type SweptDomain,
  } from '../api/sweep';
  import { missing } from '../i18n';
  import Button from '../ui/Button.svelte';
  import ConfirmDialog from '../ui/ConfirmDialog.svelte';
  import DataTable, { type Column } from '../ui/DataTable.svelte';
  import EmptyState from '../ui/EmptyState.svelte';
  import IconButton from '../ui/IconButton.svelte';
  import Input from '../ui/Input.svelte';
  import Menu from '../ui/Menu.svelte';
  import MenuItem from '../ui/MenuItem.svelte';
  import MenuSeparator from '../ui/MenuSeparator.svelte';
  import RelativeTime from '../ui/RelativeTime.svelte';
  import Status from '../ui/Status.svelte';
  import { toasts } from '../ui/toast.svelte';
  import { tooltip } from '../ui/tooltip';

  /**
   * Settings › Alerting › Domains: the public domains the sweep probes for
   * DNS, HTTPS, certificate expiry, SPF/DMARC and security headers
   * (ADR 028). Adding and removing happen inline; there is nothing else to
   * configure per domain but its interval, which the API defaults.
   */
  let {
    domains,
    loading = false,
    onchanged,
  }: {
    domains: SweptDomain[];
    loading?: boolean;
    onchanged?: () => void;
  } = $props();

  let newDomain = $state('');
  let adding = $state(false);
  let addError = $state('');
  let busy = $state<string | null>(null);
  let removing = $state<SweptDomain | null>(null);
  let confirmOpen = $state(false);

  async function add(event: SubmitEvent) {
    event.preventDefault();
    const domain = newDomain.trim();
    if (!domain) return;
    adding = true;
    addError = '';
    try {
      const created = await createDomain({ domain });
      toasts.success('Domain added', { description: created.domain });
      newDomain = '';
      onchanged?.();
    } catch (reason) {
      addError = errorMessage(reason);
    } finally {
      adding = false;
    }
  }

  async function run(domain: SweptDomain) {
    busy = domain.id;
    try {
      const result = await runDomain(domain.id);
      if (unreachable(result))
        toasts.warning(`${domain.domain} is unreachable`, {
          description: result.httpStatus
            ? `HTTP ${result.httpStatus}`
            : failureLabel(result.failureCode),
        });
      else
        toasts.success(`${domain.domain} swept`, {
          description: `HTTP ${result.httpStatus} · certificate ${certLabel(result.certDaysLeft)}`,
        });
      onchanged?.();
    } catch (reason) {
      toasts.error('Domain could not be swept', {
        description: errorMessage(reason),
      });
    } finally {
      busy = null;
    }
  }

  async function toggle(domain: SweptDomain) {
    busy = domain.id;
    try {
      await updateDomain(domain.id, { enabled: !domain.enabled });
      toasts.success(domain.enabled ? 'Domain paused' : 'Domain resumed', {
        description: domain.domain,
      });
      onchanged?.();
    } catch (reason) {
      toasts.error('Domain could not be updated', {
        description: errorMessage(reason),
      });
    } finally {
      busy = null;
    }
  }

  async function remove() {
    if (!removing) return;
    await deleteDomain(removing.id);
    toasts.success('Domain removed', { description: removing.domain });
    removing = null;
    onchanged?.();
  }

  const rank: Record<string, number> = {
    Unreachable: 0,
    'DNS missing': 1,
    'Certificate expiring': 2,
    'Mail auth missing': 3,
    'Headers missing': 4,
    'Not swept yet': 5,
    Healthy: 6,
    Disabled: 7,
  };

  const columns: Column<SweptDomain>[] = [
    {
      key: 'domain',
      label: 'Domain',
      sortable: true,
      sortValue: (row) => row.domain,
      cell: domainCell,
      card: 'title',
    },
    {
      key: 'status',
      label: 'Status',
      width: '172px',
      sortable: true,
      sortValue: (row) => rank[domainStatus(row).label] ?? 5,
      cell: statusCell,
    },
    {
      key: 'cert',
      label: 'Cert',
      align: 'right',
      width: '64px',
      sortable: true,
      sortValue: (row) => row.state?.certDaysLeft ?? Number.MAX_SAFE_INTEGER,
      cell: certCell,
    },
    { key: 'spf', label: 'SPF', align: 'center', width: '52px', cell: spfCell },
    {
      key: 'dmarc',
      label: 'DMARC',
      align: 'center',
      width: '84px',
      cell: dmarcCell,
    },
    {
      key: 'hsts',
      label: 'HSTS',
      align: 'center',
      width: '56px',
      cell: hstsCell,
    },
    { key: 'csp', label: 'CSP', align: 'center', width: '52px', cell: cspCell },
    {
      key: 'checked',
      label: 'Checked',
      align: 'right',
      width: '84px',
      sortable: true,
      sortValue: (row) =>
        row.state ? Date.parse(row.state.checkedAt) : Number.MIN_SAFE_INTEGER,
      cell: checkedCell,
    },
    {
      key: 'actions',
      label: 'Actions',
      srOnly: true,
      align: 'right',
      width: '48px',
      cell: actionsCell,
      card: 'hide',
    },
  ];
</script>

{#snippet mark(present: boolean | undefined, name: string)}
  {#if present == null}
    <span class="muted">{missing}</span>
  {:else if present}
    <span class="mark ok" role="img" aria-label={`${name} present`}>✓</span>
  {:else}
    <span class="mark bad" role="img" aria-label={`${name} missing`}>✗</span>
  {/if}
{/snippet}
{#snippet domainCell(row: SweptDomain)}
  {@const issuer = row.state?.certIssuer?.replace(/^CN=([^,]+).*$/, '$1')}
  <div class="name">
    <span class="title mono">{row.domain}</span>
    {#if row.state?.failureCode}
      <span class="failure">{failureLabel(row.state.failureCode)}</span>
    {:else if row.state}
      <span class="meta" use:tooltip={row.state.certIssuer ?? ''}
        ><span class:bad={unreachable(row.state)}
          >HTTP {httpLabel(row.state)}</span
        >{#if issuer}&nbsp;· {issuer}{/if}</span
      >
    {/if}
  </div>
{/snippet}
{#snippet statusCell(row: SweptDomain)}
  {@const state = domainStatus(row)}
  <Status tone={state.tone} label={state.label} />
{/snippet}
{#snippet certCell(row: SweptDomain)}
  {@const days = row.state?.certDaysLeft}
  <span
    class="num"
    class:bad={days != null && days <= certExpiringDays}
    use:tooltip={row.state?.certNotAfter
      ? `Expires ${new Date(row.state.certNotAfter).toLocaleDateString()}`
      : ''}>{certLabel(days)}</span
  >
{/snippet}
{#snippet spfCell(row: SweptDomain)}
  <span use:tooltip={row.state?.spf ?? ''}
    >{@render mark(row.state ? Boolean(row.state.spf) : undefined, 'SPF')}</span
  >
{/snippet}
{#snippet dmarcCell(row: SweptDomain)}
  {#if !row.state}
    <span class="muted">{missing}</span>
  {:else if row.state.dmarcPolicy}
    <span
      class="mono"
      class:bad={row.state.dmarcPolicy === 'none'}
      class:ok={row.state.dmarcPolicy !== 'none'}>{row.state.dmarcPolicy}</span
    >
  {:else}
    {@render mark(false, 'DMARC')}
  {/if}
{/snippet}
{#snippet hstsCell(row: SweptDomain)}
  {@render mark(
    row.state && row.state.httpStatus ? row.state.hsts : undefined,
    'Strict-Transport-Security',
  )}
{/snippet}
{#snippet cspCell(row: SweptDomain)}
  {@render mark(
    row.state && row.state.httpStatus ? row.state.csp : undefined,
    'Content-Security-Policy',
  )}
{/snippet}
{#snippet checkedCell(row: SweptDomain)}
  <RelativeTime value={row.state?.checkedAt} />
{/snippet}
{#snippet actionsCell(row: SweptDomain)}
  <Menu label={`Actions for ${row.domain}`}>
    {#snippet trigger(props)}
      <IconButton label="Domain actions" size="sm" {...props}
        ><EllipsisVertical /></IconButton
      >
    {/snippet}
    <MenuItem
      disabled={!row.enabled || busy === row.id}
      onselect={() => run(row)}
      >{#snippet icon()}<Play />{/snippet}Run now</MenuItem
    >
    <MenuItem disabled={busy === row.id} onselect={() => toggle(row)}
      >{#snippet icon()}{#if row.enabled}<Pause />{:else}<Play
          />{/if}{/snippet}{row.enabled ? 'Pause' : 'Resume'}</MenuItem
    >
    <MenuSeparator />
    <MenuItem
      danger
      onselect={() => {
        removing = row;
        confirmOpen = true;
      }}>{#snippet icon()}<Trash2 />{/snippet}Remove…</MenuItem
    >
  </Menu>
{/snippet}
{#snippet empty()}
  <EmptyState
    title="No domains swept yet"
    description="Add the public domains you own. Each is probed hourly by default for DNS, HTTPS, certificate expiry, SPF and DMARC, and security headers."
  >
    {#snippet icon()}<Globe />{/snippet}
  </EmptyState>
{/snippet}

<form class="toolbar" onsubmit={add}>
  <p class="hint">
    Public domains probed from this server, hourly by default. Alerts fire when
    a domain is unreachable, its certificate expires within {certExpiringDays} days,
    or DNS, mail authentication or security headers are missing.
  </p>
  <div class="add">
    <Input
      bind:value={newDomain}
      placeholder="example.com"
      aria-label="Domain to add"
      mono
      autocomplete="off"
      spellcheck={false}
      invalid={Boolean(addError)}
      disabled={adding}
    />
    <Button
      type="submit"
      size="sm"
      variant="primary"
      loading={adding}
      disabled={!newDomain.trim()}
    >
      {#snippet icon()}<Plus />{/snippet}
      Add domain
    </Button>
  </div>
  {#if addError}
    <p class="error" role="alert">{addError}</p>
  {/if}
</form>
<DataTable
  rows={domains}
  {columns}
  rowKey={(row) => row.id}
  caption="Swept domains"
  {loading}
  {empty}
/>

<ConfirmDialog
  bind:open={confirmOpen}
  title="Remove this domain?"
  description={removing
    ? `${removing.domain} will no longer be swept. Its change history stays.`
    : ''}
  confirmLabel="Remove domain"
  onconfirm={remove}
/>

<style>
  .toolbar {
    display: grid;
    grid-template-columns: 1fr auto;
    align-items: center;
    gap: var(--space-2) var(--space-4);
    padding: var(--space-3) var(--space-4);
    border-bottom: 1px solid var(--line);
  }
  .hint {
    color: var(--muted);
    font-size: var(--text-meta);
  }
  .add {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-width: 280px;
  }
  .error {
    grid-column: 1 / -1;
    color: var(--critical-fg);
    font-size: var(--text-meta);
  }
  @media (max-width: 720px) {
    .toolbar {
      grid-template-columns: 1fr;
    }
    .add {
      min-width: 0;
    }
  }
  .name {
    display: grid;
    gap: 2px;
    white-space: normal;
  }
  .title {
    font-weight: var(--weight-semibold);
  }
  .meta {
    color: var(--muted);
    font-size: var(--text-meta);
  }
  .failure {
    color: var(--critical-fg);
    font-size: var(--text-meta);
  }
  .mono {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
  }
  .muted {
    color: var(--muted);
  }
  .mark {
    font-weight: var(--weight-semibold);
  }
  .ok {
    color: var(--ok-fg);
  }
  .bad {
    color: var(--critical-fg);
  }
</style>
