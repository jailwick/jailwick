<script lang="ts" module>
  export type MenuItem = { label: string; hint?: string; danger?: boolean; run: () => void } | 'separator'
</script>

<script lang="ts">
  import { css, cva } from 'styled-system/css'
  import { untrack, type Snippet } from 'svelte'
  import { button } from '../lib/styles'
  import Icon from './Icon.svelte'

  // A button that opens a short list of actions under it. Esc, a click outside or choosing one
  // closes it. The list is fixed to the window, so a table cell or panel doesn't cut it off.
  // Without a label the button is the arrow alone (title names it), for a split button; class
  // replaces the button's look.
  let {
    label,
    items,
    disabled = false,
    title = '',
    class: cls = '',
  }: { label?: Snippet; items: MenuItem[]; disabled?: boolean; title?: string; class?: string } = $props()

  let open = $state(false)
  let trigger: HTMLButtonElement | undefined = $state()
  let menu: HTMLDivElement | undefined = $state()
  let pos = $state({ top: 0, left: 0 })

  function toggle(e: MouseEvent) {
    e.stopPropagation() // rows may be links
    if (!open && trigger) {
      const r = trigger.getBoundingClientRect()
      pos = { top: r.bottom + 4, left: r.left }
    }
    open = !open
  }
  function choose(e: MouseEvent, run: () => void) {
    e.stopPropagation()
    open = false
    run()
  }
  // Keep the list inside the window once it has a size.
  // (pos is read untracked: the list's size is what moves it, not where it was put.)
  $effect(() => {
    if (!open || !menu) return
    const r = menu.getBoundingClientRect()
    untrack(() => {
      let { top, left } = pos
      if (r.right > window.innerWidth - 8) left = Math.max(8, window.innerWidth - 8 - r.width)
      if (r.bottom > window.innerHeight - 8 && trigger) top = trigger.getBoundingClientRect().top - 4 - r.height
      if (top !== pos.top || left !== pos.left) pos = { top, left }
    })
  })
  $effect(() => {
    if (!open) return
    const outside = (e: Event) => {
      if (!menu?.contains(e.target as Node) && !trigger?.contains(e.target as Node)) open = false
    }
    const key = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        open = false
        trigger?.focus()
      }
    }
    // A scroll moves the button (a click on it can scroll a table that is too wide to show it
    // whole): the list follows it, and closes once the button is out of sight.
    const away = () => {
      const r = trigger?.getBoundingClientRect()
      if (!r || r.bottom < 0 || r.top > window.innerHeight) {
        open = false
        return
      }
      const w = menu?.getBoundingClientRect().width ?? 0
      pos = { top: r.bottom + 4, left: Math.max(8, Math.min(r.left, window.innerWidth - 8 - w)) }
    }
    document.addEventListener('pointerdown', outside, true)
    document.addEventListener('keydown', key)
    const resized = () => (open = false)
    window.addEventListener('scroll', away, true)
    window.addEventListener('resize', resized)
    return () => {
      document.removeEventListener('pointerdown', outside, true)
      document.removeEventListener('keydown', key)
      window.removeEventListener('scroll', away, true)
      window.removeEventListener('resize', resized)
    }
  })

  const list = css({
    position: 'fixed', zIndex: 50, minW: '180px', py: '1', bg: 'bg.panel', borderRadius: 'control', boxShadow: 'pop',
    border: '1px solid {colors.border.default}', display: 'grid',
  })
  // A variant, so a danger item has one colour class only.
  const itemC = cva({
    base: {
      display: 'flex', alignItems: 'baseline', gap: '3', px: '3', py: '1.5', textAlign: 'left', fontSize: 'md', cursor: 'pointer',
      whiteSpace: 'nowrap', _hover: { bg: 'bg.hover' }, _focusVisible: { bg: 'bg.hover', outline: 'none' },
    },
    variants: { danger: { true: { color: 'bad' }, false: { color: 'fg.default' } } },
  })
  const hintC = css({ ml: 'auto', color: 'fg.faint', fontSize: 'xs' })
  const sepC = css({ h: '1px', my: '1', bg: 'border.default' })
</script>

<button bind:this={trigger} class={cls || button({ size: 'sm' })} {disabled} {title} aria-label={label ? undefined : title}
  aria-haspopup="menu" aria-expanded={open} onclick={toggle}>
  {@render label?.()}<Icon name="chevronDown" size={11} />
</button>
{#if open}
  <div bind:this={menu} class={list} role="menu" style="top:{pos.top}px;left:{pos.left}px">
    {#each items as it}
      {#if it === 'separator'}
        <div class={sepC} role="separator"></div>
      {:else}
        <button class={itemC({ danger: !!it.danger })} role="menuitem" onclick={(e) => choose(e, it.run)}>
          {it.label}{#if it.hint}<span class={hintC}>{it.hint}</span>{/if}
        </button>
      {/if}
    {/each}
  </div>
{/if}
