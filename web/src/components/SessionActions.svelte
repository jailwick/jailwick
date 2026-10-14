<script lang="ts">
  import { css } from 'styled-system/css'
  import { api, type FocusResult, type Session } from '../lib/api'
  import { button, mono, muted } from '../lib/styles'
  import Icon from './Icon.svelte'
  import Menu from './Menu.svelte'
  import Modal from './Modal.svelte'
  import { toast } from './Toasts.svelte'

  // What the app can do with a running session, having no terminal of its own: bring forward the
  // terminal it runs in (its own tab where the terminal can be asked for it), or end it. When only
  // the app, or nothing, comes forward, a dialog says so and why. A session in the project's own
  // container (from before each session got a sandbox) has neither.
  let { project, session, onchange }: { project: string; session: Session; onchange?: () => void } = $props()

  let busy = $state(false)
  let info = $state<FocusResult | null>(null)
  let infoOpen = $state(false)
  let ending = $state(false)
  const label = $derived(session.name || (session.worktree ? `the session in ${session.worktree}` : 'the session in the main checkout'))

  async function show(e: MouseEvent) {
    e.stopPropagation() // rows may be links
    busy = true
    try {
      const r = await api.showTerminal(project, session.sid!)
      if (r.found !== 'tab') {
        info = r
        infoOpen = true
      }
    } catch (err) {
      toast((err as Error).message, 'bad')
    } finally {
      busy = false
    }
  }

  async function end() {
    ending = false
    busy = true
    try {
      await api.endSession(project, session.sid!)
      toast(`Ended ${label}: its terminal stays open`, 'ok')
      onchange?.()
    } catch (err) {
      toast((err as Error).message, 'bad')
    } finally {
      busy = false
    }
  }

  const wrap = css({ display: 'inline-flex', gap: '1', alignItems: 'center', whiteSpace: 'nowrap' })
  const body = css({ display: 'grid', gap: '3', fontSize: 'md', lineHeight: 'normal' })
  const foot = css({ display: 'flex', gap: '2', justifyContent: 'flex-end', mt: '2' })
</script>

{#if session.sid}
  <span class={wrap}>
    <button class={button({ size: 'sm' })} disabled={busy}
      title={session.terminal ? `Bring its ${session.terminal} window forward` : 'Bring its terminal forward'} onclick={show}>
      <Icon name="terminal" size={12} />Show terminal
    </button>
    <Menu title="More for {label}" disabled={busy} class={button({ size: 'sm' })}
      items={[{ label: 'End session…', danger: true, run: () => (ending = true) }]} />
  </span>
{/if}

<Modal bind:open={infoOpen} title={info?.found === 'app' ? `${info?.app || 'The terminal'} is in front` : "Couldn't show its terminal"}>
  <div class={body}>
    {#if info?.found === 'app'}<p>jailwick brought {info.app || 'the terminal'} forward, but not the session's own tab.</p>{/if}
    <p class={info?.found === 'app' ? muted : ''}>{info?.note}</p>
    <p class={muted}>Session: <span class={mono}>{label}</span>{#if session.worktree} · worktree <span class={mono}>{session.worktree}</span>{/if}</p>
  </div>
  <div class={foot}><button class={button({ variant: 'primary' })} onclick={() => (infoOpen = false)}>OK</button></div>
</Modal>

<Modal bind:open={ending} title="End {label}?">
  <div class={body}>
    <p>Claude stops in that session, as if you closed its terminal: what it was doing is interrupted. The conversation is kept, to resume later; its terminal stays open.</p>
  </div>
  <div class={foot}>
    <button class={button()} onclick={() => (ending = false)}>Cancel</button>
    <button class={button({ variant: 'danger' })} onclick={end}>End session</button>
  </div>
</Modal>
