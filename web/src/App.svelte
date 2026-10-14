<script lang="ts">
  import { css } from 'styled-system/css'
  import Icon from './components/Icon.svelte'
  import Mark from './components/Mark.svelte'
  import Toasts from './components/Toasts.svelte'
  import { route, go, back, canBack } from './lib/router.svelte'
  import { theme, setTheme, type ThemePref } from './lib/theme.svelte'
  import { poll } from './lib/poll.svelte'
  import { api, type Dashboard } from './lib/api'
  import { updates, loadUpdates } from './lib/updates.svelte'
  import Projects from './views/Projects.svelte'
  import Sessions from './views/Sessions.svelte'
  import Workspaces from './views/Workspaces.svelte'
  import Network from './views/Network.svelte'
  import Images from './views/Images.svelte'
  import Settings from './views/Settings.svelte'
  import NewProject from './views/NewProject.svelte'
  import NewWorkspace from './views/NewWorkspace.svelte'
  import About from './views/About.svelte'
  import Shortcuts from './views/Shortcuts.svelte'
  import Guide from './views/Guide.svelte'

  // The list comes from the configuration at once; what runs fills in when the runtime answers.
  const overview = poll(api.overview, 3000)
  const live = poll(api.live, 3000)
  // A newer jailwick: asked at start and every few hours (the server asks GitHub at most daily).
  $effect(() => {
    const look = () => void loadUpdates().catch(() => {})
    look()
    const t = setInterval(look, 6 * 60 * 60 * 1000)
    return () => clearInterval(t)
  })
  const ov = $derived.by((): Dashboard | undefined => {
    const o = overview.state.data
    const l = live.state.data
    if (!o) return undefined
    return {
      ...o,
      runtime: l?.runtime,
      live: !!l,
      projects: o.projects.map((p) => {
        const op = l?.ops?.[p.name]
        // A restart or stop recreates or removes the containers: the project counts as running
        // until it ends, so it doesn't move between the groups meanwhile.
        const health = l?.health?.[p.name]
        // ...and so does one paused or recovering (it crashed: the runtime brings it back).
        const running = !!l?.projects[p.name] || op === 'restarting' || op === 'stopping' || !!health?.main
        return { ...p, ...l?.projects[p.name], running, op, health }
      }),
    }
  })
  // The sidebar item a page belongs to: "new" pages to their list.
  const section = $derived(({ 'new-project': 'projects', 'new-workspace': 'workspaces' } as Record<string, string>)[route.parts[0]] ?? route.parts[0])
  const sessions = $derived(ov?.projects.reduce((n, p) => n + p.sessions, 0) ?? 0)

  const nav = [
    { id: 'projects', label: 'Projects', icon: 'projects' },
    { id: 'sessions', label: 'Sessions', icon: 'sessions' },
    { id: 'workspaces', label: 'Workspaces', icon: 'workspaces' },
    { id: 'network', label: 'Network', icon: 'network' },
    { id: 'images', label: 'Images', icon: 'images' },
    { id: 'settings', label: 'Settings', icon: 'settings' },
  ]
  // Help pages, at the bottom of the sidebar.
  const help = [
    { id: 'guide', label: 'Guide', icon: 'book' },
    { id: 'shortcuts', label: 'Shortcuts', icon: 'keyboard' },
    { id: 'about', label: 'About', icon: 'info' },
  ]
  const themes: { v: ThemePref; icon: string; label: string }[] = [
    { v: 'light', icon: 'sun', label: 'Light' },
    { v: 'dark', icon: 'moon', label: 'Dark' },
    { v: 'system', icon: 'monitor', label: 'System' },
  ]

  const shell = css({ display: 'grid', gridTemplateColumns: '{sizes.sidebar} 1fr', h: '100%' })
  const side = css({
    bg: 'bg.sidebar', borderRight: '1px solid {colors.border.default}',
    display: 'flex', flexDir: 'column', px: '2.5', pt: '4', pb: '3', gap: '1',
  })
  const brand = css({ display: 'flex', alignItems: 'center', gap: '2', px: '2', pb: '4', fontWeight: 'heavy', fontSize: 'xl', letterSpacing: 'snug' })
  // Labelled, so it isn't read as "collapse the sidebar": the app's own Back (see router).
  const backBtn = css({
    ml: 'auto', display: 'inline-flex', alignItems: 'center', gap: '0.5', h: 'control.compact', pl: '1', pr: '2', borderRadius: 'control',
    color: 'fg.muted', fontSize: 'sm', fontWeight: 'medium', letterSpacing: 0, cursor: 'pointer',
    _hover: { bg: 'bg.hover', color: 'fg.default' }, _disabled: { opacity: 0.4, cursor: 'default', bg: 'transparent' },
  })
  const item = css({
    display: 'flex', alignItems: 'center', gap: '2.5', h: 'control.md', px: '2', borderRadius: 'control',
    color: 'fg.muted', cursor: 'pointer', fontWeight: 'medium', textAlign: 'left', w: '100%',
    _hover: { bg: 'bg.hover', color: 'fg.default' },
  })
  const active = css({ bg: 'selected.bg', color: 'selected.fg', fontWeight: 'semibold', _hover: { bg: 'selected.bg' } })
  const count = css({ ml: 'auto', fontSize: '2xs', color: 'fg.faint', fontVariantNumeric: 'tabular-nums' })
  const newVersion = css({ ml: 'auto', fontSize: '2xs', fontWeight: 'semibold', color: 'accent.fg', fontVariantNumeric: 'tabular-nums' })
  const footer = css({ mt: 'auto', display: 'flex', flexDir: 'column', gap: '2.5', px: '1' })
  const helpList = css({ display: 'grid', gap: '0.5', mx: '-1', pb: '2', borderBottom: '1px solid {colors.border.default}' })
  const runtime = css({ display: 'flex', alignItems: 'center', gap: '2', fontSize: 'xs', color: 'fg.muted', px: '1' })
  const seg = css({ display: 'flex', bg: 'bg.hover', borderRadius: 'control', p: '0.5' })
  const segBtn = css({
    flex: 1, display: 'grid', placeItems: 'center', h: 'control.compact', borderRadius: 'inner', color: 'fg.muted', cursor: 'pointer',
    _hover: { color: 'fg.default' },
  })
  const segOn = css({ bg: 'bg.panel', color: 'fg.default', boxShadow: 'panel' })
  const main = css({ overflow: 'hidden', display: 'flex', flexDir: 'column', minW: 0 })
  const dotOk = css({ w: 'dot.md', h: 'dot.md', borderRadius: 'full', bg: 'ok' })
  const dotBad = css({ w: 'dot.md', h: 'dot.md', borderRadius: 'full', bg: 'bad' })
  const dotWait = css({ w: 'dot.md', h: 'dot.md', borderRadius: 'full', bg: 'fg.faint' })
