<script lang="ts">
  import { css } from 'styled-system/css'
  import type { SetupField, SetupStep } from '../lib/api'
  import type { SetupForm } from '../lib/setupForm.svelte'
  import { link, muted } from '../lib/styles'

  // The answers of a setup in words, step by step: each field that applies with its value
  // (passwords hidden), and a way back to the step to change it.
  let { form, steps, onedit }: { form: SetupForm; steps: SetupStep[]; onedit?: (step: number) => void } = $props()

  function shown(f: SetupField): { text: string; empty: boolean } {
    const v = form.valueOf(f)
    if (Array.isArray(v)) return v.length ? { text: v.join(', '), empty: false } : { text: 'none', empty: true }
    // Empty takes a default where the field says so ("Empty: 24." or "Default: app.").
    const def = f.help?.match(/^(?:Empty|Default): ([^.]+)\./)?.[1]
    if (!v) return { text: def ? `default (${def})` : 'not set', empty: true }
    if (f.kind === 'choice') return { text: f.options?.find((o) => o.value === v)?.label || v, empty: false }
    return { text: f.kind === 'secret' ? '••••••' : v, empty: false }
  }

  const list = css({ display: 'grid', gap: '4' })
  const head = css({ display: 'flex', alignItems: 'baseline', gap: '3', pb: '1.5', mb: '1.5', borderBottom: '1px solid {colors.border.default}' })
  const title = css({ fontWeight: 'semibold' })
  const kv = css({ display: 'grid', gridTemplateColumns: '{sizes.label} 1fr', rowGap: '1', columnGap: '4', fontSize: 'md', m: 0, '& dt': { color: 'fg.muted' } })
  const value = css({ overflowWrap: 'anywhere' })
</script>

<div class={list}>
  {#each steps as s, i (s.id)}
    {@const fields = s.fields.filter((f) => form.visible(f))}
    {#if fields.length}
      <section>
        <div class={head}>
          <span class={title}>{s.title}</span>
          {#if onedit}<button class={link} style="margin-left:auto;font-size:var(--font-sizes-sm)" onclick={() => onedit(i)}>Edit</button>{/if}
        </div>
        <dl class={kv}>
          {#each fields as f (f.key)}
            {@const v = shown(f)}
            <dt>{f.label}</dt>
            <dd class="{value} {v.empty ? muted : ''}">{v.text}</dd>
          {/each}
        </dl>
      </section>
    {/if}
  {/each}
</div>
