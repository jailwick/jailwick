<script lang="ts">
  import { css } from 'styled-system/css'
  import { api, type SessionNotify, type Settings } from '../lib/api'
  import { go } from '../lib/router.svelte'
  import { button, chip, input, link, panel, panelHeader, mono, muted } from '../lib/styles'
  import ImageCheck from '../components/ImageCheck.svelte'
  import Page from '../lib/Page.svelte'
  import Icon from '../components/Icon.svelte'
  import SecretsPanel from './settings/SecretsPanel.svelte'
  import LoginPanel from './settings/LoginPanel.svelte'
  import TerminalPicker from './settings/TerminalPicker.svelte'
  import { toast } from '../components/Toasts.svelte'

  let { projects }: { projects: string[] } = $props()

  let s = $state<Settings | null>(null)
  const load = () => api.settings().then((v) => (s = v))
  async function saveUpdates(on: boolean) {
    try {
      await api.saveUpdates(on)
      toast(on ? 'Checking for new versions daily' : 'No daily check for new versions', 'ok')
    } catch (e) {
      toast((e as Error).message, 'bad')
    }
    await load()
  }
  async function saveNotifications(change: Parameters<typeof api.saveNotifications>[0], done: string) {
    try {
      await api.saveNotifications(change)
      toast(done, 'ok')
      load()
    } catch (e) {
      toast((e as Error).message, 'bad')
    }
  }
  // Where a session tells you it waits for you (a permission, an answer, or done).
  const sessionModes: { v: SessionNotify; label: string; help: string }[] = [
    { v: 'auto', label: 'Auto', help: "In the session's terminal where it shows notifications (iTerm2, Ghostty, kitty, WezTerm): a click brings back its tab. Elsewhere (macOS Terminal, tmux), from jailwick." },
    { v: 'terminal', label: 'Terminal', help: "Always in the session's terminal; where it can't show a notification, its bell rings." },
    { v: 'app', label: 'jailwick app', help: 'From jailwick, whatever the terminal: a click opens the project\'s sessions.' },
    { v: 'off', label: 'Off', help: 'No notification when a session waits.' },
  ]
  const sessionMode = $derived<SessionNotify>(s?.global.session_notifications ?? 'auto')

  // The image every project builds on unless it names its own: empty is jailwick's base.
  let img = $state({ base: '', user: '', home: '' })
  let imgSaved = $state('')
  $effect(() => {
    const g = s?.global.image
    const now = { base: g?.base ?? '', user: g?.user ?? '', home: g?.home ?? '' }
    if (JSON.stringify(now) !== imgSaved) {
      img = now
      imgSaved = JSON.stringify(now)
    }
  })
  const imgChanged = $derived(JSON.stringify(img) !== imgSaved)
  async function saveImage() {
    try {
      await api.saveDefaultImage(img)
      toast(img.base ? `Projects build on ${img.base} unless they name their own` : "Projects build on jailwick's base", 'ok')
      load()
    } catch (e) {
      toast((e as Error).message, 'bad')
    }
  }
  const appOn = $derived(s?.global.notifications !== false)
  load()
  const kv = css({ display: 'grid', gridTemplateColumns: '{sizes.label} 1fr', rowGap: '2.5', px: '4', py: '3.5', '& dt': { color: 'fg.muted' } })
  const grid = css({ display: 'grid', gridTemplateColumns: { base: '1fr', xl: '1fr 1fr' }, gap: '5', alignItems: 'start' })
  const note = css({ px: '4', py: '3', color: 'fg.muted', fontSize: 'md' })
  const imgBody = css({ px: '4', py: '3.5', display: 'grid', gap: '3' })
  const imgRow = css({ display: 'grid', gridTemplateColumns: { base: '1fr', md: '1fr 1fr' }, gap: '3' })
  const fieldC = css({ display: 'grid', gap: '1', fontSize: 'md', '& > span': { fontWeight: 'semibold' } })
  const check = css({ display: 'flex', gap: '2', alignItems: 'baseline', cursor: 'pointer', '&:has(input:disabled)': { color: 'fg.faint', cursor: 'default' } })
  const modes = css({ display: 'flex', gap: '1.5', flexWrap: 'wrap' })
  const help = css({ mt: '1.5', color: 'fg.muted', fontSize: 'sm' })
</script>

