<script lang="ts">
  import { css } from 'styled-system/css'
  import { onDestroy } from 'svelte'
  import type { ProjectDetail } from '../../lib/api'
  import { api } from '../../lib/api'
  import { poll } from '../../lib/poll.svelte'
  import { focus, route } from '../../lib/router.svelte'
  import { panel, chip } from '../../lib/styles'

  let { d }: { d: ProjectDetail } = $props()
  let service = $state('')
  // A service to show first (the project page's problems, the menu bar: ?service=postgres).
  $effect(() => {
    if (route.focus.service) service = focus('service')
  })
  const services = $derived(['', ...d.containers.map((c) => c.Service)])
  const logs = poll(() => api.logs(d.name, service, 400), 4000)
  onDestroy(() => logs.stop())
  let pre = $state<HTMLPreElement>()

  $effect(() => {
    service
    logs.refresh()
  })
  $effect(() => {
    logs.state.data
    if (pre) pre.scrollTop = pre.scrollHeight
  })

  const bar = css({ display: 'flex', gap: '1.5', mb: '3', flexWrap: 'wrap' })
  // Lines of the log; the proxy's refusals stand out, so "why was it blocked?" is answered here.
  const lines = $derived((logs.state.data?.text ?? '').split('\n'))
  const denied = (l: string) => /TCP_DENIED|\/403 /.test(l)
  const deniedC = css({ display: 'block', bg: 'badSubtle', color: 'bad', mx: '-4', px: '4' })
  const out = css({
    m: 0, p: '4', h: 'calc(100vh - 290px)', minH: '240px', overflow: 'auto', fontSize: 'xs', lineHeight: 'code',
    whiteSpace: 'pre-wrap', wordBreak: 'break-all', bg: 'bg.inset', color: 'fg.default',
  })
</script>

<div class={bar}>
  {#each services as s}
    <label class={chip}><input type="radio" name="log-service" checked={service === s} onchange={() => (service = s)} />{s || 'all'}</label>
  {/each}
</div>
<div class={panel}>
  <pre class={out} bind:this={pre}>{#if logs.state.data?.text}{#each lines as l, i}{#if denied(l)}<span class={deniedC} title="Refused by the allowlist proxy">{l}</span>{:else}{l}{'\n'}{/if}{/each}{:else}{logs.state.error || 'No output yet.'}{/if}</pre>
</div>
