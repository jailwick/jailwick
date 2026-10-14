<script lang="ts">
  import { css } from 'styled-system/css'
  import { api, type ProjectSummary } from '../lib/api'
  import { needRestart } from '../lib/restarts.svelte'
  import { go } from '../lib/router.svelte'
  import { button, mono, muted, panel, tab as tabC, tabOn } from '../lib/styles'
  import Page from '../lib/Page.svelte'
  import ConfirmBar from '../components/ConfirmBar.svelte'
  import Icon from '../components/Icon.svelte'
  import RestartBanner from '../components/RestartBanner.svelte'
  import { toast } from '../components/Toasts.svelte'
  import Overview from './workspace/Overview.svelte'
  import Settings from './workspace/Settings.svelte'
  import Notes from './workspace/Notes.svelte'

  // The workspaces, one at a time: what it is now (Overview), its settings and its notes.
  let {
    names,
    selected,
    tab = 'overview',
    projects,
    onchanged,
  }: { names: string[]; selected?: string; tab?: string; projects: ProjectSummary[]; onchanged: () => void } = $props()

  const current = $derived(selected && names.includes(selected) ? selected : names[0])
  const tabs = [
    { id: 'overview', label: 'Overview' },
    { id: 'settings', label: 'Settings' },
    { id: 'notes', label: 'Notes' },
  ]
  let removing = $state(false)
  let version = $state(0) // a save makes the overview load again

  // After a save: the overview loads again, and a renamed workspace is shown under its new name.
  function saved(name: string) {
    version++
    onchanged()
    if (name !== current) go('workspaces', name, 'settings')
  }

  async function remove() {
    removing = false
    try {
      const { changed } = await api.removeWorkspace(current)
      needRestart(changed)
      toast(`Removed ${current}. Its repos and handoff folder are kept.`, 'ok')
      onchanged()
      go('workspaces')
    } catch (e) {
      toast((e as Error).message, 'bad')
    }
  }

  // Which workspace: its name as a heading (a picker when there are several), then its tabs.
  const pickRow = css({ display: 'flex', alignItems: 'center', gap: '2.5', flexWrap: 'wrap', mb: '3' })
  const wsName = css({ display: 'inline-flex', alignItems: 'center', gap: '2', fontSize: '2xl', fontWeight: 'bold', '& svg': { color: 'fg.muted' } })
  const picker = css({
    h: 'control.icon', px: '2', borderRadius: 'control', border: '1px solid {colors.border.strong}', bg: 'bg.panel', color: 'fg.default',
    fontSize: 'base', fontWeight: 'semibold',
  })
  const tabsC = css({ display: 'flex', gap: '1', alignItems: 'center', borderBottom: '1px solid {colors.border.default}', mb: '5' })
  const empty = css({ p: '7', textAlign: 'center', display: 'grid', gap: '2', justifyItems: 'center' })
</script>

<Page title="Workspaces" subtitle="Related repos: each project sees the others read-only, with secrets hidden.">
  {#snippet actions()}
    <button class={button({ variant: 'primary' })} onclick={() => go('new-workspace')}><Icon name="plus" size={13} />New workspace</button>
  {/snippet}

  <RestartBanner {projects} />

  {#if names.length === 0}
    <section class="{panel} {empty}">
      <p style="font-weight:var(--font-weights-semibold)">No workspaces</p>
      <p class={muted}>Group a folder of related repos so each project sees the others read-only.</p>
      <button class={button({ variant: 'primary' })} onclick={() => go('new-workspace')}><Icon name="plus" size={13} />New workspace</button>
      <p class={muted} style="font-size:var(--font-sizes-sm)">Or in a terminal: <span class={mono}>jw workspace add &lt;name&gt; &lt;folder&gt;</span></p>
    </section>
  {:else}
    <div class={pickRow}>
      {#if names.length > 1}
        <label class={wsName} for="workspace-pick"><Icon name="workspaces" size={15} /></label>
        <select id="workspace-pick" class={picker} value={current} onchange={(e) => go('workspaces', e.currentTarget.value, tab)}>
          {#each names as n (n)}<option value={n}>{n}</option>{/each}
        </select>
      {:else}
        <h2 class={wsName}><Icon name="workspaces" size={15} />{current}</h2>
      {/if}
      <button class={button({ variant: 'danger', size: 'sm' })} style="margin-left:auto" onclick={() => (removing = true)}>Remove workspace</button>
    </div>
    <nav class={tabsC}>
      {#each tabs as t (t.id)}
        <button class="{tabC} {tab === t.id ? tabOn : ''}" onclick={() => go('workspaces', current, t.id)}>{t.label}</button>
      {/each}
    </nav>
    {#if removing}
      <ConfirmBar
        message={`Remove ${current}? Its projects leave it; the repos, the handoff folder and notes are kept.`}
        action="Remove"
        danger
        onconfirm={remove}
        oncancel={() => (removing = false)}
      />
    {/if}
    {#key `${current}/${version}`}
      {#if tab === 'settings'}
        <Settings name={current} onsaved={saved} />
      {:else if tab === 'notes'}
        <Notes name={current} />
      {:else}
        <Overview name={current} />
      {/if}
    {/key}
  {/if}
</Page>
