<script lang="ts">
  import { css } from 'styled-system/css'
  import { api, type Problem } from '../../lib/api'
  import { goWith } from '../../lib/router.svelte'
  import { button, mono, muted } from '../../lib/styles'
  import Icon from '../../components/Icon.svelte'
  import { toast } from '../../components/Toasts.svelte'

  // A sandbox's containers not doing their work, each with its logs (why) and a way to start it
  // again: alone, so the sessions carry on. One that crashes again is stopped after a few tries.
  // The main container has no start of its own: onstart starts the sandbox.
  let { project, problems, locked, onstart, onchange }:
    { project: string; problems: Problem[]; locked: boolean; onstart: () => void; onchange: () => void } = $props()

  let starting = $state('')
  async function startAgain(p: Problem) {
    if (p.service === 'sandbox' || p.service === 'claude') return onstart()
    starting = p.service
    try {
      await api.startService(project, p.service)
      toast(`${p.service} started again`, 'ok')
    } catch (e) {
      toast((e as Error).message, 'bad')
    } finally {
      starting = ''
      onchange()
    }
  }

  const box = css({
    display: 'grid', gap: '2', mt: '4', px: '3.5', py: '2.5', borderRadius: 'panel', fontSize: 'md',
    bg: 'badSubtle', borderWidth: '1px', borderStyle: 'solid', borderColor: 'bad/30',
  })
  const row = css({ display: 'flex', alignItems: 'center', gap: '2.5', flexWrap: 'wrap' })
  const icon = css({ color: 'bad', display: 'inline-flex' })
  const what = css({ flex: 1, minW: '200px' })
</script>

<div class={box} role="alert">
  {#each problems as p (p.service)}
    <div class={row}>
      <span class={icon}><Icon name="alert" size={15} /></span>
      <span class={what}><strong class={mono}>{p.service}</strong> <span class={muted}>{p.what}</span></span>
      <button class={button({ size: 'sm' })} onclick={() => goWith({ service: p.service }, 'projects', project, 'logs')}>
        <Icon name="book" size={12} />Logs
      </button>
      <button class={button({ size: 'sm' })} disabled={locked || !!starting} onclick={() => startAgain(p)}>
        <Icon name={starting === p.service ? 'spinner' : 'play'} size={12} />{p.state === 'paused' ? 'Resume' : 'Start again'}
      </button>
    </div>
  {/each}
</div>
