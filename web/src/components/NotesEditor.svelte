<script lang="ts">
  import { css } from 'styled-system/css'
  import { onMount } from 'svelte'
  import type { Notes } from '../lib/api'
  import { button, muted, mono, panel } from '../lib/styles'
  import { toast } from './Toasts.svelte'

  // A notes.md, edited in place: the project's or the workspace's. The parent keys it by what it
  // edits; loading again would overwrite what is being typed.
  let {
    load,
    save,
    help,
    placeholder = '',
  }: { load: () => Promise<Notes>; save: (text: string) => Promise<unknown>; help: string; placeholder?: string } = $props()

  let text = $state('')
  let saved = $state('')
  let path = $state('')
  let saving = $state(false)

  onMount(() => {
    load().then((n) => {
      text = saved = n.text
      path = n.path
    })
  })

  async function store() {
    saving = true
    try {
      await save(text)
      saved = text
      toast('Notes saved; the next session reads them', 'ok')
    } catch (e) {
      toast((e as Error).message, 'bad')
    } finally {
      saving = false
    }
  }

  const bar = css({ display: 'flex', alignItems: 'center', gap: '3', mb: '3', flexWrap: 'wrap' })
  const area = css({
    w: '100%', h: 'calc(100vh - 300px)', minH: '260px', p: '4', border: 'none', resize: 'none', outline: 'none',
    fontFamily: 'mono', fontSize: 'md', lineHeight: 'code', bg: 'bg.panel', color: 'fg.default',
  })
</script>

<div class={bar}>
  <button class={button({ variant: 'primary' })} disabled={saving || text === saved} onclick={store}>Save</button>
  <span class={muted}>{help}</span>
  <span class="{muted} {mono}" style="margin-left:auto">{path}</span>
</div>
<div class={panel}>
  <textarea class={area} bind:value={text} {placeholder}></textarea>
</div>
