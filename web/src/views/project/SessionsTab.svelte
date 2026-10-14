<script lang="ts">
  import { css } from 'styled-system/css'
  import { api, type Conversation, type ProjectDetail, type SessionRequest } from '../../lib/api'
  import { ago, when } from '../../lib/format'
  import { tick } from 'svelte'
  import { focus, go } from '../../lib/router.svelte'
  import { badge, button, linkButton, mono, muted, panel, panelHeader, pointedOut, table, panelCount as count } from '../../lib/styles'
  import CopyCommand from '../../components/CopyCommand.svelte'
  import Icon from '../../components/Icon.svelte'
  import InternetControl from '../../components/InternetControl.svelte'
  import SessionActions from '../../components/SessionActions.svelte'
  import { toast } from '../../components/Toasts.svelte'
  import PhaseNote from '../../components/PhaseNote.svelte'
  import type { Phase } from '../../lib/phase'

  // A project's sessions: how to start one, the ones running now, and past conversations to
  // resume. Each can open in the terminal set in Settings, or be copied for any other terminal.
  let { d, phase = '', onchange }: { d: ProjectDetail; phase?: Phase; onchange: () => void } = $props()

  // d is replaced at every refresh of the page: depend on what changes, not on d itself.
  const name = $derived(d.name)
  const running = $derived(d.running)
  // A start, restart or stop under way: its containers come and go, nothing opens meanwhile.
  const ready = $derived(running && !phase)
  // The conversations sessions hold now, by id (a running session reports its conversation's id):
  // those show where they run instead of Resume, which would start a second Claude on them.
  const held = $derived(new Map(d.session_list.filter((s) => s.id).map((s) => [s.id!, s])))

  let convs = $state<Conversation[] | null>(null)
  let error = $state('')
  let loading = $state(false)
  async function load() {
    loading = true
    error = ''
    try {
      convs = (await api.history(name)) ?? []
    } catch (e) {
      error = (e as Error).message
    } finally {
      loading = false
    }
  }
  $effect(() => {
    if (running) void load()
  })

  // The session a notification was about ("Claude is waiting for you", ?session=<sid>:<pid>): marked
  // and scrolled to.
  const key = (s: { sid?: string; pid: number }) => `${s.sid ?? ''}:${s.pid}`
  let marked = $state('')
  $effect(() => {
    const k = focus('session')
    if (!k) return
    marked = k
    void tick().then(() => document.querySelector(`[data-session="${CSS.escape(k)}"]`)?.scrollIntoView({ block: 'center', behavior: 'smooth' }))
  })

  // Sessions of a project can message each other; a private one is kept out of that.
  let alone = $state(false)
  const claudeCommand = $derived(d.commands.claude + (alone ? ' --private' : ''))

  async function open(s: SessionRequest, what: string) {
    try {
      await api.terminal(name, s)
      toast(`Opened ${what} in your terminal`, 'ok')
    } catch (e) {
      toast((e as Error).message, 'bad')
    }
  }

  const stack = css({ display: 'grid', gap: '5' })
  const starts = css({ display: 'grid', gap: '3', px: '4', py: '3' })
  const start = css({ display: 'grid', gridTemplateColumns: { base: '1fr', md: '190px auto 1fr' }, gap: '3', alignItems: 'center' })
  const emptyRow = css({ px: '4', py: '4', color: 'fg.muted' })
  const hint = css({ px: '4', pb: '3', fontSize: 'sm' })
  const prompt = css({ display: 'grid', gap: '1.5', minW: 0, maxW: 'content.prose' })
  const convName = css({ fontWeight: 'bold', wordBreak: 'break-word' })
  const folder = css({ display: 'flex', alignItems: 'center', gap: '1.5', minW: 0, color: 'fg.muted', fontSize: 'sm', '& > svg': { flexShrink: 0 } })
  const folderBtn = css({ h: 'control.xs!', w: 'control.xs!' })
  const folderPath = css({ fontFamily: 'mono', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' })
  const option = css({ display: 'inline-flex', gap: '2', alignItems: 'center', fontSize: 'base' })
  const runningHere = css({ display: 'inline-flex', alignItems: 'center', gap: '1.5', fontSize: 'sm', color: 'fg.muted', whiteSpace: 'nowrap' })
  const actions = css({ textAlign: 'right', whiteSpace: 'nowrap', verticalAlign: 'top' })
  const where = css({ display: 'inline-flex', alignItems: 'center', gap: '1.5', whiteSpace: 'nowrap', '& svg': { color: 'fg.muted' } })
  const date = css({ whiteSpace: 'nowrap', color: 'fg.muted', fontVariantNumeric: 'tabular-nums' })
</script>

<div class={stack}>
  <section class={panel}>
    <div class={panelHeader}><Icon name="terminal" size={14} />Start a session</div>
    <div class={starts}>
      <div class={start}>
        <strong>Claude, main checkout</strong>
        <button class={button({ size: 'sm', variant: 'primary' })} disabled={!ready} onclick={() => open({ private: alone }, 'a session')}>
          <Icon name="terminal" size={12} />Open
        </button>
        <CopyCommand command={claudeCommand} />
      </div>
      <label class={option} title="The project's sessions can message each other (Claude Code's SendMessage). A private session can't see or message them, nor they it.">
        <input type="checkbox" bind:checked={alone} />Private: no messages to or from the project's other sessions
      </label>
      <div class={start}>
        <strong>A shell in the sandbox</strong>
        <button class={button({ size: 'sm' })} disabled={!ready} onclick={() => open({ command: 'shell' }, 'a shell')}>
          <Icon name="terminal" size={12} />Open
        </button>
        <CopyCommand command={d.commands.shell} />
      </div>
    </div>
    <p class="{hint} {muted}">
      Open uses your terminal from <button class={linkButton} onclick={() => go('settings')}>Settings</button>. In any other,
      run the command: it starts the sandbox if needed.
    </p>
  </section>

  <section class={panel}>
    <div class={panelHeader}><Icon name="sessions" size={14} />Running now<span class={count}>{d.session_list.length}</span></div>
    {#if phase}
      <div class={emptyRow}><PhaseNote {phase} /></div>
    {:else if !running}
      <div class={emptyRow}>Not running. Start the sandbox, or run a command above: it starts it.</div>
    {:else if d.session_list.length === 0}
      <div class={emptyRow}>No sessions running.</div>
    {:else}
      <table class={table}>
        <thead><tr><th>Where</th><th>Name</th><th>State</th><th>Up</th><th>Internet</th><th></th></tr></thead>
        <tbody>
          {#each d.session_list as s ((s.sid ?? '') + s.pid)}
            <tr class={marked === key(s) ? pointedOut : ''} data-session={key(s)}>
              <td>
                <span class={where}>{#if s.worktree}<Icon name="branch" size={12} />{s.worktree}{:else}main checkout{/if}</span>
                {#if s.chrome}<span class={badge({ tone: 'warn' })} title="This session can use your signed-in Chrome, outside the allowlist">Chrome</span>{/if}
                {#if s.private}<span class={badge({ tone: 'neutral' })} title="No messages to or from the project's other sessions">private</span>{/if}
              </td>
              <td class={muted}>{s.name || '–'}</td>
              <td>{#if s.state}<span class={badge({ tone: s.state === 'busy' ? 'info' : 'neutral' })}>{s.state}</span>{:else}<span class={muted}>–</span>{/if}</td>
              <td style="font-variant-numeric:tabular-nums">{ago(s.started)}</td>
              <td><InternetControl project={name} session={s} {onchange} /></td>
              <td style="text-align:right"><SessionActions project={name} session={s} {onchange} /></td>
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}
  </section>

  <section class={panel}>
    <div class={panelHeader}>
      <Icon name="inbox" size={14} />Past conversations
      <span class={count}>
        {#if ready}
          <button class={button({ size: 'sm' })} disabled={loading} onclick={load}>
            <Icon name={loading ? 'spinner' : 'refresh'} size={12} />Refresh
          </button>
        {/if}
      </span>
    </div>
    {#if phase}
      <div class={emptyRow}><PhaseNote {phase} /></div>
    {:else if !running}
      <div class={emptyRow}>Start the sandbox to see its saved conversations.</div>
    {:else if error}
      <div class={emptyRow}>{error}</div>
    {:else if convs === null}
      <div class={emptyRow}>Loading…</div>
    {:else if convs.length === 0}
      <div class={emptyRow}>No saved conversations in this sandbox yet.</div>
    {:else}
      <table class={table}>
        <thead><tr><th>Last active</th><th>Where</th><th>Conversation</th><th></th></tr></thead>
        <tbody>
          {#each convs as c (c.id)}
            <tr style={c.gone ? 'opacity:.55' : ''}>
              <td class={date} style="vertical-align:top">{when(c.when)}</td>
              <td style="vertical-align:top">{c.where}{c.gone ? ' (folder gone)' : ''}</td>
              <td>
                <div class={prompt}>
                  <!-- Its folder, with a terminal there; the command to copy is then only what runs in it. -->
                  {#if c.folder}
                    <div class={folder}>
                      <Icon name="folder" size={12} /><span class={folderPath} title={c.folder}>{c.folder}</span>
                      {#if !c.gone}
                        <button class="{button({ size: 'icon', variant: 'ghost' })} {folderBtn}" title="Open a terminal in this folder"
                          aria-label="Open a terminal in {c.folder}" onclick={() => open({ command: 'folder', dir: c.dir }, 'a terminal in the folder')}>
                          <Icon name="terminal" size={12} />
                        </button>
                      {/if}
                    </div>
                  {/if}
                  {#if c.name}<strong class={convName}>{c.name}</strong>{/if}
                  <span class={c.name ? muted : ''}>{c.prompt}</span>
                  {#if (c.run || c.command) && !c.gone && !held.has(c.id)}<CopyCommand command={c.run || c.command!} />{/if}
                </div>
              </td>
              <td class={actions}>
                {#if held.get(c.id)}
                  {@const s = held.get(c.id)!}
                  <span class={runningHere} title="Open in a session now: switch to its terminal">
                    <span>Running</span>
                    {#if s.state}<span class={badge({ tone: s.state === 'busy' ? 'info' : 'neutral' })}>{s.state}</span>{/if}
                    <span>{s.worktree ? `wt ${s.worktree}` : 'main checkout'}</span>
                  </span>
                {:else}
                  <button class={button({ size: 'sm' })} disabled={c.gone} title="Resume it in your terminal, in the folder it ran in"
                    onclick={() => open({ resume: c.id, dir: c.dir }, 'the conversation')}>
                    <Icon name="play" size={11} />Resume
                  </button>
                {/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}
  </section>
</div>
