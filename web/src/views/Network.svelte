<script lang="ts">
  import { css } from 'styled-system/css'
  import { onDestroy } from 'svelte'
  import { api, type ProjectSummary } from '../lib/api'
  import { poll } from '../lib/poll.svelte'
  import { when } from '../lib/format'
  import { panel, panelHeader, table, mono, muted, faint, badge, button, input, panelCount as count } from '../lib/styles'
  import Page from '../lib/Page.svelte'
  import Icon from '../components/Icon.svelte'
  import { toast } from '../components/Toasts.svelte'
  import PresetEditor from '../components/PresetEditor.svelte'
  import ConfirmBar from '../components/ConfirmBar.svelte'
  import HostGroups from '../components/HostGroups.svelte'
  import ProjectPicker from '../components/ProjectPicker.svelte'
  import Menu from '../components/Menu.svelte'

  let { projects }: { projects: ProjectSummary[] } = $props()
  const net = poll(api.network, 5000)
  onDestroy(() => net.stop())
  const n = $derived(net.state.data)
  let domain = $state('')
  // The preset being edited: null for none, 'new' for a new one, else its name.
  let editing = $state<string | null>(null)
  let deleting = $state('')
  // Blocked hosts, ten at first and ten more at a time.
  let limit = $state(10)
  const more = css({
    display: 'flex', gap: '3', alignItems: 'center', px: '4', py: '2.5', fontSize: 'md', color: 'fg.muted', borderTop: '1px solid {colors.border.default}',
  })

  async function deletePreset(name: string) {
    deleting = ''
    try {
      await api.deletePreset(name)
      toast(`Removed the ${name} preset`, 'ok')
      net.refresh()
    } catch (e) {
      toast((e as Error).message, 'bad')
    }
  }
  function saved() {
    editing = null
    net.refresh()
  }

  // The catalogue as collapsible rows; the user's own presets under "yours".
  const presetGroups = $derived(
    (n?.presets ?? []).map((p) => ({
      key: p.name, title: p.name, subtitle: p.description, hosts: p.hosts,
      category: p.category === 'custom' ? 'yours' : p.category, preset: p,
    })),
  )
  const usedBy = (preset: string) =>
    Object.entries(n?.project_presets ?? {}).filter(([, list]) => list.includes(preset)).map(([proj]) => proj)

  // Adds a preset to a project, or removes it; true when it worked.
  async function togglePreset(preset: string, project: string, add: boolean): Promise<boolean> {
    try {
      if (add) await api.allowPreset(preset, project)
      else await api.denyPreset(preset, project)
      toast(add ? `The ${preset} preset is allowed for ${project}` : `The ${preset} preset is removed from ${project}`, 'ok')
      await net.refresh()
      return true
    } catch (e) {
      toast((e as Error).message, 'bad')
      return false
    }
  }

  async function allow(d: string, project = '') {
    try {
      const r = await api.allow(d, project)
      toast(`${r.domain} allowed ${project ? `for ${project}` : 'everywhere'}${r.reloaded.length ? ` · reloaded ${r.reloaded.join(', ')}` : ''}`, 'ok')
      net.refresh()
    } catch (e) {
      toast((e as Error).message, 'bad')
    }
  }
  async function remove(d: string) {
    try {
      await api.deny(d)
      toast(`${d} removed`, 'ok')
      net.refresh()
    } catch (e) {
      toast((e as Error).message, 'bad')
    }
  }
  function submit(e: SubmitEvent) {
    e.preventDefault()
    if (domain.trim()) allow(domain.trim()).then(() => (domain = ''))
  }

  const grid = css({ display: 'grid', gridTemplateColumns: { base: '1fr', xl: '3fr 2fr' }, gap: '5', alignItems: 'start' })
  const form = css({ display: 'flex', gap: '2', p: '3', borderBottom: '1px solid {colors.border.default}' })
  const emptyRow = css({ px: '4', py: '4', color: 'fg.muted' })
  const problems = css({ px: '4', py: '2', color: 'warn', fontSize: 'sm', borderBottom: '1px solid {colors.border.default}' })
  const userRow = css({ display: 'flex', alignItems: 'center', px: '4', py: '1.5', borderBottom: '1px solid {colors.border.default}', _last: { borderBottom: 'none' } })
  // Who uses a preset: cut short so the badges and the add button keep their place.
  const usedC = css({ color: 'fg.muted', fontSize: 'sm', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis', minW: 0, flexShrink: 1 })
</script>

<Page title="Network" subtitle="Sandboxes reach the internet only through an allowlist proxy; everything else, and your machine, is blocked.">
  <div class={grid}>
    <div style="display:flex;flex-direction:column;gap:var(--spacing-5)">
      <section class={panel}>
        <div class={panelHeader}><Icon name="alert" size={14} />Blocked requests<span class={count}>{n?.blocked.length ?? 0} hosts</span></div>
        {#if !n || n.blocked.length === 0}
          <div class={emptyRow}>Nothing blocked: every host a sandbox asked for is allowed, kept blocked or discarded.</div>
        {:else}
          <table class={table}>
            <thead><tr><th>Host</th><th>Count</th><th>Last</th><th>Projects</th><th></th></tr></thead>
            <tbody>
              {#each n.blocked.slice(0, limit) as b (b.Host)}
                <tr>
                  <td class={mono}>{b.Host}</td>
                  <td style="font-variant-numeric:tabular-nums">{b.Count}</td>
                  <td class={muted} style="white-space:nowrap">{when(b.Last)}</td>
                  <td>{#each b.Projects as p}<span class={badge()} style="margin-right:var(--spacing-1)">{p}</span>{/each}</td>
                  <td style="text-align:right;white-space:nowrap">
                    <!-- The narrower choice is the main one: allowing everywhere opens it to every project. -->
                    {#if b.Projects.length === 1}
                      <button class={button({ size: 'sm' })} onclick={() => allow(b.Host, b.Projects[0])}>Allow for {b.Projects[0]}</button>
                    {:else}
                      <Menu items={b.Projects.map((p) => ({ label: `Allow for ${p}`, run: () => allow(b.Host, p) }))}>
                        {#snippet label()}Allow for…{/snippet}
                      </Menu>
                    {/if}
                    <button class={button({ size: 'sm' })} title="Add it to your allowlist: every project can reach it" onclick={() => allow(b.Host)}>Allow everywhere</button>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
          {#if n.blocked.length > limit}
            <div class={more}>
              <button class={button({ size: 'sm' })} onclick={() => (limit += 10)}>
                <Icon name="chevronDown" size={12} />Show {Math.min(10, n.blocked.length - limit)} more
              </button>
              <span>{limit} of {n.blocked.length} shown</span>
            </div>
          {/if}
        {/if}
      </section>
      <section class={panel}>
        <div class={panelHeader}><Icon name="network" size={14} />Your allowlist<span class={count}>applies to every project</span></div>
        <form class={form} onsubmit={submit}>
          <input class={input} style="flex:1" placeholder="api.example.com or .example.com (with subdomains)" bind:value={domain} />
          <button class={button({ variant: 'primary' })} type="submit"><Icon name="plus" size={13} />Allow</button>
        </form>
        {#if !n || n.user.length === 0}
          <div class={emptyRow}>Only the built-in presets.</div>
        {:else}
          {#each n.user as d}
            <div class={userRow}>
              <span class={mono}>{d}</span>
              <button class={button({ size: 'sm', variant: 'danger' })} style="margin-left:auto" onclick={() => remove(d)}><Icon name="x" size={12} />Remove</button>
            </div>
          {/each}
        {/if}
      </section>
      {#if n && Object.keys(n.projects).length}
        <section class={panel}>
          <div class={panelHeader}>Per project (own domains)</div>
          <table class={table}>
            <tbody>
              {#each Object.entries(n.projects) as [p, list]}
                <tr><td style="width:140px"><strong>{p}</strong></td><td class={mono}>{list.join('  ')}</td></tr>
              {/each}
            </tbody>
          </table>
        </section>
      {/if}
    </div>
    <section class={panel}>
      <div class={panelHeader}>
        <Icon name="lock" size={14} />Presets
        <button class={button({ size: 'sm' })} style="margin-left:auto;text-transform:none;letter-spacing:0" onclick={() => (editing = 'new')}>
          <Icon name="plus" size={12} />New preset
        </button>
      </div>
      {#if editing === 'new'}
        <PresetEditor onsaved={saved} oncancel={() => (editing = null)} />
      {/if}
      {#if n?.preset_problems}<div class={problems}>Presets of yours left out: {n.preset_problems}</div>{/if}
      {#if n}
        <HostGroups groups={presetGroups} placeholder="Search presets and domains">
          {#snippet head(g)}
            {#if g.preset.always}<span class={badge()} title="Always allowed: every sandbox needs it">always</span>{/if}
            {#if g.preset.broad}<span class={badge({ tone: 'warn' })} title="Broad: {g.preset.broad}">broad</span>{/if}
            {#if usedBy(g.key).length}<span class={usedC} title={usedBy(g.key).join(', ')}>{usedBy(g.key).join(', ')}</span>{/if}
            {#if !g.preset.always}
              <ProjectPicker title="Add {g.key} to a project" projects={projects.map((p) => p.name)} selected={usedBy(g.key)}
                ontoggle={(project, add) => togglePreset(g.key, project, add)} />
            {/if}
          {/snippet}
          {#snippet body(g)}
            {#if g.preset.broad}<div class={muted} style="font-size:var(--font-sizes-sm)">Broad: {g.preset.broad}</div>{/if}
            {#if g.preset.category === 'custom'}
              {#if editing === g.key}
                <PresetEditor preset={g.preset} onsaved={saved} oncancel={() => (editing = null)} />
              {:else}
                <div style="display:flex;gap:var(--spacing-2);margin-top:var(--spacing-1)">
                  <button class={button({ size: 'sm' })} onclick={() => (editing = g.key)}>Edit</button>
                  <button class={button({ size: 'sm', variant: 'danger' })} onclick={() => (deleting = g.key)}>Remove</button>
                </div>
              {/if}
              {#if deleting === g.key}
                <ConfirmBar message={`Remove the ${g.key} preset? Projects that use it must drop it first.`} action="Remove preset"
                  danger onconfirm={() => deletePreset(g.key)} oncancel={() => (deleting = '')} />
              {/if}
            {/if}
          {/snippet}
        </HostGroups>
      {/if}
    </section>
  </div>
</Page>
