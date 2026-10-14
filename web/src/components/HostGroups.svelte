<script module lang="ts">
  // One collapsible row: a title (a preset's name, "This project", …), what it is, its hosts.
  export type HostGroup = { key: string; title: string; subtitle?: string; category?: string; hosts: string[] }
</script>

<script lang="ts" generics="G extends HostGroup">
  import { css } from 'styled-system/css'
  import type { Snippet } from 'svelte'
  import { mono, muted, faint, input } from '../lib/styles'
  import Icon from './Icon.svelte'

  // Groups of hosts, collapsed to their titles, under their categories, with a search over
  // titles, descriptions and hosts (matches open, showing the matching hosts). head adds to a
  // row's title line (badges, actions), body to its open part.
  let {
    groups,
    placeholder = 'Search',
    empty = 'Nothing here.',
    head,
    body,
  }: { groups: G[]; placeholder?: string; empty?: string; head?: Snippet<[G]>; body?: Snippet<[G]> } = $props()

  let query = $state('')
  let open = $state<Record<string, boolean>>({})
  const q = $derived(query.trim().toLowerCase())

  // The groups to show, with the hosts to list: all of them, or those matching the search.
  const shown = $derived.by(() => {
    const out: { g: G; hosts: string[] }[] = []
    for (const g of groups) {
      if (!q) {
        out.push({ g, hosts: g.hosts })
        continue
      }
      const named = g.title.toLowerCase().includes(q) || !!g.subtitle?.toLowerCase().includes(q)
      const hosts = named ? g.hosts : g.hosts.filter((h) => h.includes(q))
      if (named || hosts.length) out.push({ g, hosts })
    }
    return out
  })
  // Consecutive groups of one category, in order.
  const sections = $derived.by(() => {
    const out: { category: string; rows: typeof shown }[] = []
    for (const r of shown) {
      const c = r.g.category ?? ''
      if (out.at(-1)?.category !== c) out.push({ category: c, rows: [] })
      out.at(-1)!.rows.push(r)
    }
    return out
  })

  const search = css({
    display: 'flex', alignItems: 'center', gap: '2', px: '3', py: '2.5', borderBottom: '1px solid {colors.border.default}',
    color: 'fg.faint',
  })
  const categoryRow = css({ px: '4', pt: '3', pb: '1', fontSize: '2xs', color: 'fg.faint', fontWeight: 'semibold', textTransform: 'capitalize' })
  const row = css({ borderBottom: '1px solid {colors.border.default}', _last: { borderBottom: 'none' } })
  // The name gives way to the badges and buttons after it, never the other way round.
  const line = css({ display: 'flex', alignItems: 'center', gap: '1.5', pr: '3', _hover: { bg: 'bg.subtle' } })
  const toggle = css({
    display: 'flex', alignItems: 'center', gap: '1.5', flex: '1 0 auto', px: '4', py: '2', fontWeight: 'semibold', textAlign: 'left', cursor: 'pointer',
  })
  const chevron = css({ color: 'fg.faint', transitionProperty: 'transform', transitionDuration: 'fast', flexShrink: 0, display: 'inline-flex' })
  const chevronOpen = css({ transform: 'rotate(90deg)' })
  const bodyC = css({ display: 'grid', gap: '1', px: '4', pb: '2.5', pl: '9' })
  const hostList = css({ display: 'flex', flexWrap: 'wrap', columnGap: '3', rowGap: '0.5', overflowWrap: 'anywhere' })
  const emptyRow = css({ px: '4', py: '4', color: 'fg.muted' })
</script>

<label class={search}>
  <Icon name="search" size={14} />
  <input class={input} style="flex:1" type="search" {placeholder} bind:value={query} aria-label={placeholder} />
  {#if q}<span class={muted} style="font-size:var(--font-sizes-sm);white-space:nowrap">{shown.length} of {groups.length}</span>{/if}
</label>
{#if shown.length === 0}
  <div class={emptyRow}>{q ? 'Nothing matches.' : empty}</div>
{/if}
{#each sections as sec (sec.category + sec.rows[0].g.key)}
  {#if sec.category}<div class={categoryRow}>{sec.category}</div>{/if}
  {#each sec.rows as { g, hosts } (g.key)}
    {@const isOpen = !!q || !!open[g.key]}
    <div class={row}>
      <div class={line}>
        <button class={toggle} aria-expanded={isOpen} onclick={() => (open[g.key] = !open[g.key])}>
          <span class="{chevron} {isOpen ? chevronOpen : ''}"><Icon name="chevron" size={13} /></span>
          {g.title}
        </button>
        {@render head?.(g)}
      </div>
      {#if isOpen}
        <div class={bodyC}>
          {#if g.subtitle}<div class={muted}>{g.subtitle}</div>{/if}
          <div class="{mono} {faint} {hostList}">{#each hosts as h}<span>{h}</span>{/each}</div>
          {@render body?.(g)}
        </div>
      {/if}
    </div>
  {/each}
{/each}
