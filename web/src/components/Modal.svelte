<script lang="ts">
  import { css, cva } from 'styled-system/css'
  import type { Snippet } from 'svelte'
  import { button } from '../lib/styles'
  import Icon from './Icon.svelte'

  // A modal on the native <dialog>: Esc and a click outside close it, and focus stays inside while
  // it's open. The parent owns `open`.
  let {
    open = $bindable(false),
    title,
    wide = false,
    large = false,
    children,
  }: { open?: boolean; title: string; wide?: boolean; large?: boolean; children: Snippet } = $props()

  let dialog: HTMLDialogElement | undefined = $state()

  $effect(() => {
    if (!dialog) return
    if (open && !dialog.open) dialog.showModal()
    else if (!open && dialog.open) dialog.close()
  })

  // One width per size (a variant, so only one width class applies). large: most of the window,
  // for content that needs the room (a diff).
  const box = cva({
    base: {
      maxH: 'calc(100vh - 64px)', m: 'auto', p: 0, border: 'none', borderRadius: 'panel',
      bg: 'bg.panel', color: 'fg.default', boxShadow: 'dialog',
      // A dialog opened from a table cell would inherit its alignment and wrapping.
      textAlign: 'left', whiteSpace: 'normal', fontWeight: 'normal',
      '&::backdrop': { bg: 'backdrop' },
    },
    variants: {
      size: {
        md: { w: 'min({sizes.dialog.md}, calc(100vw - 32px))' },
        wide: { w: 'min({sizes.dialog.wide}, calc(100vw - 32px))' },
        large: { w: 'calc(100vw - 64px)', h: 'calc(100vh - 64px)', '& > div': { h: '100%' } },
      },
    },
  })
  const inner = css({ display: 'flex', flexDir: 'column', maxH: 'calc(100vh - 64px)' })
  const head = css({ display: 'flex', alignItems: 'center', gap: '2', px: '4', py: '3', borderBottom: '1px solid {colors.border.default}' })
  const h = css({ fontSize: 'lg', fontWeight: 'bold' })
  const body = css({ p: '4', overflowY: 'auto', display: 'grid', gap: '3' })
</script>

<dialog
  bind:this={dialog}
  class={box({ size: large ? 'large' : wide ? 'wide' : 'md' })}
  aria-label={title}
  onclose={() => (open = false)}
  onclick={(e) => e.target === dialog && dialog.close()}
>
  {#if open}
    <div class={inner}>
      <header class={head}>
        <h2 class={h}>{title}</h2>
        <button class={button({ variant: 'ghost', size: 'icon' })} style="margin-left:auto" aria-label="Close" onclick={() => dialog?.close()}>
          <Icon name="x" size={14} />
        </button>
      </header>
      <div class={body}>{@render children()}</div>
    </div>
  {/if}
</dialog>
