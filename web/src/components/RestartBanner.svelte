<script lang="ts">
  import { css } from 'styled-system/css'
  import { api, ApiError, type ProjectSummary } from '../lib/api'
  import { restarted, restarts } from '../lib/restarts.svelte'
  import { button } from '../lib/styles'
  import ConfirmBar from './ConfirmBar.svelte'
  import Icon from './Icon.svelte'
  import { toast } from './Toasts.svelte'

  // Offers the restarts that apply a change to running projects. A project with sessions running
  // asks first: restarting ends them.
  let { projects }: { projects: ProjectSummary[] } = $props()

  const running = $derived(restarts.projects.filter((n) => projects.some((p) => p.name === n && p.running)))
  let busy = $state<string[]>([])
  let asking = $state<{ names: string[]; message: string } | null>(null)

  async function restart(names: string[], force = false) {
    asking = null
    busy = names
    const ask: string[] = []
    await Promise.all(
      names.map(async (n) => {
        try {
          await api.restart(n, force)
          restarted(n)
          toast(`${n} restarted`, 'ok')
        } catch (e) {
          if (e instanceof ApiError && e.status === 409) ask.push(n)
          else toast(`${n}: ${(e as Error).message}`, 'bad')
        }
      }),
    )
    busy = []
    if (ask.length) asking = { names: ask, message: `${ask.join(', ')}: sessions are running. Restarting ends them.` }
  }

  const box = css({
    display: 'flex', alignItems: 'center', gap: '2.5', flexWrap: 'wrap', mb: '5', px: '3.5', py: '2.5', borderRadius: 'panel',
    bg: 'warnSubtle', fontSize: 'md', borderWidth: '1px', borderStyle: 'solid', borderColor: 'warn/30', '& > svg': { color: 'warn' },
  })
</script>

{#if running.length}
  <div class={box} role="status">
    <Icon name="restart" size={14} />
    <span>Restart to apply the change:</span>
    {#each running as n (n)}
      <button class={button({ size: 'sm' })} disabled={busy.includes(n)} onclick={() => restart([n])}>
        <Icon name={busy.includes(n) ? 'spinner' : 'restart'} size={12} />{n}
      </button>
    {/each}
    {#if running.length > 1}
      <button class={button({ size: 'sm', variant: 'primary' })} disabled={busy.length > 0} onclick={() => restart(running)}>Restart all</button>
    {/if}
  </div>
  {#if asking}
    <ConfirmBar message={asking.message} action="Restart anyway" danger onconfirm={() => restart(asking!.names, true)} oncancel={() => (asking = null)} />
  {/if}
{/if}
