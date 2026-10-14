<script lang="ts">
  import { css } from 'styled-system/css'
  import { api, type Session } from '../lib/api'
  import { left } from '../lib/format'
  import { faint } from '../lib/styles'
  import Menu, { type MenuItem } from './Menu.svelte'
  import { toast } from './Toasts.svelte'

  // A session's network: on the allowlist, or open to the whole internet (for a while or until
  // closed), with the controls to open, extend and close it.
  let { project, session, onchange }: { project: string; session: Session; onchange: () => void } = $props()

  const choices = [
    { minutes: 15, label: '15 minutes' },
    { minutes: 30, label: '30 minutes' },
    { minutes: 60, label: '1 hour' },
    { minutes: 0, label: 'Until closed' },
  ]

  let busy = $state(false)
  const net = $derived(session.internet)

  async function run(fn: () => Promise<unknown>, done: string) {
    busy = true
    try {
      await fn()
      toast(done, 'ok')
    } catch (e) {
      toast((e as Error).message, 'bad')
    } finally {
      busy = false
      onchange()
    }
  }

  function open(minutes: number) {
    const sid = session.sid!
    const how = minutes ? `for ${choices.find((c) => c.minutes === minutes)?.label}` : 'until closed'
    run(() => api.openInternet(project, sid, minutes), `Session ${sid} can reach the internet ${how}`)
  }

  function close() {
    const sid = session.sid!
    run(() => api.closeInternet(project, sid), `Session ${sid} is back on the allowlist`)
  }

  // One menu: open the internet for a while, or (when open) change how long and close it.
  const items = $derived<MenuItem[]>(
    net?.open
      ? [
          ...choices.map((c) => ({ label: c.minutes ? `Open for ${c.label} from now` : 'Open until closed', run: () => open(c.minutes) })),
          'separator' as const,
          { label: 'Back to the allowlist', danger: true, run: close },
        ]
      : choices.map((c) => ({ label: c.minutes ? `Open the internet for ${c.label}` : 'Open the internet until closed', run: () => open(c.minutes) })),
  )
  const open_ = $derived(!!net?.open)
  const icon = css({ display: 'inline-block', w: 'dot.sm', h: 'dot.sm', borderRadius: 'full', flexShrink: 0 })
  const onC = css({ bg: 'warn' })
  const offC = css({ bg: 'fg.faint' })
</script>

{#if !session.sid || !net}
  <span class={faint}>–</span>
{:else}
  <Menu {items} disabled={busy}
    title={open_ ? 'Can reach every host and port, except your machine and network' : 'Only the allowlist: open the internet to this session alone'}>
    {#snippet label()}
      <span class="{icon} {open_ ? onC : offC}"></span>
      {#if open_}Internet{net.until ? ` · ${left(net.until)} left` : ''}{:else}Allowlist{/if}
    {/snippet}
  </Menu>
{/if}
