<script lang="ts">
  import { css } from 'styled-system/css'
  import { button } from '../lib/styles'

  // An in-page confirmation. The desktop app's web view doesn't show the browser's confirm(), so
  // pages ask here instead.
  let {
    message,
    action,
    danger = false,
    onconfirm,
    oncancel,
  }: { message: string; action: string; danger?: boolean; onconfirm: () => void; oncancel: () => void } = $props()

  const box = css({
    display: 'flex', alignItems: 'center', gap: '3', flexWrap: 'wrap', mt: '4', px: '3.5', py: '2.5',
    borderRadius: 'panel', bg: 'warnSubtle', fontSize: 'md', whiteSpace: 'pre-line',
    borderWidth: '1px', borderStyle: 'solid', borderColor: 'warn/30',
  })
  const actions = css({ ml: 'auto', display: 'flex', gap: '2' })
</script>

<div class={box} role="alertdialog" aria-label={action}>
  <span>{message}</span>
  <span class={actions}>
    <button class={button({ size: 'sm' })} onclick={oncancel}>Cancel</button>
    <button class={button({ size: 'sm', variant: danger ? 'danger' : 'primary' })} onclick={onconfirm}>{action}</button>
  </span>
</div>
