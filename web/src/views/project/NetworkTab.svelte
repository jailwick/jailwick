<script lang="ts">
  import { css } from 'styled-system/css'
  import type { ProjectDetail, Port, Preset } from '../../lib/api'
  import { api } from '../../lib/api'
  import { panel, panelHeader, table, mono, muted, input, button, link, panelCount as count } from '../../lib/styles'
  import Icon from '../../components/Icon.svelte'
  import { toast } from '../../components/Toasts.svelte'
  import { poll } from '../../lib/poll.svelte'
  import PresetEditor from '../../components/PresetEditor.svelte'
  import HostGroups from '../../components/HostGroups.svelte'
  import { badge } from '../../lib/styles'
  import BlockedRequests from './BlockedRequests.svelte'
  import PhaseNote from '../../components/PhaseNote.svelte'
  import type { Phase } from '../../lib/phase'

  let { d, phase = '', onchange }: { d: ProjectDetail; phase?: Phase; onchange?: () => void } = $props()
  // A domain, or the name of a preset (suggested as you type).
  let entry = $state('')
  let creating = $state(false)
  let catalogue = $state<Preset[]>([])
  const loadCatalogue = () => api.presets().then((r) => (catalogue = r.presets)).catch(() => {})
  loadCatalogue()

  // d is replaced at every refresh of the page: depend on what changes, not on d itself.
  const name = $derived(d.name)
  const running = $derived(d.running)
  // The listening ports, kept fresh one request at a time while the sandbox runs.
  let portsPoll = $state.raw<ReturnType<typeof poll<Port[]>> | null>(null)
  $effect(() => {
    if (!running) return
    const p = poll(() => api.ports(name), 5000)
    portsPoll = p
    return () => p.stop()
  })
  const ports = $derived(portsPoll?.state.data ?? (portsPoll?.state.error ? [] : null))
  const projectPresets = $derived(d.config.allow_presets ?? [])
  const always = $derived(catalogue.filter((p) => p.always).map((p) => p.name))
  // Where the allowlist comes from, as collapsible rows.
  const titles: Record<string, string> = {
    project: 'This project', user: 'Your allowlist (all projects)', global: 'config.yaml (all projects)', chrome: 'Claude in Chrome',
  }
  const groups = $derived(
    d.allow_groups.map((g) => ({
      key: g.kind + ':' + g.name, title: g.kind === 'preset' ? g.name : titles[g.kind], hosts: g.hosts,
      subtitle: g.kind === 'preset' ? catalogue.find((p) => p.name === g.name)?.description : undefined,
      category: g.kind === 'preset' ? 'presets' : 'domains', group: g,
    })),
  )
  // Presets the project can still add, for the suggestions.
  const suggestions = $derived(catalogue.filter((p) => !p.always && !projectPresets.includes(p.name)))

  async function run(fn: () => Promise<unknown>, done: string) {
    try {
      await fn()
      toast(done, 'ok')
      onchange?.()
      return true
    } catch (err) {
      toast((err as Error).message, 'bad')
      return false
    }
  }

  async function add(e: SubmitEvent) {
    e.preventDefault()
    const v = entry.trim()
    if (!v) return
    const ok = catalogue.some((p) => p.name === v)
      ? await run(() => api.allowPreset(v, name), `The ${v} preset is allowed for ${name}`)
      : await run(() => api.allow(v, name), `${v} allowed for ${name}`)
    if (ok) entry = ''
  }

  function removePreset(preset: string) {
    run(() => api.denyPreset(preset, name), `The ${preset} preset is removed from ${name}`)
  }

  async function created(preset: string) {
    creating = false
    await loadCatalogue()
    run(() => api.allowPreset(preset, name), `The ${preset} preset is allowed for ${name}`)
  }

  const grid = css({ display: 'grid', gridTemplateColumns: { base: '1fr', xl: '1fr 1fr' }, gap: '5', alignItems: 'start' })
  const form = css({ display: 'flex', gap: '2', p: '3', borderBottom: '1px solid {colors.border.default}' })

  const emptyRow = css({ px: '4', py: '4', color: 'fg.muted' })
  const full = css({ gridColumn: '1 / -1' })
</script>

<div class={grid}>
  <div class={full}><BlockedRequests {name} {running} {onchange} /></div>

  <section class={panel}>
    <div class={panelHeader}><Icon name="external" size={14} />Listening ports</div>
    {#if phase}
      <div class={emptyRow}><PhaseNote {phase} /></div>
    {:else if !d.running}
      <div class={emptyRow}>Not running.</div>
    {:else if ports === null}
      <div class={emptyRow}>Loading…</div>
    {:else if ports.length === 0}
      <div class={emptyRow}>Nothing is listening.</div>
    {:else}
      <table class={table}>
        <thead><tr><th>Port</th><th>Where</th><th>Command</th></tr></thead>
        <tbody>
          {#each ports as p ((p.sid ?? '') + p.port + p.where)}
            <tr>
              <td style="font-variant-numeric:tabular-nums">
                {#if p.url}
                  <a class={link} href={p.url} target="_blank" rel="noreferrer">{p.port}</a>
                {:else}{p.port}{/if}
              </td>
              <td>{p.where || '–'}</td>
              <td class="{mono} {muted}">{p.command ?? ''}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}
  </section>

  <section class={panel}>
    <div class={panelHeader}><Icon name="network" size={14} />Allowlist<span class={count}>{d.allow.length} hosts</span></div>
    <form class={form} onsubmit={add}>
      <input class={input} style="flex:1" list="presets-{name}" placeholder="Add a preset (npm, pypi…) or a host (api.example.com)"
        aria-label="Add a preset or a host to this project" bind:value={entry} autocomplete="off" />
      <datalist id="presets-{name}">
        {#each suggestions as p (p.name)}<option value={p.name}>{p.description || p.hosts.join(' ')}</option>{/each}
      </datalist>
      <button class={button({ variant: 'primary' })} type="submit"><Icon name="plus" size={13} />Allow</button>
      <button class={button()} type="button" title="A preset of your own, for any project" onclick={() => (creating = !creating)}>
        <Icon name="plus" size={13} />New preset
      </button>
    </form>
    {#if creating}
      <PresetEditor onsaved={created} oncancel={() => (creating = false)} />
    {/if}
    <HostGroups {groups} placeholder="Find in this allowlist">
      {#snippet head(g)}
        {#if g.group.kind === 'preset' && always.includes(g.group.name)}
          <span class={badge()} title="Always allowed: every sandbox needs it">always</span>
        {:else if g.group.kind === 'preset'}
          <button class={button({ size: 'icon', variant: 'ghost' })} title="Remove the {g.group.name} preset" aria-label="Remove the {g.group.name} preset"
            onclick={() => removePreset(g.group.name)}><Icon name="x" size={13} /></button>
        {/if}
      {/snippet}
    </HostGroups>
  </section>
</div>
