<script lang="ts">
  import { css } from 'styled-system/css'
  import { api, type ProjectDetail, type Worktree, type WorktreeCheck } from '../../../lib/api'
  import { go } from '../../../lib/router.svelte'
  import { button, linkButton, mono, muted } from '../../../lib/styles'
  import Icon from '../../../components/Icon.svelte'
  import Modal from '../../../components/Modal.svelte'
  import { toast } from '../../../components/Toasts.svelte'

  // Removing a worktree, said before it's done: the folder, its database copies and Redis
  // database go; the commits are on the branch, kept unless deleting it is ticked. A merged
  // branch is offered to delete (its commits are in the main checkout's branch); an unmerged one
  // is kept, with deleting it anyway a separate, red choice. Uncommitted changes and sessions
  // running in it are said first: the changes go only when ticked, the sessions end first.
  let { d, wt, open = $bindable(false), onchange }: { d: ProjectDetail; wt: Worktree; open?: boolean; onchange: () => void } = $props()

  let check = $state<WorktreeCheck | null>(null)
  let error = $state('')
  let withChanges = $state(false)
  let deleteMerged = $state(true)
  let deleteAnyway = $state(false)
  let busy = $state(false)
  $effect(() => {
    if (!open) return
    check = null
    error = ''
    withChanges = deleteAnyway = false
    deleteMerged = true
    api.checkWorktree(d.name, wt.name).then((c) => (check = c)).catch((e) => (error = (e as Error).message))
  })

  const branchKept = $derived(!!check && !check.missing && !!check.into)
  const branch = $derived<'' | 'merged' | 'any'>(
    !check || !check.into ? '' : check.merged ? (deleteMerged ? 'merged' : '') : deleteAnyway ? 'any' : '',
  )
  const blocked = $derived(!check || check.sessions > 0 || (check.changed > 0 && !withChanges))

  async function remove() {
    busy = true
    try {
      await api.removeWorktree(d.name, wt.name, withChanges, branch)
      toast(branch ? `Removed ${wt.name} and its branch` : `Removed ${wt.name}; the branch ${check?.branch} is kept`, 'ok')
      open = false
      onchange()
    } catch (e) {
      toast((e as Error).message, 'bad')
    } finally {
      busy = false
    }
  }

  const n = (k: number, one: string, many = one + 's') => `${k} ${k === 1 ? one : many}`
  const body = css({ display: 'grid', gap: '3', fontSize: 'md', lineHeight: 'normal' })
  const note = css({ display: 'flex', gap: '2', alignItems: 'flex-start', px: '3', py: '2.5', borderRadius: 'control', fontSize: 'md', '& > svg': { flexShrink: 0, mt: '0.5' } })
  const warn = css({ bg: 'warnSubtle', '& > svg': { color: 'warn' } })
  const badNote = css({ bg: 'badSubtle', '& > svg': { color: 'bad' } })
  const choice = css({ display: 'flex', gap: '2', alignItems: 'flex-start', cursor: 'pointer', '& input': { mt: '1' } })
  const danger = css({ color: 'bad' })
  const foot = css({ display: 'flex', gap: '2', justifyContent: 'flex-end', mt: '2' })
</script>

<Modal bind:open title="Remove worktree {wt.name}?">
  <div class={body}>
    {#if error}
      <p class={danger}>{error}</p>
    {:else if !check}
      <p class={muted}>Checking what it has…</p>
    {:else}
      {#if check.missing}
        <p>Its folder is gone already: this frees its ports{wt.dbs?.length ? ', drops its database copies' : ''} and Redis database.</p>
      {:else}
        <p>
          This deletes its folder <span class={mono}>.jailwick/worktrees/{wt.name.replaceAll('/', '-')}</span>{#if wt.dbs?.length}, its database copies
          (<span class={mono}>{wt.dbs.join(', ')}</span>){/if} and Redis database {wt.redis_db}, and stops its app if it runs. What's committed is on the
          branch <span class={mono}>{check.branch}</span>.
        </p>
      {/if}

      {#if check.sessions > 0}
        <div class="{note} {badNote}"><Icon name="alert" size={14} />
          <span>{n(check.sessions, 'session')} {check.sessions === 1 ? 'runs' : 'run'} in it: end {check.sessions === 1 ? 'it' : 'them'} first, on the
            <button class={linkButton} onclick={() => { open = false; go('projects', d.name, 'sessions') }}>Sessions tab</button>.</span>
        </div>
      {/if}

      {#if check.changed > 0}
        <div class="{note} {warn}"><Icon name="alert" size={14} />
          <span>{n(check.changed, 'file')} {check.changed === 1 ? 'has' : 'have'} uncommitted changes: they're lost with the folder. Commit them in the worktree to keep them.</span>
        </div>
        <label class={choice}><input type="checkbox" bind:checked={withChanges} />
          <span class={danger}>Remove it with its uncommitted changes</span></label>
      {/if}

      {#if branchKept && check.merged}
        <label class={choice}><input type="checkbox" bind:checked={deleteMerged} />
          <span>Also delete the branch <span class={mono}>{check.branch}</span> <span class={muted}>· merged into {check.into}: its commits are there</span></span>
        </label>
      {:else if branchKept}
        <div class="{note} {warn}"><Icon name="alert" size={14} />
          <span>
            <span class={mono}>{check.branch}</span> has {n(check.ahead, 'commit')} not in {check.into}{check.pushed ? ' (pushed to its remote)' : ''}: the branch is
            kept, so nothing is lost. Merge it, then delete it, or delete it here if you don't need {check.ahead === 1 ? 'it' : 'them'}.
          </span>
        </div>
        <label class={choice}><input type="checkbox" bind:checked={deleteAnyway} />
          <span class={danger}>Delete the unmerged branch anyway: {check.pushed ? `its ${n(check.ahead, 'commit')} stay only on the remote` : `its ${n(check.ahead, 'commit')} are lost`}</span>
        </label>
      {/if}
    {/if}
  </div>
  <div class={foot}>
    <button class={button()} onclick={() => (open = false)}>Cancel</button>
    <button class={button({ variant: 'danger' })} disabled={busy || blocked} onclick={remove}>
      {busy ? 'Removing…' : branch ? 'Remove worktree and branch' : 'Remove worktree'}
    </button>
  </div>
</Modal>
