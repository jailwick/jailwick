<script lang="ts">
  import { css } from 'styled-system/css'
  import { untrack } from 'svelte'
  import { api, type ProjectDetail } from '../../lib/api'
  import { go } from '../../lib/router.svelte'
  import { panel, panelHeader, button, link, muted } from '../../lib/styles'
  import Icon from '../../components/Icon.svelte'
  import { toast } from '../../components/Toasts.svelte'

  // A project's Claude settings: Claude in Chrome and its settings profiles. Which login it uses
  // is chosen in Settings, with the other projects'.
  let { d, onchange }: { d: ProjectDetail; onchange: () => void } = $props()

  const name = $derived(d.name)
  let available = $state<string[]>([])
  api.settings().then((s) => (available = s.profiles)).catch(() => {})

  type Draft = { chrome: boolean; profiles: string[] }
  function fromConfig(): Draft {
    const c = d.config.claude ?? {}
    return { chrome: !!c.chrome, profiles: [...(c.profiles ?? [])] }
  }
  let draft = $state<Draft>({ chrome: false, profiles: [] })
  let saved = $state('')
  let saving = $state(false)
  // Loaded once per project: the page refreshes every few seconds and must not undo edits.
  $effect(() => {
    void name
    untrack(() => {
      draft = fromConfig()
      saved = JSON.stringify(draft)
    })
  })
  const dirty = $derived(JSON.stringify(draft) !== saved)
  // Profiles to choose from: the existing ones, plus any the project lists that are missing.
  const choices = $derived([...new Set([...available, ...draft.profiles])])

  function toggleProfile(p: string) {
    draft.profiles = draft.profiles.includes(p) ? draft.profiles.filter((x) => x !== p) : [...draft.profiles, p]
  }

  async function save() {
    saving = true
    try {
      await api.saveClaude(name, draft)
      saved = JSON.stringify(draft)
      toast('Saved; applies to the next session', 'ok')
      onchange()
    } catch (e) {
      toast((e as Error).message, 'bad')
    } finally {
      saving = false
    }
  }

  const body = css({ display: 'grid', gap: '3.5', px: '4', py: '3.5' })
  const row = css({ display: 'grid', gap: '1.5' })
  const label = css({ fontWeight: 'semibold' })
  const opts = css({ display: 'flex', flexWrap: 'wrap', gap: '1.5' })
  const opt = css({
    display: 'inline-flex', alignItems: 'center', gap: '1.5', px: '2.5', h: 'control.icon', borderRadius: 'control', cursor: 'pointer',
    border: '1px solid {colors.border.strong}', _hover: { bg: 'bg.subtle' },
  })
  const optOn = css({ borderColor: 'selected.border', bg: 'selected.bg', color: 'selected.fg', _hover: { bg: 'selected.bg' } })
  const hint = css({ color: 'fg.muted', fontSize: 'sm' })
  const warn = css({ color: 'warn', fontSize: 'sm' })

</script>

<section class={panel}>
  <div class={panelHeader}><Icon name="settings" size={14} />Claude</div>
  <div class={body}>
    <div class={row}>
      <span class={label}>Login</span>
      <span class={hint}>
        {d.config.claude?.login === 'project' ? "This project's own." : "The app's, shared by projects."}
        <button class={link} onclick={() => go('settings')}>Change in Settings</button>
      </span>
    </div>

    <div class={row}>
      <span class={label}>Claude in Chrome</span>
      <label style="display:inline-flex;gap:var(--spacing-2);align-items:center">
        <input type="checkbox" bind:checked={draft.chrome} />Sessions can use your signed-in Chrome
      </label>
      {#if draft.chrome}<span class={warn}>Chrome reaches any site and account outside the allowlist.</span>{/if}
    </div>

    <div class={row}>
      <span class={label}>Settings profiles</span>
      {#if choices.length === 0}
        <span class={hint}>None yet: create one with <code>jw settings edit --profile NAME</code>.</span>
      {:else}
        <div class={opts}>
          {#each choices as p (p)}
            {@const i = draft.profiles.indexOf(p)}
            <button class="{opt} {i >= 0 ? optOn : ''}" aria-pressed={i >= 0} onclick={() => toggleProfile(p)}>
              {#if i >= 0}<span class={muted}>{i + 1}</span>{/if}{p}{#if !available.includes(p)}<span class={warn}>missing</span>{/if}
            </button>
          {/each}
        </div>
        <span class={hint}>Applied in the order chosen, over the app-wide settings; the project's own file comes last.</span>
      {/if}
    </div>

    <div style="display:flex;justify-content:flex-end;gap:var(--spacing-2)">
      {#if dirty}<button class={button()} onclick={() => (draft = JSON.parse(saved))}>Undo</button>{/if}
      <button class={button({ variant: 'primary' })} disabled={!dirty || saving} onclick={save}>{saving ? 'Saving…' : 'Save'}</button>
    </div>
  </div>
</section>
