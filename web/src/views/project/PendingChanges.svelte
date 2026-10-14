<script lang="ts">
  import { css } from 'styled-system/css'
  import type { Change } from '../../lib/api'
  import { button, linkButton, mono, muted, table } from '../../lib/styles'
  import Icon from '../../components/Icon.svelte'
  import Modal from '../../components/Modal.svelte'

  // What a restart would apply: one line per container, and every change with its values before
  // and after a click away. onrestart: the restart that applies them (none while one can't run).
  let { pending, changes, onrestart }: { pending: string[]; changes: Change[]; onrestart?: () => void } = $props()

  let details = $state(false)

  const box = css({
    display: 'flex', gap: '3', alignItems: 'flex-start', mt: '4', px: '3.5', py: '2.5', borderRadius: 'panel',
    bg: 'warnSubtle', color: 'fg.default', fontSize: 'md', borderWidth: '1px', borderStyle: 'solid', borderColor: 'warn/30',
  })
  const icon = css({ color: 'warn', mt: '1px' })
  const text = css({ flex: 1, minW: 0 })
  const list = css({ m: 0, mt: '1', pl: '4', display: 'grid', gap: '0.5', color: 'fg.muted', listStyle: 'disc' })
  const value = css({ fontFamily: 'mono', fontSize: 'xs', overflowWrap: 'anywhere', minW: '200px', w: '38%' })
  const none = css({ color: 'fg.faint' })
  const notes = css({ pt: '0 !important', fontSize: 'md', '& ul': { m: 0, mt: '1', pl: '4', display: 'grid', gap: '0.5' } })
  const group = css({ '& td': { pt: '3', fontWeight: 'semibold', borderBottom: 'none' } })
</script>

<div class={box}>
  <span class={icon}><Icon name="alert" size={15} /></span>
  <div class={text}>
    <strong>{changes.length || pending.length} pending change{(changes.length || pending.length) === 1 ? '' : 's'}</strong>
    <span class={muted}>· running sessions are untouched until a restart.</span>
    {#if changes.length}<button class={linkButton} onclick={() => (details = true)}>Details</button>{/if}
    <ul class={list}>{#each pending as p}<li>{p}</li>{/each}</ul>
  </div>
  {#if onrestart}
    <button class={button({ size: 'sm' })} onclick={onrestart}><Icon name="restart" size={12} />Restart to apply</button>
  {/if}
</div>

<Modal bind:open={details} wide title="Pending changes">
  <p class={muted} style="font-size:var(--font-sizes-md)">What a restart would change, compared with the containers running now. Secret values never show.</p>
  <div style="overflow-x:auto">
    <table class={table}>
      <thead><tr><th>Change</th><th>Before</th><th>After</th></tr></thead>
      <tbody>
        {#each changes as c, i}
          {#if i === 0 || changes[i - 1].container !== c.container}
            <tr class={group}><td colspan="3">{c.container}</td></tr>
          {/if}
          <tr title={c.detail}>
            <td>{c.what}{#if c.item}&nbsp;<span class={mono}>{c.item}</span>{/if}</td>
            <td class={value}>{#if c.before}{c.before}{:else}<span class={none}>none</span>{/if}</td>
            <td class={value}>{#if c.after}{c.after}{:else}<span class={none}>none</span>{/if}</td>
          </tr>
          {#if c.notes?.length}
            <tr>
              <td colspan="3" class={notes}>
                <span class={muted}>{c.notes_title}</span>
                <ul>{#each c.notes as n}<li>{n}</li>{/each}</ul>
              </td>
            </tr>
          {/if}
        {/each}
      </tbody>
    </table>
  </div>
</Modal>
