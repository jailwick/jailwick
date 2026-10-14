<script lang="ts">
  import { css, cva } from 'styled-system/css'
  import { api, type Changes } from '../../lib/api'
  import { button, mono, muted } from '../../lib/styles'
  import Icon from '../../components/Icon.svelte'
  import Modal from '../../components/Modal.svelte'

  // What the main checkout (worktree "") or a worktree changed, for review: its commits, its
  // files, and a file's diff. The sandbox computes it (git never runs on the host for this).
  let { project, worktree = '', disabled = false }: { project: string; worktree?: string; disabled?: boolean } = $props()

  let open = $state(false)
  let ch = $state<Changes | null>(null)
  let file = $state('')
  let diff = $state<{ text: string; cut: boolean } | null>(null)
  let error = $state('')

  async function show() {
    open = true
    ch = null
    diff = null
    file = ''
    error = ''
    try {
      ch = await api.changes(project, worktree)
      if (ch.files.length) await pick(ch.files[0].path)
    } catch (e) {
      error = (e as Error).message
    }
  }
  async function pick(path: string) {
    file = path
    diff = null
    try {
      const c = await api.changes(project, worktree, path)
      diff = { text: c.diff ?? '', cut: !!c.diff_cut }
    } catch (e) {
      diff = { text: (e as Error).message, cut: false }
    }
  }

  const totals = $derived(ch ? ch.files.reduce((t, f) => ({ add: t.add + f.added, del: t.del + f.deleted }), { add: 0, del: 0 }) : null)
  const lines = $derived(diff ? diff.text.split('\n') : [])
  const kind = (l: string) => (l.startsWith('+++') || l.startsWith('---') ? 'meta' : l[0] === '+' ? 'add' : l[0] === '-' ? 'del' : l.startsWith('@@') ? 'hunk' : 'ctx')

  const layout = css({ display: 'grid', gridTemplateColumns: { base: '1fr', lg: 'minmax(240px, 22%) 1fr' }, gap: '4', minH: 0 })
  const list = css({ display: 'grid', alignContent: 'start', gap: '0.5', maxH: 'calc(100vh - 280px)', overflowY: 'auto' })
  const item = css({
    display: 'grid', gridTemplateColumns: '1fr auto', gap: '2', alignItems: 'baseline', textAlign: 'left', px: '2', py: '1',
    borderRadius: 'control', cursor: 'pointer', fontSize: 'md', _hover: { bg: 'bg.hover' },
  })
  const itemOn = css({ bg: 'selected.bg', fontWeight: 'semibold', _hover: { bg: 'selected.bg' } })
  const path = css({ fontFamily: 'mono', fontSize: 'xs', wordBreak: 'break-all' })
  const counts = css({ fontFamily: 'mono', fontSize: '2xs', whiteSpace: 'nowrap' })
  const add = css({ color: 'ok' })
  const del = css({ color: 'bad' })
  const pane = css({
    m: 0, h: 'calc(100vh - 280px)', minH: '240px', overflow: 'auto', borderRadius: 'control', border: '1px solid {colors.border.default}', bg: 'bg.inset',
    fontFamily: 'mono', fontSize: 'xs', lineHeight: 'relaxed',
  })
  const line = cva({
    base: { px: '3', whiteSpace: 'pre', minW: 'fit-content' },
    variants: {
      kind: {
        add: { bg: 'okSubtle' }, del: { bg: 'badSubtle' }, hunk: { color: 'info', bg: 'infoSubtle' },
        meta: { color: 'fg.muted', fontWeight: 'semibold' }, ctx: {},
      },
    },
  })
  const commits = css({ m: 0, pl: '4', display: 'grid', gap: '0.5', fontSize: 'md' })
</script>

<button class={button({ size: 'sm' })} {disabled} onclick={show}><Icon name="branch" size={12} />Changes</button>

<Modal bind:open title="{worktree || 'Main checkout'}: changes" large>
  {#if error}
    <p class={muted}>{error}</p>
  {:else if !ch}
    <p class={muted}>Loading…</p>
  {:else}
    <p class={muted} style="font-size:var(--font-sizes-md)">
      {#if ch.base}<span class={mono}>{ch.head}</span> against <span class={mono}>{ch.base}</span>, from where they part: its commits and
        uncommitted edits.{:else}Uncommitted edits in <span class={mono}>{ch.head}</span>.{/if}
      {ch.files.length} file{ch.files.length === 1 ? '' : 's'}{#if totals}, <span class={add}>+{totals.add}</span> <span class={del}>−{totals.del}</span>{/if}
    </p>
    {#if ch.commits?.length}
      <ul class={commits}>{#each ch.commits as c}<li><span class={mono}>{c.hash}</span> {c.subject}</li>{/each}</ul>
    {/if}
    {#if ch.files.length === 0}
      <p class={muted}>Nothing changed.</p>
    {:else}
      <div class={layout}>
        <div class={list}>
          {#each ch.files as f (f.path)}
            <button class="{item} {f.path === file ? itemOn : ''}" onclick={() => pick(f.path)} title="{f.path} ({f.status})">
              <span class={path}>{f.path}</span>
              <span class={counts}>
                {#if f.status === 'untracked'}<span class={muted}>new</span>{:else if f.binary}<span class={muted}>binary</span>{:else}<span class={add}>+{f.added}</span> <span class={del}>−{f.deleted}</span>{/if}
              </span>
            </button>
          {/each}
        </div>
        <div class={pane}>
          {#if !diff}
            <div class={line({ kind: 'meta' })}>Loading…</div>
          {:else if !diff.text}
            <div class={line({ kind: 'meta' })}>No diff to show (a new untracked file, or a binary).</div>
          {:else}
            {#each lines as l}<div class={line({ kind: kind(l) })}>{l || ' '}</div>{/each}
            {#if diff.cut}<div class={line({ kind: 'meta' })}>… only the first 512 KB are shown</div>{/if}
          {/if}
        </div>
      </div>
    {/if}
  {/if}
</Modal>
