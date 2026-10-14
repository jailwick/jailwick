<script lang="ts">
  import { css } from 'styled-system/css'
  import { copyText } from '../lib/clipboard'
  import { button } from '../lib/styles'
  import Icon from './Icon.svelte'
  import { toast } from './Toasts.svelte'

  // A command to run in a terminal, shown in full, with a Copy button: for terminals the app
  // can't open by itself.
  let { command, label = 'Copy' }: { command: string; label?: string } = $props()

  let copied = $state(false)
  async function copy() {
    if (await copyText(command)) {
      copied = true
      setTimeout(() => (copied = false), 1500)
    } else {
      toast('Copying failed: select the command and copy it', 'bad')
    }
  }

  const row = css({ display: 'flex', alignItems: 'center', gap: '2', minW: 0 })
  const code = css({
    flex: 1, minW: 0, px: '2', py: '1', borderRadius: 'control', bg: 'bg.inset', border: '1px solid {colors.border.default}',
    // Cut with an ellipsis rather than mid-word; the tooltip and Copy have the whole command.
    fontFamily: 'mono', fontSize: 'xs', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis', userSelect: 'all',
  })
</script>

<div class={row}>
  <code class={code} title={command}>{command}</code>
  <button class={button({ size: 'sm' })} onclick={copy} aria-label="Copy the command">
    <Icon name={copied ? 'check' : 'copy'} size={12} />{copied ? 'Copied' : label}
  </button>
</div>
