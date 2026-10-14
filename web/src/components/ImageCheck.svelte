<script lang="ts">
  import { css } from 'styled-system/css'
  import { api, type ImageFinding } from '../lib/api'
  import { button } from '../lib/styles'
  import Icon from './Icon.svelte'

  // Tries an image for jailwick (see api.checkImage) and lists what it found: each thing checked
  // with ✓ or ✗ and why. Pulling a large image the first time takes a while.
  let { ref, user = '', home = '', asIs = false }: { ref: string; user?: string; home?: string; asIs?: boolean } = $props()

  let checking = $state(false)
  let result = $state<{ ok: boolean; findings: ImageFinding[] } | null>(null)
  let error = $state('')
  let checked = $state('') // what the result is about: a change of the inputs makes it stale

  const key = $derived(`${ref}|${user}|${home}|${asIs}`)
  async function check() {
    checking = true
    error = ''
    result = null
    const asked = key
    try {
      result = await api.checkImage({ ref, user, home, as_is: asIs })
    } catch (e) {
      error = (e as Error).message
    } finally {
      checked = asked
      checking = false
    }
  }
  const names: Record<string, string> = { distro: 'System', libc: 'C library', arch: 'Architecture', user: 'User', home: 'Home', claude: 'Claude Code', git: 'git' }

  const row = css({ display: 'flex', alignItems: 'center', gap: '2', flexWrap: 'wrap' })
  const list = css({ display: 'grid', gap: '1', m: 0, p: '2.5', borderRadius: 'control', bg: 'bg.inset', border: '1px solid {colors.border.default}', fontSize: 'md' })
  const item = css({ display: 'grid', gridTemplateColumns: 'auto 7rem 1fr', gap: '2', alignItems: 'baseline' })
  const ok = css({ color: 'ok' })
  const bad = css({ color: 'bad' })
  const name = css({ color: 'fg.muted' })
  const stale = css({ opacity: 0.55 })
</script>

<div style="display:grid;gap:var(--spacing-2)">
  <div class={row}>
    <button class={button({ size: 'sm' })} type="button" disabled={!ref || checking} onclick={check}>
      <Icon name={checking ? 'spinner' : 'check'} size={12} />{checking ? 'Checking… (pulling it the first time)' : 'Check image'}
    </button>
    {#if result && checked === key}
      <span class={result.ok ? ok : bad}>{result.ok ? 'Ready for jailwick' : 'Not usable as it is'}</span>
    {/if}
  </div>
  {#if error && checked === key}<p class={bad} style="font-size:var(--font-sizes-md)">{error}</p>{/if}
  {#if result}
    <ul class="{list} {checked === key ? '' : stale}" aria-label="What the check found">
      {#each result.findings as f (f.name)}
        <li class={item}>
          <span class={f.ok ? ok : bad}><Icon name={f.ok ? 'check' : 'x'} size={12} /></span>
          <span class={name}>{names[f.name] ?? f.name}</span>
          <span>{f.detail}</span>
        </li>
      {/each}
    </ul>
  {/if}
</div>
