<script lang="ts">
  import { css } from 'styled-system/css'
  import type { Dashboard } from '../lib/api'
  import { go } from '../lib/router.svelte'
  import Dot from '../components/Dot.svelte'
  import { phaseOf } from '../lib/phase'
  import Icon from '../components/Icon.svelte'
  import ProjectDetail from './ProjectDetail.svelte'
  import { badge, button, muted } from '../lib/styles'

  let { overview, error, selected, tab }: { overview?: Dashboard; error: string; selected?: string; tab?: string } = $props()

  const projects = $derived(overview?.projects ?? [])
  // Without one in the route: the project last viewed, else the first running, else the first.
  const lastKey = 'jailwick.lastProject'
  function lastViewed() {
    try {
      return localStorage.getItem(lastKey) ?? ''
    } catch {
      return ''
    }
  }
  const current = $derived(
    selected ?? projects.find((p) => p.name === lastViewed())?.name ?? projects.find((p) => p.running)?.name ?? projects[0]?.name,
  )
  $effect(() => {
    if (!selected) return
    try {
      localStorage.setItem(lastKey, selected)
    } catch {
      // private window: the first running project it is
    }
  })

  const layout = css({ display: 'grid', gridTemplateColumns: '{sizes.projectList} 1fr', h: '100%', minH: 0 })
  const list = css({ borderRight: '1px solid {colors.border.default}', overflowY: 'auto', bg: 'bg.inset' })
  const head = css({
    display: 'flex', alignItems: 'center', px: '4', pt: '3', pb: '2',
    position: 'sticky', top: 0, bg: 'bg.inset', zIndex: 1,
  })
  const title = css({ fontSize: 'xl', fontWeight: 'bold' })
  const row = css({
    display: 'flex', alignItems: 'center', gap: '2.5', w: 'calc(100% - 12px)', mx: '1.5', px: '2.5', py: '2',
    borderRadius: 'control', cursor: 'pointer', textAlign: 'left', _hover: { bg: 'bg.hover' },
  })
  const sel = css({ bg: 'selected.bg!' })
  const name = css({ fontWeight: 'semibold', fontSize: 'base', lineHeight: 'tight' })
  const path = css({ fontSize: 'xs', color: 'fg.faint', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' })
  const meta = css({ minW: 0, flex: 1 })
  const empty = css({ display: 'grid', placeItems: 'center', h: '100%', textAlign: 'center', p: '8', color: 'fg.muted' })
  const code = css({ fontFamily: 'mono', bg: 'bg.subtle', px: '1.5', py: '0.5', borderRadius: 'sm', color: 'fg.default' })
  const failedMark = css({ display: 'inline-flex', color: 'bad', flexShrink: 0 })
  const groupLabel = css({ px: '4', pt: '3', pb: '1', fontSize: '2xs', color: 'fg.faint', fontWeight: 'semibold', textTransform: 'uppercase', letterSpacing: 'caps' })

  // Until the runtime answers, nothing is known to run or not: one group, without a label.
  const groups = $derived.by(() => {
    if (!overview?.live) return [{ label: '', items: projects }]
    const running = projects.filter((p) => p.running)
    const stopped = projects.filter((p) => !p.running)
    return [
      { label: 'Running', items: running },
      { label: 'Stopped', items: stopped },
    ].filter((g) => g.items.length)
  })
</script>

{#if !overview && error}
  <div class={empty}>
    <div>
      <Icon name="alert" size={28} />
      <p>Can't reach jailwick: {error}</p>
    </div>
  </div>
{:else if overview && projects.length === 0}
  <div class={empty}>
    <div>
      <p style="font-size:var(--font-sizes-xl);font-weight:var(--font-weights-semibold);margin-bottom:var(--spacing-1\.5)">No projects yet</p>
      <p style="margin-bottom:var(--spacing-3\.5)">Add a repo here, or run <span class={code}>jw project add</span> in it.</p>
      <button class={button({ variant: 'primary' })} onclick={() => go('new-project')}><Icon name="plus" size={13} />New project</button>
    </div>
  </div>
{:else}
  <div class={layout}>
    <nav class={list} aria-label="Projects">
      <div class={head}>
        <span class={title}>Projects</span>
        <span class={muted} style="font-size:var(--font-sizes-xs);margin-left:auto;margin-right:var(--spacing-2)">{overview?.live ? `${projects.filter((p) => p.running).length} running` : ''}</span>
        <button class={button({ size: 'icon', variant: 'ghost' })} title="New project" aria-label="New project" onclick={() => go('new-project')}><Icon name="plus" size={14} /></button>
      </div>
      {#each groups as g}
        {#if g.label}<div class={groupLabel}>{g.label}</div>{/if}
        {#each g.items as p (p.name)}
          <button class="{row} {p.name === current ? sel : ''}" onclick={() => go('projects', p.name)}>
            <Dot on={p.running} busy={p.busy > 0} unknown={!overview?.live} phase={phaseOf(p)} problems={p.health?.problems?.length ?? 0} />
            <span class={meta}>
              <div class="{name} pname">{p.name}</div>
              <div class={path}>{p.dir}</div>
            </span>
            {#if p.build_failed}<span class={failedMark} title="Setting up the sandbox failed: the project page has the output"><Icon name="alert" size={14} /></span>{/if}
            {#if p.sessions}
              <span class={badge({ tone: p.busy ? 'info' : 'neutral' })}
                title="{p.sessions} session{p.sessions === 1 ? '' : 's'}{p.busy ? `, ${p.busy} busy` : ''}">{p.sessions}</span>
            {/if}
          </button>
        {/each}
      {/each}
    </nav>
    {#if current}
      {#key current}
        <ProjectDetail name={current} tab={tab ?? 'overview'} urls={overview?.runtime?.urls ?? false} />
      {/key}
    {/if}
  </div>
{/if}
