<script lang="ts">
  import { css } from 'styled-system/css'
  import { onDestroy } from 'svelte'
  import { api, type Leftover } from '../lib/api'
  import { poll } from '../lib/poll.svelte'
  import { ago } from '../lib/format'
  import { badge, button, mono, muted, panel, panelHeader, table } from '../lib/styles'
  import Icon from './Icon.svelte'
  import { toast } from './Toasts.svelte'

  // Sandboxes whose project file is gone: no project, so their sessions are in no other list. They
  // shouldn't happen (jw project rm stops the sandbox first); when one does, it can be stopped here.
  const l = poll(api.leftovers, 5000)
  onDestroy(() => l.stop())
  const list = $derived(l.state.data ?? []) // no runtime: nothing to show

  let confirming = $state('')
  let withData = $state(false)
  let removing = $state('')

  function ask(name: string) {
    confirming = name
    withData = false
  }

  async function remove(x: Leftover) {
    removing = x.name
    try {
      await api.removeLeftover(x.name, withData)
      toast(`Removed the ${x.name} sandbox${withData ? ' and its data' : ''}`, 'ok')
      confirming = ''
    } catch (e) {
      toast((e as Error).message, 'bad')
    } finally {
      removing = ''
      l.refresh()
    }
  }

  const wrap = css({ mt: '5' })
  const head = css({ color: 'warn' })
  const intro = css({ px: '4', py: '3', fontSize: 'md', borderBottom: '1px solid {colors.border.default}' })
  const item = css({ px: '4', py: '3', display: 'grid', gap: '2', '& + &': { borderTop: '1px solid {colors.border.default}' } })
  const row = css({ display: 'flex', alignItems: 'center', gap: '3', flexWrap: 'wrap' })
  const confirmBox = css({
    display: 'flex', alignItems: 'center', gap: '3', flexWrap: 'wrap', px: '3', py: '2', borderRadius: 'control',
    bg: 'warnSubtle', fontSize: 'md',
  })
</script>

{#if list.length}
  <section class="{panel} {wrap}">
    <div class="{panelHeader} {head}"><Icon name="alert" size={14} />Sandboxes without a project</div>
    <p class={intro}>
      Their project files are gone, so they belong to no project and their sessions aren't listed above. Stop them
      if you don't need them; their data is kept unless you choose to delete it.
    </p>
    {#each list as x (x.name)}
      <div class={item}>
        <div class={row}>
          <strong class={mono}>{x.name}</strong>
          <span class={muted}>since {new Date(x.created).toLocaleString()}</span>
          <span class={muted}>· {x.containers.filter((c) => c.state === 'running').length} of {x.containers.length} containers running</span>
          <span style="margin-left:auto">
            {#if confirming !== x.name}
              <button class={button({ size: 'sm', variant: 'danger' })} onclick={() => ask(x.name)}><Icon name="stop" size={12} />Stop</button>
            {/if}
          </span>
        </div>
        {#if x.sessions.length}
          <table class={table}>
            <thead><tr><th>Where</th><th>State</th><th>Up</th><th>Name</th><th>PID</th></tr></thead>
            <tbody>
              {#each x.sessions as s ((s.sid ?? '') + s.pid)}
                <tr>
                  <td>{#if s.worktree}<Icon name="branch" size={12} /> {s.worktree}{:else}main checkout{/if}</td>
                  <td>{#if s.state}<span class={badge()}>{s.state}</span>{:else}–{/if}</td>
                  <td class={mono}>{ago(s.started)}</td>
                  <td class={muted}>{s.name ?? ''}</td>
                  <td class="{mono} {muted}">{s.pid}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        {/if}
        {#if confirming === x.name}
          <div class={confirmBox} role="group" aria-label="Confirm stopping {x.name}">
            <span>Stop and remove the <strong>{x.name}</strong> sandbox{x.sessions.length ? `, ending ${x.sessions.length} session${x.sessions.length === 1 ? '' : 's'}` : ''}?</span>
            <label style="display:inline-flex;gap:var(--spacing-1\.5);align-items:center">
              <input type="checkbox" bind:checked={withData} />Also delete its data (databases, history, its own Claude login)
            </label>
            <span style="margin-left:auto;display:flex;gap:var(--spacing-2)">
              <button class={button({ size: 'sm' })} onclick={() => (confirming = '')}>Cancel</button>
              <button class={button({ size: 'sm', variant: 'danger' })} disabled={removing === x.name} onclick={() => remove(x)}>
                {removing === x.name ? 'Removing…' : 'Stop and remove'}
              </button>
            </span>
          </div>
        {/if}
      </div>
    {/each}
  </section>
{/if}
