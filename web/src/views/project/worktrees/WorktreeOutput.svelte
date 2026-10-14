<script lang="ts">
  import { css } from 'styled-system/css'
  import { tick } from 'svelte'
  import { api, type WorktreeJob } from '../../../lib/api'
  import { button, muted } from '../../../lib/styles'
  import Modal from '../../../components/Modal.svelte'

  // The output of a worktree being made (database copies, setup commands), growing while it runs.
  // ondismiss: forget a job that ended with a problem (its output with it).
  let { project, job, open = $bindable(false), ondismiss }: { project: string; job: WorktreeJob; open?: boolean; ondismiss?: () => void } = $props()

  let text = $state('')
  let out: HTMLPreElement | undefined = $state()
  const running = $derived(job.running)
  $effect(() => {
    if (!open) return
    let stopped = false
    let timer: ReturnType<typeof setTimeout>
    async function load() {
      try {
        const atEnd = !out || out.scrollHeight - out.scrollTop - out.clientHeight < 24
        text = (await api.worktreeLog(project, job.name)).text
        if (atEnd) {
          await tick()
          out?.scrollTo({ top: out.scrollHeight })
        }
      } catch {
        // made fine: the job and its output are gone
      }
      if (!stopped && running) timer = setTimeout(load, 1000)
    }
    void load()
    return () => {
      stopped = true
      clearTimeout(timer)
    }
  })

  const foot = css({ display: 'flex', justifyContent: 'flex-end' })
  const pre = css({
    m: 0, p: '3', h: 'min(60vh, 520px)', overflowY: 'auto', borderRadius: 'control', bg: 'bg.inset',
    border: '1px solid {colors.border.default}', fontFamily: 'mono', fontSize: 'xs', lineHeight: 'code', whiteSpace: 'pre-wrap', wordBreak: 'break-all',
  })
</script>

<Modal bind:open wide title={running ? `Making worktree ${job.name}…` : `Worktree ${job.name}`}>
  {#if job.setup_failed}<p class={muted}>The worktree is there, but a setup command failed: run it yourself in the worktree (Shell), or remove the worktree and make it again.</p>{/if}
  <pre class={pre} bind:this={out}>{text || (running ? 'Starting…' : 'No output.')}</pre>
  {#if !running && ondismiss}
    <div class={foot}><button class={button()} onclick={ondismiss}>Dismiss</button></div>
  {/if}
</Modal>
