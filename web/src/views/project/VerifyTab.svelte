<script lang="ts">
  import { css } from 'styled-system/css'
  import type { ProjectDetail, Check } from '../../lib/api'
  import { api } from '../../lib/api'
  import { panel, panelHeader, button, muted, badge } from '../../lib/styles'
  import Icon from '../../components/Icon.svelte'
  import PhaseNote from '../../components/PhaseNote.svelte'
  import type { Phase } from '../../lib/phase'

  let { d, phase = '' }: { d: ProjectDetail; phase?: Phase } = $props()
  let checks = $state<Check[] | null>(null)
  let running = $state(false)
  let error = $state('')

  async function run() {
    running = true
    error = ''
    try {
      checks = await api.verify(d.name)
    } catch (e) {
      error = (e as Error).message
    } finally {
      running = false
    }
  }

  const sections = $derived.by(() => {
    const m = new Map<string, Check[]>()
    for (const c of checks ?? []) m.set(c.section, [...(m.get(c.section) ?? []), c])
    return [...m.entries()]
  })
  const failed = $derived((checks ?? []).filter((c) => !c.ok).length)
  const passed = $derived((checks ?? []).filter((c) => c.ok && !c.info).length)

  const head = css({ display: 'flex', alignItems: 'center', gap: '3', mb: '4' })
  const grid = css({ display: 'grid', gridTemplateColumns: { base: '1fr', xl: '1fr 1fr' }, gap: '4', alignItems: 'start' })
  const row = css({ display: 'flex', gap: '2.5', alignItems: 'flex-start', px: '4', py: '1.5', fontSize: 'md' })
  const ok = css({ color: 'ok', mt: '1px' })
  const bad = css({ color: 'bad', mt: '1px' })
  const detail = css({ color: 'fg.muted', fontSize: 'xs', fontFamily: 'mono', wordBreak: 'break-all' })
  const list = css({ py: '2' })
</script>

<div class={head}>
  <button class={button({ variant: 'primary' })} disabled={running || !d.running || !!phase} onclick={run}>
    <Icon name="network" size={14} />{running ? 'Checking…' : checks ? 'Run again' : 'Run checks'}
  </button>
  {#if checks}
    <span class={badge({ tone: failed ? 'bad' : 'ok' })}>{passed} passed{failed ? ` · ${failed} failed` : ''}</span>
  {:else}
    <span class={muted}>Proves the isolation inside the running sandbox: identity, files, network, tools. Changes nothing.</span>
  {/if}
</div>
{#if phase}<p><PhaseNote {phase} /></p>{:else if !d.running}<p class={muted}>Start the sandbox to verify it.</p>{/if}
{#if error}<p style="color:var(--colors-bad)">{error}</p>{/if}
<div class={grid}>
  {#each sections as [name, cs]}
    <section class={panel}>
      <div class={panelHeader}>{name}</div>
      <div class={list}>
        {#each cs as c}
          <div class={row}>
            <span class={c.ok ? ok : bad}><Icon name={c.ok ? 'check' : 'x'} size={14} /></span>
            <div>
              <div>{c.name}</div>
              {#if !c.ok && c.detail}<div class={detail}>{c.detail}</div>{/if}
            </div>
          </div>
        {/each}
      </div>
    </section>
  {/each}
</div>
