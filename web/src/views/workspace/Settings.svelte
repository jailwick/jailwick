<script lang="ts">
  import { css } from 'styled-system/css'
  import { api } from '../../lib/api'
  import { needRestart } from '../../lib/restarts.svelte'
  import { SetupForm } from '../../lib/setupForm.svelte'
  import { button, muted, panel, panelHeader } from '../../lib/styles'
  import SetupFields from '../../components/SetupFields.svelte'
  import { toast } from '../../components/Toasts.svelte'

  // A workspace's settings: the same fields as when it was created, on one page, with their problems
  // as they come. (The parent keys it by name: another workspace is another form.)
  // Renaming it calls onsaved with the new name.
  let { name, onsaved }: { name: string; onsaved: (name: string) => void } = $props()

  const form = new SetupForm((values) => api.draftWorkspace({ name }, values), true)
  let saving = $state(false)
  form.start()

  async function save() {
    saving = true
    try {
      const { name: saved = name, changed } = await api.saveWorkspace(name, form.values)
      needRestart(changed)
      toast(saved !== name ? `Renamed to ${saved}` : 'Saved; restarts apply it', 'ok')
      onsaved(saved)
    } catch (e) {
      toast((e as Error).message, 'bad')
      await form.refresh()
    } finally {
      saving = false
    }
  }

  const grid = css({ display: 'grid', gridTemplateColumns: { base: '1fr', xl: '1fr 1fr' }, gap: '5', alignItems: 'start' })
  const body = css({ p: '4', display: 'grid', gap: '4' })
  const bar = css({ display: 'flex', alignItems: 'center', gap: '3', mt: '5' })
  const problems = css({ m: 0, pl: '4', color: 'bad', display: 'grid', gap: '1', fontSize: 'md' })
</script>

{#if form.error}<p class={muted}>{form.error}</p>{/if}
{#if form.draft}
  <div class={grid}>
    {#each form.draft.steps as s (s.id)}
      <section class={panel}>
        <div class={panelHeader}>{s.title}</div>
        <div class={body}><SetupFields {form} fields={s.fields} /></div>
      </section>
    {/each}
  </div>
  {#if form.draft.problems.length}
    <ul class={problems} style="margin-top:var(--spacing-4)">{#each form.draft.problems as p}<li>{p}</li>{/each}</ul>
  {/if}
  <div class={bar}>
    <button class={button({ variant: 'primary' })} disabled={saving || !form.changed || form.checking || form.draft.problems.length > 0} onclick={save}>
      {saving ? 'Saving…' : 'Save'}
    </button>
    <button class={button()} disabled={saving || !form.changed} onclick={() => form.start()}>Discard changes</button>
    <span class={muted} style="font-size:var(--font-sizes-sm)">Running projects see the change after a restart.</span>
  </div>
{/if}
