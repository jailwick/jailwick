<script lang="ts">
  import { css } from 'styled-system/css'
  import type { Snippet } from 'svelte'
  let { title, subtitle = '', actions, children }: { title: string; subtitle?: string; actions?: Snippet; children: Snippet } = $props()
  const page = css({ h: '100%', overflowY: 'auto' })
  const inner = css({ maxW: 'content.wide', mx: 'auto', px: '7', py: '6' })
  const head = css({ display: 'flex', alignItems: 'flex-end', gap: '4', mb: '5' })
  const h1 = css({ fontSize: '3xl', fontWeight: 'bold', letterSpacing: 'tight' })
  const sub = css({ color: 'fg.muted', mt: '0.5' })
</script>

<div class={page}>
  <div class={inner}>
    <div class={head}>
      <div>
        <h1 class={h1}>{title}</h1>
        {#if subtitle}<p class={sub}>{subtitle}</p>{/if}
      </div>
      {#if actions}<div style="margin-left:auto;display:flex;gap:var(--spacing-2)">{@render actions()}</div>{/if}
    </div>
    {@render children()}
  </div>
</div>
