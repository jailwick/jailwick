<script lang="ts">
  import { css } from 'styled-system/css'
  import { untrack } from 'svelte'
  import { api } from '../../lib/api'
  import { button, input, mono, muted } from '../../lib/styles'
  import { toast } from '../../components/Toasts.svelte'

  // The terminal app sessions open in (config.yaml's terminal): one jailwick knows, or any
  // other through a command with {cmd} in it.
  let { value, onchange }: { value: string; onchange: () => void } = $props()

  const known = [
    { v: '', label: 'Terminal (macOS)' },
    { v: 'iterm2', label: 'iTerm2' },
    { v: 'ghostty', label: 'Ghostty' },
    { v: 'kitty', label: 'kitty' },
    { v: 'wezterm', label: 'WezTerm' },
  ]
  const isKnown = (v: string) => known.some((k) => k.v === v)
  // The form starts from the saved value; the parent reloads it after a save.
  const saved = untrack(() => value)
  let choice = $state(isKnown(saved) ? saved : 'custom')
  let custom = $state(isKnown(saved) ? '' : saved)
  let saving = $state(false)

  async function save(v: string) {
    saving = true
    try {
      await api.saveTerminal(v)
      toast('Sessions now open in ' + (known.find((k) => k.v === v)?.label ?? 'your command'), 'ok')
      onchange()
    } catch (e) {
      toast((e as Error).message, 'bad')
    } finally {
      saving = false
    }
  }
  function pick(v: string) {
    choice = v
    if (v !== 'custom') save(v)
  }

  const row = css({ display: 'flex', gap: '2', alignItems: 'center', flexWrap: 'wrap' })
  const select = css({ h: 'control.icon', px: '2', borderRadius: 'control', border: '1px solid {colors.border.strong}', bg: 'bg.panel' })
</script>

<div style="display:grid;gap:var(--spacing-1\.5)">
  <div class={row}>
    <select class={select} value={choice} disabled={saving} onchange={(e) => pick(e.currentTarget.value)} aria-label="Terminal">
      {#each known as k}<option value={k.v}>{k.label}</option>{/each}
      <option value="custom">Another, by command…</option>
    </select>
  </div>
  {#if choice === 'custom'}
    <div class={row}>
      <input class="{input} {mono}" style="flex:1;min-width:220px" placeholder="alacritty -e sh -c {'{cmd}'}" bind:value={custom} />
      <button class={button({ size: 'sm', variant: 'primary' })} disabled={saving || !custom.includes('{cmd}')} onclick={() => save(custom)}>Save</button>
    </div>
    <span class={muted} style="font-size:var(--font-sizes-sm)">{'{cmd}'} is replaced by the session's command.</span>
  {/if}
</div>
