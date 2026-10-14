<script lang="ts">
  import { css } from 'styled-system/css'
  import { untrack } from 'svelte'
  import { api, type Preset } from '../lib/api'
  import { button, input } from '../lib/styles'
  import { toast } from './Toasts.svelte'

  // Creates a preset of the user's own, or edits one (preset given): a name, what it is for, and
  // its domains, separated by commas or new lines. Any project can then add it.
  let { preset = null, onsaved, oncancel }: { preset?: Preset | null; onsaved: (name: string) => void; oncancel: () => void } = $props()

  const initial = untrack(() => preset)
  let name = $state(initial?.name ?? '')
  let description = $state(initial?.description ?? '')
  let hosts = $state(initial?.hosts.join('\n') ?? '')
  let saving = $state(false)

  async function save(e: SubmitEvent) {
    e.preventDefault()
    saving = true
    try {
      const r = await api.savePreset(name.trim(), description, hosts)
      const reloaded = r.reloaded.length ? ` · reloaded ${r.reloaded.join(', ')}` : ''
      toast(`Saved the ${r.name} preset: ${r.hosts.length} domain${r.hosts.length === 1 ? '' : 's'}${reloaded}`, 'ok')
      onsaved(r.name)
    } catch (err) {
      toast((err as Error).message, 'bad')
    } finally {
      saving = false
    }
  }

  const form = css({ display: 'grid', gap: '2.5', p: '4', borderBottom: '1px solid {colors.border.default}', bg: 'bg.subtle' })
  const area = css({
    minH: '96px', p: '2.5', borderRadius: 'control', border: '1px solid {colors.border.strong}', bg: 'bg.panel',
    fontFamily: 'mono', fontSize: 'sm', resize: 'vertical', outline: 'none', _focus: { borderColor: 'accent.default' },
  })
  const actions = css({ display: 'flex', gap: '2', justifyContent: 'flex-end', alignItems: 'center' })
  const hint = css({ color: 'fg.muted', fontSize: 'sm' })
</script>

<form class={form} onsubmit={save}>
  <input class={input} placeholder="name (lowercase, e.g. acme)" bind:value={name} disabled={!!initial} required />
  <input class={input} placeholder="what it is for (optional)" bind:value={description} />
  <textarea class={area} bind:value={hosts} required
    placeholder={'api.example.com, .cdn.example.com\nanother.example.org'}></textarea>
  <span class={hint}>Comma or line separated; .example.com includes its subdomains.</span>
  <div class={actions}>
    <button class={button()} type="button" onclick={oncancel}>Cancel</button>
    <button class={button({ variant: 'primary' })} type="submit" disabled={saving || !name.trim() || !hosts.trim()}>
      {saving ? 'Saving…' : initial ? 'Save preset' : 'Create preset'}
    </button>
  </div>
</form>
