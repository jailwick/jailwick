<script lang="ts">
  import { css } from 'styled-system/css'
  import { onDestroy } from 'svelte'
  import { api } from '../lib/api'
  import { poll } from '../lib/poll.svelte'
  import { ago } from '../lib/format'
  import { go } from '../lib/router.svelte'
  import { panel, table, mono, muted, badge } from '../lib/styles'
  import Page from '../lib/Page.svelte'
  import Icon from '../components/Icon.svelte'
  import InternetControl from '../components/InternetControl.svelte'
  import SessionActions from '../components/SessionActions.svelte'
  import Leftovers from '../components/Leftovers.svelte'

  const s = poll(api.sessions, 3000)
  onDestroy(() => s.stop())
  const list = $derived(s.state.data ?? [])
  const where = css({ display: 'inline-flex', alignItems: 'center', gap: '1.5', whiteSpace: 'nowrap', '& svg': { color: 'fg.muted' } })
</script>

<Page title="Sessions" subtitle="Every agent session, newest first. Each has its own proxy: open the internet to one without the others.">
  {#if s.state.loading}
    <p class={muted}>Loading…</p>
  {:else if list.length === 0}
    <section class={panel} style="padding:var(--spacing-7);text-align:center">
      <p style="font-weight:var(--font-weights-semibold);margin-bottom:var(--spacing-1)">No sessions running</p>
      <p class={muted}>Start one from a project, or with <span class={mono}>jw claude</span> in a repo.</p>
    </section>
  {:else}
    <section class={panel}>
      <table class={table}>
        <thead><tr><th>Project</th><th>Where</th><th>Name</th><th>State</th><th>Up</th><th>Internet</th><th></th></tr></thead>
        <tbody>
          {#each list as x (x.project + (x.sid ?? '') + x.pid)}
            <tr style="cursor:pointer" title="Process {x.pid} · click for the project" onclick={(e) => (e.target as Element).closest('button, dialog') || go('projects', x.project)}>
              <td><strong>{x.project}</strong></td>
              <td><span class={where}>{#if x.worktree}<Icon name="branch" size={12} />{x.worktree}{:else}main checkout{/if}</span></td>
              <td class={muted}>{x.name || '–'}</td>
              <td>{#if x.state}<span class={badge({ tone: x.state === 'busy' ? 'info' : 'neutral' })}>{x.state}</span>{:else}–{/if}</td>
              <td style="font-variant-numeric:tabular-nums">{ago(x.started)}</td>
              <td><InternetControl project={x.project} session={x} onchange={() => s.refresh()} /></td>
              <td style="text-align:right"><SessionActions project={x.project} session={x} onchange={() => s.refresh()} /></td>
            </tr>
          {/each}
        </tbody>
      </table>
    </section>
  {/if}
  <Leftovers />
</Page>
