<script lang="ts" module>
  export type Toast = { id: number; text: string; tone: 'ok' | 'bad' | 'neutral' }
  export const toasts = $state<Toast[]>([])
  let next = 1
  export function toast(text: string, tone: Toast['tone'] = 'neutral') {
    const id = next++
    toasts.push({ id, text, tone })
    setTimeout(() => {
      const i = toasts.findIndex((t) => t.id === id)
      if (i >= 0) toasts.splice(i, 1)
    }, tone === 'bad' ? 7000 : 3500)
  }
</script>

<script lang="ts">
  import { css } from 'styled-system/css'
  import Icon from './Icon.svelte'
  const wrap = css({ position: 'fixed', bottom: '4', right: '4', display: 'flex', flexDir: 'column', gap: '2', zIndex: 50 })
  const item = css({
    display: 'flex', alignItems: 'center', gap: '2', maxW: '420px', px: '3.5', py: '2.5',
    bg: 'bg.panel', color: 'fg.default', borderRadius: 'panel', boxShadow: 'pop', border: '1px solid {colors.border.default}', fontSize: 'md',
  })
  const tone = { ok: css({ color: 'ok' }), bad: css({ color: 'bad' }), neutral: css({ color: 'fg.muted' }) }
</script>

<div class={wrap}>
  {#each toasts as t (t.id)}
    <div class={item}>
      <span class={tone[t.tone]}><Icon name={t.tone === 'bad' ? 'alert' : t.tone === 'ok' ? 'check' : 'flame'} size={15} /></span>
      <span>{t.text}</span>
    </div>
  {/each}
</div>
