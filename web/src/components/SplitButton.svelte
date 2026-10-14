<script lang="ts">
  import { css } from 'styled-system/css'
  import type { Snippet } from 'svelte'
  import { button } from '../lib/styles'
  import Menu, { type MenuItem } from './Menu.svelte'

  // A button with an arrow beside it for related actions (Allow all ▾ Allow all everywhere).
  let {
    children,
    onclick,
    items,
    more,
    disabled = false,
    title = '',
  }: { children: Snippet; onclick: () => void; items: MenuItem[]; more: string; disabled?: boolean; title?: string } = $props()

  const wrap = css({ display: 'inline-flex' })
  const main = css({ borderTopRightRadius: '0!', borderBottomRightRadius: '0!' })
  const arrow = css({ borderTopLeftRadius: '0!', borderBottomLeftRadius: '0!', ml: '-1px', px: '1.5!' })
</script>

<span class={wrap}>
  <button class="{button({ size: 'sm' })} {main}" {disabled} {title} {onclick}>{@render children()}</button>
  <Menu {items} {disabled} title={more} class="{button({ size: 'sm' })} {arrow}" />
</span>
