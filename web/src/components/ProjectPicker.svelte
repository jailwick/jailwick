<script lang="ts">
  import { css } from 'styled-system/css'
  import { button } from '../lib/styles'
  import Icon from './Icon.svelte'

  // A "+" that lists projects, ticking those that already have something (a preset). Clicking a
  // project adds it there, or removes it when ticked; the tick follows once that succeeds.
  let {
    projects,
    selected,
    ontoggle,
    title,
  }: { projects: string[]; selected: string[]; ontoggle: (project: string, add: boolean) => Promise<boolean>; title: string } = $props()

  let open = $state(false)
  let pending = $state('')
  let root: HTMLElement

  async function pick(p: string) {
    pending = p
    try {
      await ontoggle(p, !selected.includes(p))
    } finally {
      pending = ''
    }
  }
  function outside(e: MouseEvent) {
    if (open && !root.contains(e.target as Node)) open = false
  }

  const wrap = css({ position: 'relative', display: 'inline-flex' })
  const menu = css({
    position: 'absolute', right: 0, top: 'calc(100% + 4px)', zIndex: 20, minW: '200px', maxH: '260px', overflowY: 'auto',
    bg: 'bg.panel', border: '1px solid {colors.border.strong}', borderRadius: 'control', boxShadow: 'pop', py: '1',
  })
  const head = css({ px: '3', py: '1.5', fontSize: '2xs', color: 'fg.faint', fontWeight: 'semibold' })
  const item = css({
    display: 'flex', alignItems: 'center', gap: '2', w: '100%', px: '3', h: 'control.md', textAlign: 'left', cursor: 'pointer',
    _hover: { bg: 'bg.subtle' }, _disabled: { opacity: 0.6, cursor: 'default' },
  })
  const tick = css({ w: '14px', display: 'inline-flex', color: 'selected.fg' })
  const none = css({ px: '3', py: '2', color: 'fg.muted' })
</script>

<svelte:window onclick={outside} onkeydown={(e) => e.key === 'Escape' && (open = false)} />

<span class={wrap} bind:this={root}>
  <button class={button({ size: 'sm' })} {title} aria-haspopup="menu" aria-expanded={open}
    onclick={() => (open = !open)}>
    <Icon name="plus" size={12} />Add to…
  </button>
  {#if open}
    <div class={menu} role="menu">
      <div class={head}>{title}</div>
      {#each projects as p (p)}
        <button class={item} role="menuitemcheckbox" aria-checked={selected.includes(p)} disabled={!!pending} onclick={() => pick(p)}>
          <span class={tick}>
            {#if pending === p}<Icon name="spinner" size={13} />{:else if selected.includes(p)}<Icon name="check" size={14} />{/if}
          </span>
          {p}
        </button>
      {:else}
        <div class={none}>No projects yet.</div>
      {/each}
    </div>
  {/if}
</span>
