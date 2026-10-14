<script lang="ts">
  import { css } from 'styled-system/css'
  import { tick } from 'svelte'
  import { api, type BuildStatus } from '../../lib/api'
  import { focus } from '../../lib/router.svelte'
  import { button, linkButton, mono } from '../../lib/styles'
  import Icon from '../../components/Icon.svelte'
  import Modal from '../../components/Modal.svelte'

  // The image a start is building, in plain words; the build's output is a click away, growing
  // as the build goes. A failed build stays until the next start.
  // onretry: starts again (none while that can't run).
  let { project, build, onretry }: { project: string; build: BuildStatus; onretry?: () => void } = $props()

  // The line that most likely says why it failed: a specific cause ("E: Unable to locate package
  // x", "not found") before a generic "ERROR: process … did not complete", else the last line.
  const reason = $derived.by(() => {
    const lines = (build.tail ?? []).map((l) => l.replace(/^#\d+\s+[\d.]+\s+/, '').trim()).filter(Boolean)
    const last = (re: RegExp) => [...lines].reverse().find((l) => re.test(l))
    return last(/^E: |unable to|not found|no such|denied|cannot|could not/i) ?? last(/error|failed/i) ?? lines.at(-1) ?? ''
  })

  let details = $state(false)
  // A failed start's notification opens the output (?show=build), the place to see why.
  $effect(() => {
    if (focus('show') === 'build') details = true
  })
  let text = $state('')
  let size = 0 // how much of the log text holds
  let out: HTMLPreElement | undefined = $state()
  const log = $derived(build.log) // build is a new object each poll; this changes with the log only

  // While the details are open, append what the log gained each second, and stay at the bottom
  // unless scrolled up. Until the log can be read, the latest line stands in. (The server serves
  // the log of the build it tracks, so log only says when that changes.)
  $effect(() => {
    if (!details || !log) return // a new image's build (another log) starts over
    let stopped = false
    let timer: ReturnType<typeof setTimeout>
    text = ''
    size = 0
    async function load() {
      try {
        const r = await api.buildLog(project, size)
        if (stopped) return
        const atEnd = !out || out.scrollHeight - out.scrollTop - out.clientHeight < 24
        text = r.from === size ? text + r.text : r.text
        size = r.size
        if (atEnd) {
          await tick()
          out?.scrollTo({ top: out.scrollHeight })
        }
      } catch {
        // the build ended (nothing to read) or the log isn't there yet
      }
      if (!stopped && !build.failed) timer = setTimeout(load, 1000)
    }
    load()
    return () => {
      stopped = true
      clearTimeout(timer)
    }
  })

  const box = css({
    display: 'flex', alignItems: 'center', gap: '2.5', flexWrap: 'wrap', mt: '4', px: '3.5', py: '2.5', borderRadius: 'panel',
    fontSize: 'md', borderWidth: '1px', borderStyle: 'solid',
  })
  const busy = css({ bg: 'infoSubtle', borderColor: 'info/30', '& > svg': { color: 'info' } })
  const failed = css({ bg: 'badSubtle', borderColor: 'bad/30', alignItems: 'flex-start', '& > svg': { color: 'bad', mt: '1px' } })
  const progress = css({ color: 'fg.muted', fontVariantNumeric: 'tabular-nums' })
  const bar = css({ flex: '1 1 120px', maxW: '220px', h: '4px', borderRadius: 'full', bg: 'info/20', overflow: 'hidden' })
  const fill = css({ h: '100%', bg: 'info', borderRadius: 'full', transitionProperty: 'width', transitionDuration: 'slow' })
  const failedText = css({ flex: 1, minW: 0, display: 'grid', gap: '1' })
  const reasonC = css({ fontFamily: 'mono', fontSize: 'xs', color: 'fg.default', overflowWrap: 'anywhere' })
  const right = css({ ml: 'auto', display: 'flex', alignItems: 'center', gap: '3' })
  const label = css({ fontSize: 'xs', fontWeight: 'semibold', color: 'fg.muted' })
  const output = css({
    m: 0, p: '3', h: 'min(60vh, 560px)', overflowY: 'auto', borderRadius: 'control', bg: 'bg.inset',
    border: '1px solid {colors.border.default}', fontFamily: 'mono', fontSize: 'xs', lineHeight: 'code',
    whiteSpace: 'pre-wrap', wordBreak: 'break-all',
  })
  const path = css({ userSelect: 'all', wordBreak: 'break-all' })
</script>

<div class="{box} {build.failed ? failed : busy}" role={build.failed ? 'alert' : 'status'}>
  {#if build.failed}
    <Icon name="alert" size={15} />
    <div class={failedText}>
      <strong>Setting up the sandbox failed</strong>
      {#if reason}<span class={reasonC}>{reason}</span>{/if}
    </div>
    <div class={right}>
      <button class={linkButton} onclick={() => (details = true)}>Show output</button>
      {#if onretry}<button class={button({ size: 'sm' })} onclick={onretry}><Icon name="restart" size={12} />Retry</button>{/if}
    </div>
  {:else}
    <Icon name="spinner" size={14} />
    <strong>{build.step}</strong>
    {#if build.total}
      <span class={bar} role="progressbar" aria-valuemin={0} aria-valuemax={build.total} aria-valuenow={build.done}>
        <span class={fill} style="display:block;width:{Math.round(((build.done ?? 0) / build.total) * 100)}%"></span>
      </span>
      <span class={progress}>Step {build.done} of {build.total}</span>
    {/if}
    <button class={linkButton} style="margin-left:auto" onclick={() => (details = true)}>Show output</button>
  {/if}
</div>

<Modal bind:open={details} wide title={build.failed ? 'Setting up the sandbox failed' : build.step}>
  <span class={label}>Output{#if !build.failed && build.total} · step {build.done} of {build.total}{/if}</span>
  <pre class={output} bind:this={out}>{text || (build.failed ? build.tail?.join('\n') : build.line) || 'Starting…'}</pre>
  <span class={label}>Log file</span>
  <span class="{mono} {path}">{build.log}</span>
</Modal>