<Page title="Settings" subtitle="For every project. A project's own are on its page; the theme is at the bottom of the sidebar.">
  <div class={grid}>
    {#if s}
      <section class={panel}>
        <div class={panelHeader}><Icon name="settings" size={14} />General</div>
        <dl class={kv}>
          <dt>Terminal</dt><dd><TerminalPicker value={s.global.terminal ?? ''} onchange={load} /></dd>
        </dl>
      </section>
      <section class={panel}>
        <div class={panelHeader}><Icon name="bell" size={14} />Notifications</div>
        <dl class={kv}>
          <dt>From jailwick</dt>
          <dd>
            <label class={check}>
              <input type="checkbox" checked={appOn} onchange={(e) => saveNotifications({ on: e.currentTarget.checked }, e.currentTarget.checked ? 'Notifications on' : 'Notifications off')} />
              <span>When a long start ends, a handoff arrives, a host is blocked, or a session waits (below)</span>
            </label>
          </dd>
          <dt>Sessions waiting</dt>
          <dd>
            <div class={modes} role="radiogroup" aria-label="Where sessions tell they wait">
              {#each sessionModes as m (m.v)}
                <label class={chip}>
                  <input type="radio" name="session-notify" checked={sessionMode === m.v}
                    onchange={() => saveNotifications({ sessions: m.v }, `Sessions waiting: ${m.label.toLowerCase()}`)} />{m.label}
                </label>
              {/each}
            </div>
            <p class={help}>{sessionModes.find((m) => m.v === sessionMode)?.help} New sessions follow a change.</p>
            {#if !appOn && (sessionMode === 'app' || sessionMode === 'auto')}
              <p class={help}>jailwick's notifications are off, so {sessionMode === 'app' ? 'none come' : 'only terminals that show notifications tell you'}.</p>
            {/if}
          </dd>
          <dt>Blocked hosts</dt>
          <dd>
            <label class={check}>
              <input type="checkbox" checked={s.global.blocked_notifications !== false} disabled={!appOn}
                onchange={(e) => saveNotifications({ blocked: e.currentTarget.checked }, e.currentTarget.checked ? 'Blocked hosts: notified' : 'Blocked hosts: not notified')} />
              <span>Tell me when a sandbox is refused a host (always from jailwick; the Network tab lists them either way)</span>
            </label>
          </dd>
        </dl>
      </section>
      <section class={panel}>
        <div class={panelHeader}><Icon name="images" size={14} />Default image</div>
        <div class={imgBody}>
          <p class={muted} style="font-size:var(--font-sizes-md)">
            What projects build on unless they name their own (the project's Settings, Image). Empty: jailwick's base. A
            Debian or Ubuntu image with the user below; jailwick adds its tools, the stacks and Claude Code on top.
          </p>
          <label class={fieldC}><span>Base image</span>
            <input class={input} placeholder="jailwick's base (e.g. eclipse-temurin:21-jdk)" bind:value={img.base} />
          </label>
          <div class={imgRow}>
            <label class={fieldC}><span>Sandbox user</span><input class={input} placeholder="node" bind:value={img.user} /></label>
            <label class={fieldC}><span>Home folder</span><input class={input} placeholder="/home/node" bind:value={img.home} /></label>
          </div>
          {#if img.base}<ImageCheck ref={img.base} user={img.user} home={img.home} />{/if}
          <div style="display:flex;gap:var(--spacing-2)">
            <button class={button({ variant: 'primary', size: 'sm' })} disabled={!imgChanged} onclick={saveImage}>Save</button>
            {#if imgChanged}<button class={button({ size: 'sm' })} onclick={() => (img = JSON.parse(imgSaved))}>Undo</button>{/if}
          </div>
        </div>
      </section>
      <section class={panel}>
        <div class={panelHeader}><Icon name="refresh" size={14} />Updates</div>
        <dl class={kv}>
          <dt>New versions</dt>
          <dd>
            <label class={check}>
              <input type="checkbox" checked={s.global.updates !== false} onchange={(e) => saveUpdates(e.currentTarget.checked)} />
              <span>Check once a day and tell me when a new version is out</span>
            </label>
            <p class={help}>Asks GitHub for the latest release; nothing else is sent. About shows it and updates jailwick (with Homebrew, where it came from Homebrew). <span class={mono}>jw update</span> checks either way.</p>
          </dd>
        </dl>
      </section>
      <section class={panel}>
        <div class={panelHeader}><Icon name="info" size={14} />Environment</div>
        <dl class={kv}>
          <dt>Runtime</dt><dd>{s.global.runtime || 'detected'}</dd>
          <dt>Git identity</dt><dd>{s.global.git?.name ?? '–'} <span class={muted}>{s.global.git?.email ?? ''}</span></dd>
          <dt>Permission prompts</dt><dd>{s.global.claude?.yolo ? 'skipped (YOLO) by default' : 'on by default'}</dd>
        </dl>
        <p class={note}>Change these in config.yaml: <span class={mono}>jw config edit --global</span>.</p>
      </section>
      <LoginPanel {s} onchange={load} />
      <SecretsPanel {projects} />
      <section class={panel}>
        <div class={panelHeader}><Icon name="folder" size={14} />Folders</div>
        <dl class={kv}>
          <dt>Configuration</dt><dd class={mono}>{s.config_dir}</dd>
        </dl>
        <p class={note}>Every folder jailwick keeps files in is listed in <button class={link} onclick={() => go('about')}>About</button>.</p>
      </section>
    {/if}
  </div>
</Page>
