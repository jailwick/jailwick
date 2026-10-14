<script lang="ts">
  import { css } from 'styled-system/css'
  import { api, type ProjectDetail } from '../../../lib/api'
  import { button, input, mono, muted } from '../../../lib/styles'
  import Modal from '../../../components/Modal.svelte'
  import { toast } from '../../../components/Toasts.svelte'

  // A new worktree, without a session in it: a name (its branch's, new or one there is) and what
  // it starts from. It's made in the background; the Worktrees card follows it.
  let { d, open = $bindable(false), onchange }: { d: ProjectDetail; open?: boolean; onchange: () => void } = $props()

  let name = $state('')
  let base = $state('')
  let branches = $state<string[]>([])
  let busy = $state(false)
  $effect(() => {
    if (!open) return
    name = ''
    base = ''
    api.branches(d.name).then((bs) => {
      branches = bs
      base ||= bs[0] ?? 'HEAD'
    }).catch(() => (base ||= 'HEAD'))
  })

  // As jw wt checks a name; and not one taken.
  const taken = $derived([...d.worktrees.map((w) => w.name), ...d.worktree_jobs.map((j) => j.name)])
  const problem = $derived.by(() => {
    const n = name.trim()
    if (!n) return ''
    if (!/^[A-Za-z0-9][A-Za-z0-9._/-]*$/.test(n) || n.includes('..') || n.endsWith('/') || n.endsWith('.lock') || n.length > 100)
      return 'Letters, digits, . _ - and /, starting with a letter or digit.'
    if (taken.includes(n)) return `There's a worktree ${n} already.`
    return ''
  })
  const existing = $derived(branches.includes(name.trim()))
  const slot = $derived.by(() => {
    const used = new Set(d.worktrees.filter((w) => !w.unmanaged).map((w) => w.slot))
    let n = 1
    while (used.has(n)) n++
    return n
  })

  async function create(e: SubmitEvent) {
    e.preventDefault()
    const n = name.trim()
    if (!n || problem) return
    busy = true
    try {
      await api.addWorktree(d.name, n, existing ? n : base.trim() || 'HEAD')
      toast(`Making worktree ${n}…`, 'ok')
      open = false
      onchange()
    } catch (err) {
      toast((err as Error).message, 'bad')
    } finally {
      busy = false
    }
  }

  const form = css({ display: 'grid', gap: '4' })
  const field = css({ display: 'grid', gap: '1.5', '& label': { fontWeight: 'semibold', fontSize: 'md' } })
  const note = css({ fontSize: 'sm', color: 'fg.muted', lineHeight: 'normal' })
  const bad = css({ fontSize: 'sm', color: 'bad' })
  const foot = css({ display: 'flex', gap: '2', justifyContent: 'flex-end' })
  const dbs = $derived((d.config.services ?? []).includes('postgres'))
</script>

<Modal bind:open title="New worktree">
  <form class={form} onsubmit={create}>
    <div class={field}>
      <label for="wt-name">Name</label>
      <input id="wt-name" class={input} bind:value={name} placeholder="feat-checkout" autocomplete="off" />
      {#if problem}<span class={bad}>{problem}</span>
      {:else}<span class={note}>Its branch's name too: a new branch, or {existing ? 'this existing branch' : 'one the repo has'}.</span>{/if}
    </div>
    {#if !existing}
      <div class={field}>
        <label for="wt-base">Starts from</label>
        <input id="wt-base" class="{input} {mono}" bind:value={base} list="wt-branches" autocomplete="off" />
        <datalist id="wt-branches">{#each branches as b}<option value={b}></option>{/each}</datalist>
        <span class={note}>A branch or commit; the main checkout's branch by default.</span>
      </div>
    {/if}
    <p class={note}>
      It gets its own folder (<span class={mono}>.jailwick/worktrees/{name.trim().replaceAll('/', '-') || '…'}</span>), ports
      {3000 + slot * 10}–{3000 + slot * 10 + 9}{dbs ? ', copies of the databases' : ''} and Redis database {slot}, then the project's setup
      commands run in it. No session starts: open one from its row when you want.
    </p>
    <div class={foot}>
      <button type="button" class={button()} onclick={() => (open = false)}>Cancel</button>
      <button type="submit" class={button({ variant: 'primary' })} disabled={busy || !name.trim() || !!problem}>Create worktree</button>
    </div>
  </form>
</Modal>
