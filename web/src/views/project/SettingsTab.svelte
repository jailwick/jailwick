<script lang="ts">
  import { css } from 'styled-system/css'
  import { api, type ProjectDetail } from '../../lib/api'
  import { SetupForm } from '../../lib/setupForm.svelte'
  import { button, muted, panel, panelHeader } from '../../lib/styles'
  import Icon from '../../components/Icon.svelte'
  import SetupFields from '../../components/SetupFields.svelte'
  import { toast } from '../../components/Toasts.svelte'
  import ClaudePanel from './ClaudePanel.svelte'
  import NodeModulesCard from './NodeModulesCard.svelte'
  import WorkspaceCard from './WorkspaceCard.svelte'

  // A project's settings, with the same fields as when it was set up and their problems as they
  // come: services and logins, toolset, network, worktrees and resources. The name and the
  // workspace aren't among them (the name names its files and containers; the workspace card joins
  // and leaves). They share one Save, which stays in sight while they do. Under them, the cards
  // with a Save of their own: Claude, the workspace, separate node_modules. A restart applies a
  // change; the page then lists it as pending.
  // (The parent keys it by project: another project is another form.)
  let { d, onchange }: { d: ProjectDetail; onchange: () => void } = $props()

  const form = new SetupForm((values) => api.draftSettings(d.name, values), true)
  let saving = $state(false)
  form.start()

  async function save() {
    saving = true
    try {
      await api.saveSettings(d.name, form.values)
      toast(d.running ? 'Saved: a restart applies it' : 'Saved: the next start uses it', 'ok')
      await form.start()
      onchange()
    } catch (e) {
      toast((e as Error).message, 'bad')
      await form.refresh()
    } finally {
      saving = false
    }
  }

  const icons: Record<string, string> = { services: 'database', toolset: 'settings', network: 'network', worktrees: 'branch', resources: 'monitor' }

  // Cards of different heights in two columns, without holes.
  const cols = css({ columns: { base: 1, xl: 2 }, columnGap: '5', '& > section': { breakInside: 'avoid', mb: '5' } })
  const body = css({ p: '4', display: 'grid', gap: '4' })
  // Stays in sight while the cards scroll.
  const bar = css({
    position: 'sticky', bottom: 0, zIndex: 2, display: 'flex', alignItems: 'center', gap: '3', flexWrap: 'wrap',
    mx: '-6', px: '6', py: '3', bg: 'bg.canvas', borderTop: '1px solid {colors.border.default}',
  })
  const problems = css({ m: 0, mb: '4', pl: '4', color: 'bad', display: 'grid', gap: '1', fontSize: 'md' })
  const more = css({ display: 'grid', gridTemplateColumns: { base: '1fr', xl: '1fr 1fr' }, gap: '5', alignItems: 'start', mt: '6' })
  const stack = css({ display: 'grid', gap: '5', alignContent: 'start', minW: 0 })
</script>

<div>
{#if form.error}<p class={muted}>{form.error}</p>{/if}
{#if form.draft}
  {#if form.draft.problems.length}
    <ul class={problems}>{#each form.draft.problems as p}<li>{p}</li>{/each}</ul>
  {/if}
  <div class={cols}>
    {#each form.draft.steps as s (s.id)}
      <section class={panel}>
        <div class={panelHeader}><Icon name={icons[s.id] ?? 'settings'} size={14} />{s.title}</div>
        <div class={body}><SetupFields {form} fields={s.fields} /></div>
      </section>
    {/each}
  </div>
  <div class={bar}>
    <button class={button({ variant: 'primary' })} disabled={saving || !form.changed || form.checking || form.draft.problems.length > 0} onclick={save}>
      {saving ? 'Saving…' : 'Save'}
    </button>
    <button class={button()} disabled={saving || !form.changed} onclick={() => form.start()}>Discard changes</button>
    <span class={muted} style="font-size:var(--font-sizes-sm)">
      {form.changed ? 'Unsaved changes.' : ''} A restart applies a change (a toolset change rebuilds the image).
    </span>
  </div>
{:else if !form.error}
  <p class={muted}>Loading…</p>
{/if}
</div>

<div class={more}>
  <ClaudePanel {d} {onchange} />
  <div class={stack}>
    <WorkspaceCard project={d.name} {onchange} />
    {#if d.node_modules.used}<NodeModulesCard {d} {onchange} />{/if}
  </div>
</div>
