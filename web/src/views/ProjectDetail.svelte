<script lang="ts">
  import { css } from 'styled-system/css'
  import { onDestroy } from 'svelte'
  import { api, ApiError, devBase } from '../lib/api'
  import { poll } from '../lib/poll.svelte'
  import { go } from '../lib/router.svelte'
  import { button, badge, muted, tab as tabC, tabOn } from '../lib/styles'
  import Icon from '../components/Icon.svelte'
  import Dot from '../components/Dot.svelte'
  import ConfirmBar from '../components/ConfirmBar.svelte'
  import { toast } from '../components/Toasts.svelte'
  import { copyText } from '../lib/clipboard'
  import { phaseOf, phaseLabel, underWay } from '../lib/phase'
  import OverviewTab from './project/OverviewTab.svelte'
  import NetworkTab from './project/NetworkTab.svelte'
  import LogsTab from './project/LogsTab.svelte'
  import VerifyTab from './project/VerifyTab.svelte'
  import SessionsTab from './project/SessionsTab.svelte'
  import BuildStatus from './project/BuildStatus.svelte'
  import PendingChanges from './project/PendingChanges.svelte'
  import ServiceProblems from './project/ServiceProblems.svelte'
  import NotesTab from './project/NotesTab.svelte'
  import SettingsTab from './project/SettingsTab.svelte'

  let { name, tab, urls }: { name: string; tab: string; urls: boolean } = $props()

  const detail = poll(() => api.project(name), 3000)
  onDestroy(() => detail.stop())
  const d = $derived(detail.state.data)
  let busy = $state('')
  // A question asked in the page before an action runs (see ConfirmBar).
  let asking = $state<{ message: string; action: string; danger: boolean; run: () => void } | null>(null)

  function ask(message: string, action: string, danger: boolean, run: () => void) {
    asking = { message, action, danger, run }
  }

  function confirmAsked() {
    const run = asking?.run
    asking = null
    run?.()
  }

  async function act(label: 'up' | 'restart' | 'down', fn: () => Promise<unknown>, done: string) {
    asking = null
    busy = label
    try {
      await fn()
      toast(done, 'ok')
    } catch (e) {
      // Running sessions: restarting or stopping ends them, so ask first.
      if (e instanceof ApiError && e.status === 409 && label !== 'up') {
        const restart = label === 'restart'
        ask(`${e.message}\n${restart ? 'Restarting' : 'Stopping'} ends the running sessions.`, restart ? 'Restart anyway' : 'Stop anyway', true,
          () => act(label, () => (restart ? api.restart(name, true) : api.down(name, true)), done))
      } else toast((e as Error).message, 'bad')
    } finally {
      busy = ''
      detail.refresh()
    }
  }

  async function newSession(command: 'claude' | 'shell' = 'claude', open = false) {
    try {
      await api.terminal(name, { command, open })
      toast(`Opened ${command === 'claude' ? (open ? 'a session with the whole internet' : 'a session') : 'a shell'} in your terminal`, 'ok')
    } catch (e) {
      toast((e as Error).message, 'bad')
    }
  }

  const start = () => act('up', () => api.up(name), `${name} is up`)
  const restart = () => act('restart', () => api.restart(name), `${name} restarted`)

  function newOpenSession() {
    ask('Start a session that can reach the whole internet until it ends? Your machine and network stay blocked, ' +
      'and other sessions stay on the allowlist. You can close it from the session list.',
      'Start open session', false, () => newSession('claude', true))
  }

  const tabs = [
    { id: 'overview', label: 'Overview' },
    { id: 'sessions', label: 'Sessions' },
    { id: 'network', label: 'Network' },
    { id: 'logs', label: 'Logs' },
    { id: 'verify', label: 'Verify' },
    { id: 'settings', label: 'Settings' },
    { id: 'notes', label: 'Notes' },
  ]

  const wrap = css({ display: 'flex', flexDir: 'column', minW: 0, minH: 0, h: '100%' })
  const header = css({ px: '6', pt: '5', pb: '0', borderBottom: '1px solid {colors.border.default}', bg: 'bg.canvas' })
  const titleRow = css({ display: 'flex', alignItems: 'center', gap: '3', flexWrap: 'wrap' })
  const title = css({ fontSize: '3xl', fontWeight: 'bold', letterSpacing: 'tight' })
  // The repo's folder, next to the name: a click copies it.
  const pathBtn = css({
    display: 'inline-flex', alignItems: 'center', gap: '1.5', minW: 0, maxW: '48ch', px: '1.5', py: '0.5', ml: '-0.5',
    color: 'fg.muted', fontSize: 'sm', bg: 'transparent', border: 'none', borderRadius: 'md', cursor: 'pointer',
    _hover: { color: 'fg.default', bg: 'bg.subtle' },
  })
  const pathText = css({ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' })
  let copied = $state(false)
  async function copyPath(dir: string) {
    if (await copyText(dir)) {
      copied = true
      toast(`Copied ${dir}`, 'ok')
      setTimeout(() => (copied = false), 1500)
    } else {
      toast("Couldn't copy the path", 'bad')
    }
  }
  const actions = css({ ml: 'auto', display: 'flex', alignItems: 'center', gap: '2', flexWrap: 'wrap' })
  // Restart and Stop end sessions: kept apart from the session buttons.
  const sep = css({ w: '1px', h: 'badge', bg: 'border.strong', mx: '1' })
  const tabsC = css({ display: 'flex', gap: '1', mt: '4' })
  const body = css({ flex: 1, overflowY: 'auto', p: '6', minH: 0 })
  const pending = css({
    display: 'flex', gap: '3', alignItems: 'flex-start', mt: '4', px: '3.5', py: '2.5', borderRadius: 'panel',
    bg: 'warnSubtle', color: 'fg.default', fontSize: 'md', borderWidth: '1px', borderStyle: 'solid', borderColor: 'warn/30',
  })
  const pendingIcon = css({ color: 'warn', mt: '1px' })
  const phasing = css({ display: 'inline-flex', alignItems: 'center', gap: '1.5', h: 'control.md', px: '1', fontSize: 'base', color: 'fg.muted' })
  // A start, restart or stop keeps going when the page is reloaded (the server reports it as op);
  // while one runs the page shows it instead of the buttons, and nothing can be repeated. A paused
  // sandbox offers Resume (a start carries on where it was) and Stop.
  const phase = $derived(phaseOf(d, busy))
  const locked = $derived(!!busy || underWay(phase) || !!d?.op)
  const stop = () => act('down', () => api.down(name), `${name} stopped`)
</script>

<section class={wrap}>
  <header class={header}>
    <div class={titleRow}>
      <Dot on={d?.running} busy={(d?.busy ?? 0) > 0} unknown={!d} {phase} problems={d?.health.problems?.length ?? 0} />
      <h1 class={title}>{name}</h1>
      {#if d?.workspace}<span class={badge()} title="In the workspace {d.workspace}"><Icon name="workspaces" size={11} />{d.workspace}</span>{/if}
      {#if d?.config.claude?.yolo}<span class={badge({ tone: 'warn' })} title="Sessions skip Claude's permission prompts">YOLO</span>{/if}
      {#if d?.config.claude?.chrome}<span class={badge({ tone: 'warn' })} title="Sessions can use your signed-in Chrome, outside the allowlist">Chrome</span>{/if}
      {#if d}
        <button class={pathBtn} title="{d.dir}: click to copy" aria-label="Copy the path {d.dir}" onclick={() => copyPath(d!.dir)}>
          <Icon name={copied ? 'check' : 'folder'} size={13} /><span class={pathText}>{d.dir}</span>
        </button>
      {/if}
      <div class={actions}>
        {#if phase === 'paused'}
          <span class={phasing} role="status"><Icon name="pause" size={14} />{phaseLabel.paused}</span>
          <button class={button({ variant: 'primary' })} disabled={locked} title="Carry on where it was: its sessions too" onclick={start}><Icon name="play" size={13} />Resume</button>
          <button class={button({ variant: 'danger' })} disabled={locked} onclick={stop}><Icon name="stop" size={13} />Stop</button>
        {:else if phase}
          <span class={phasing} role="status"><Icon name="spinner" size={14} />{phaseLabel[phase]}</span>
          <!-- It crashed and the runtime brings it back: it can be stopped meanwhile. -->
          {#if phase === 'recovering'}<button class={button({ variant: 'danger' })} disabled={!!busy || !!d?.op} onclick={stop}><Icon name="stop" size={13} />Stop</button>{/if}
        {:else if d?.running}
          <button class={button({ variant: 'primary' })} disabled={locked} onclick={() => newSession()}><Icon name="terminal" size={14} />New session</button>
          <button class={button()} disabled={locked} title="A new session that can reach the whole internet until it ends (others stay on the allowlist)" onclick={newOpenSession}><Icon name="network" size={14} />Session with internet</button>
          <button class={button()} disabled={locked} title="A shell in the sandbox" onclick={() => newSession('shell')}><Icon name="terminal" size={14} />Shell</button>
          {#if urls && d.url_base}
            <a class={button()} href={devBase(d) + ':3000'} target="_blank" rel="noreferrer" title="The app on port 3000, in your browser"><Icon name="external" size={14} />Open :3000</a>
          {/if}
          <span class={sep} aria-hidden="true"></span>
          <button class={button()} disabled={locked} onclick={restart}>
            <Icon name="restart" size={14} />Restart
          </button>
          <button class={button({ variant: 'danger' })} disabled={locked} onclick={stop}>
            <Icon name="stop" size={13} />Stop
          </button>
        {:else if d}
          <button class={button({ variant: 'primary' })} disabled={locked} onclick={start}>
            <Icon name={d.build?.failed ? 'restart' : 'play'} size={13} />{d.build?.failed ? 'Retry' : 'Start'}
          </button>
        {/if}
      </div>
    </div>
    {#if asking}
      <ConfirmBar message={asking.message} action={asking.action} danger={asking.danger} onconfirm={confirmAsked} oncancel={() => (asking = null)} />
    {/if}
    {#if d?.health.problems?.length}
      <ServiceProblems project={name} problems={d.health.problems} {locked} onstart={start} onchange={() => detail.refresh()} />
    {/if}
    {#if d && d.pending.length}<PendingChanges pending={d.pending} changes={d.changes} onrestart={d.running && !locked ? restart : undefined} />{/if}
    {#if d?.build}<BuildStatus project={name} build={d.build} onretry={locked || !d.running ? undefined : restart} />{/if}
    {#if d?.error}
      <div class={pending}><span class={pendingIcon}><Icon name="alert" size={15} /></span><div>{d.error}</div></div>
    {/if}
    <nav class={tabsC}>
      {#each tabs as t}
        <button class="{tabC} {tab === t.id ? tabOn : ''}" onclick={() => go('projects', name, t.id)}>{t.label}</button>
      {/each}
    </nav>
  </header>
  <div class={body}>
    {#if !d && detail.state.error}
      <p class={muted}>Can't load {name}: {detail.state.error}</p>
    {:else if !d}
      <p class={muted}>Loading…</p>
    {:else if tab === 'network'}
      <NetworkTab {d} {phase} onchange={() => detail.refresh()} />
    {:else if tab === 'logs'}
      <LogsTab {d} />
    {:else if tab === 'verify'}
      <VerifyTab {d} {phase} />
    {:else if tab === 'sessions' || tab === 'history'}
      <SessionsTab {d} {phase} onchange={() => detail.refresh()} />
    {:else if tab === 'settings' || tab === 'toolset'}
      {#key name}<SettingsTab {d} onchange={() => detail.refresh()} />{/key}
    {:else if tab === 'notes'}
      <NotesTab {d} />
    {:else}
      <OverviewTab {d} {phase} {urls} onchange={() => detail.refresh()} />
    {/if}
  </div>
</section>
