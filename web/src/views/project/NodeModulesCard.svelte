<script lang="ts">
  import { css } from 'styled-system/css'
  import { untrack } from 'svelte'
  import { api, type ProjectDetail } from '../../lib/api'
  import { badge, button, input, mono, muted, panel, panelHeader, panelCount as count } from '../../lib/styles'
  import Icon from '../../components/Icon.svelte'
  import { toast } from '../../components/Toasts.svelte'

  // Which node_modules the sandbox keeps apart from the Mac's: packages with native parts only
  // run on the system that installed them, so each side has its own. Found automatically (every
  // folder with a package.json, 4 levels deep) or chosen here; a restart applies a change.
  let { d, onchange }: { d: ProjectDetail; onchange: () => void } = $props()

  const nm = $derived(d.node_modules)
  let editing = $state(false)
  let auto = $state(true)
  let list = $state('')
  let saving = $state(false)

  function edit() {
    untrack(() => {
      auto = nm.auto
      list = (nm.auto ? nm.dirs : nm.setting).join('\n')
    })
    editing = true
  }
  async function save() {
    saving = true
    try {
      const dirs = auto ? [] : list.split('\n').map((s) => s.trim()).filter(Boolean)
      if (!auto && dirs.length === 0) {
        toast('List at least one folder, or choose automatic', 'bad')
        return
      }
      await api.saveNodeModules(d.name, dirs)
      toast('Saved: a restart applies it', 'ok')
      editing = false
      onchange()
    } catch (e) {
      toast((e as Error).message, 'bad')
    } finally {
      saving = false
    }
  }

  const body = css({ px: '4', py: '3', display: 'grid', gap: '2.5', fontSize: 'base' })
  const rows = css({ display: 'grid', gap: '1' })
  const row = css({ display: 'flex', gap: '2', alignItems: 'center', flexWrap: 'wrap' })
  const area = css({ h: 'auto', py: '1.5', fontFamily: 'mono', fontSize: 'sm', resize: 'vertical' })
  const choice = css({ display: 'flex', gap: '2', alignItems: 'baseline', cursor: 'pointer' })
</script>

<section class={panel}>
  <div class={panelHeader}>
    <Icon name="folder" size={14} />Separate node_modules
    <span class={count}>{nm.auto ? 'found automatically' : 'chosen'}</span>
  </div>
  <div class={body}>
    <p class={muted}>
      The sandbox keeps its own packages, so native ones (esbuild, sharp…) work on both sides; your Mac's stay as they are.
    </p>
    {#if !editing}
      <div class={rows}>
        {#each ['.', ...nm.dirs] as dir (dir)}
          <div class={row}>
            <span class={mono}>{dir === '.' ? 'node_modules' : dir + '/node_modules'}</span>
            {#if nm.empty.includes(dir)}
              <span class={badge()} title="Run npm ci (or your package manager) there in a Shell">nothing installed yet</span>
            {/if}
          </div>
        {/each}
      </div>
      <div><button class={button({ size: 'sm' })} onclick={edit}>Change</button></div>
    {:else}
      <label class={choice}><input type="radio" checked={auto} onchange={() => (auto = true)} />
        <span>Find them automatically <span class={muted}>(every folder with a package.json, 4 levels deep)</span></span></label>
      <label class={choice}><input type="radio" checked={!auto} onchange={() => (auto = false)} />
        <span>Only these folders</span></label>
      {#if !auto}
        <textarea class="{input} {area}" rows={Math.max(3, list.split('\n').length + 1)} bind:value={list}
          placeholder={'web\napps/*\n(one per line; patterns like apps/* work)'}></textarea>
      {/if}
      <span class={muted} style="font-size:var(--font-sizes-sm)">The repo's own node_modules is always separate. A restart applies a change.</span>
      <div class={row}>
        <button class={button({ size: 'sm', variant: 'primary' })} disabled={saving} onclick={save}>Save</button>
        <button class={button({ size: 'sm' })} disabled={saving} onclick={() => (editing = false)}>Cancel</button>
      </div>
    {/if}
  </div>
</section>