</script>

<div class={shell}>
  <aside class={side}>
    <div class={brand}>
      <Mark width={26} />
      jailwick
      <button class={backBtn} disabled={!canBack()} title="Back to the previous page" onclick={back}><Icon name="back" size={13} />Back</button>
    </div>
    {#each nav as n}
      <button class="{item} {section === n.id ? active : ''}" onclick={() => go(n.id)}>
        <Icon name={n.icon} />
        {n.label}
        {#if n.id === 'projects' && ov}<span class={count}>{ov.projects.length}</span>{/if}
        {#if n.id === 'sessions' && sessions}<span class={count}>{sessions}</span>{/if}
        {#if n.id === 'workspaces' && ov?.workspaces.length}<span class={count}>{ov.workspaces.length}</span>{/if}
      </button>
    {/each}
    <div class={footer}>
      <div class={helpList}>
        {#each help as h}
          <button class="{item} {section === h.id ? active : ''}" onclick={() => go(h.id)}>
            <Icon name={h.icon} size={15} />{h.label}
            {#if h.id === 'about' && updates.status?.available}
              <span class={newVersion} title="jailwick {updates.status.latest?.version} is available (you have {updates.status.current})">{updates.status.latest?.version} available</span>
            {:else if h.id === 'about' && ov?.version}<span class={count}>{ov.version}</span>{/if}
          </button>
        {/each}
      </div>
      <div class={runtime} title={ov?.runtime?.error ?? ''}>
        <span class={ov?.runtime?.error ? dotBad : ov?.runtime ? dotOk : dotWait}></span>
        {#if ov?.runtime}
          {ov.runtime.error ? `${ov.runtime.name || 'runtime'}: not running` : `${ov.runtime.platform ?? ov.runtime.name} ${ov.runtime.version ?? ''}`}
        {:else}
          connecting…
        {/if}
      </div>
      <div class={seg} role="radiogroup" aria-label="Theme" title="Theme: {themes.find((t) => t.v === theme.pref)?.label}">
        {#each themes as t}
          <button class="{segBtn} {theme.pref === t.v ? segOn : ''}" role="radio" aria-checked={theme.pref === t.v}
            title="{t.label} theme" aria-label="{t.label} theme" onclick={() => setTheme(t.v)}><Icon name={t.icon} size={14} /></button>
        {/each}
      </div>
    </div>
  </aside>
  <main class={main}>
    {#if route.parts[0] === 'sessions'}
      <Sessions />
    {:else if route.parts[0] === 'workspaces'}
      <Workspaces names={ov?.workspaces ?? []} selected={route.parts[1]} tab={route.parts[2]} projects={ov?.projects ?? []} onchanged={() => overview.refresh()} />
    {:else if route.parts[0] === 'network'}
      <Network projects={ov?.projects ?? []} />
    {:else if route.parts[0] === 'images'}
      <Images />
    {:else if route.parts[0] === 'settings'}
      <Settings projects={(ov?.projects ?? []).map((p) => p.name)} />
    {:else if route.parts[0] === 'new-project'}
      {#key route.parts[1]}<NewProject dir={route.parts[1]} oncreated={() => overview.refresh()} />{/key}
    {:else if route.parts[0] === 'about'}
      <About />
    {:else if route.parts[0] === 'shortcuts'}
      <Shortcuts />
    {:else if route.parts[0] === 'guide'}
      <Guide />
    {:else if route.parts[0] === 'new-workspace'}
      <NewWorkspace oncreated={() => overview.refresh()} />
    {:else}
      <Projects overview={ov} error={overview.state.error} selected={route.parts[1]} tab={route.parts[2]} />
    {/if}
  </main>
</div>
<Toasts />
