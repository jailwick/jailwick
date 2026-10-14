<script lang="ts">
  import { css } from 'styled-system/css'
  import { api, type ProjectDetail } from '../../lib/api'
  import { ago } from '../../lib/format'
  import { button, link, muted } from '../../lib/styles'
  import Icon from '../../components/Icon.svelte'
  import Modal from '../../components/Modal.svelte'
  import { toast } from '../../components/Toasts.svelte'

  // The project's run command in the main checkout (worktree "") or a worktree: Run or Stop, a
  // link to the app on its port, and its output. Runs go in the main container, so the app is at
  // the project's own address.
  let { d, worktree = '', port, onchange }: { d: ProjectDetail; worktree?: string; port: number; onchange: () => void } =
    $props()

  const run = $derived(d.runs.find((r) => (r.worktree ?? '') === worktree))
  let busy = $state(false)
  let output = $state(false)
  let text = $state('')

  async function act(action: 'start' | 'stop') {
    busy = true
    try {
      await api.run(d.name, action, worktree)
      onchange()
    } catch (e) {
      toast((e as Error).message, 'bad')
    } finally {
      busy = false
    }
  }
  async function showOutput() {
    output = true
    try {
      text = (await api.runLog(d.name, worktree)).text || 'No output yet.'
    } catch (e) {
      text = (e as Error).message
    }
  }

  const dot = css({ w: 'dot.lg', h: 'dot.lg', borderRadius: 'full', bg: 'ok', boxShadow: '0 0 0 3px {colors.okSubtle}', flexShrink: 0 })
  const row = css({ display: 'inline-flex', alignItems: 'center', gap: '2', whiteSpace: 'nowrap' })
  const log = css({
    m: 0, p: '3', maxH: '60vh', overflow: 'auto', borderRadius: 'control', bg: 'bg.inset', border: '1px solid {colors.border.default}',
    fontFamily: 'mono', fontSize: 'xs', lineHeight: 'code', whiteSpace: 'pre-wrap', wordBreak: 'break-word',
  })
</script>

{#if d.config.run}
  <span class={row}>
    {#if run?.running}
      <span class={dot} role="img" aria-label="running" title="Running, started {ago(run.started)} ago"></span>
      {#if d.url_base}<a class={link} href="{d.url_base}:{port}" target="_blank" rel="noreferrer" title="The app, in your browser">Open :{port}</a>{/if}
      <button class={button({ size: 'sm' })} disabled={busy} onclick={() => act('stop')}><Icon name="stop" size={11} />Stop</button>
    {:else}
      <button class={button({ size: 'sm' })} disabled={busy || !d.running} title={d.config.run} onclick={() => act('start')}>
        <Icon name={busy ? 'spinner' : 'play'} size={11} />Run
      </button>
    {/if}
    {#if run}<button class={button({ size: 'sm' })} onclick={showOutput}>Output</button>{/if}
  </span>
  <Modal bind:open={output} wide title="{worktree || 'Main checkout'}: {d.config.run}">
    <div style="display:flex;justify-content:flex-end">
      <button class={button({ size: 'sm' })} onclick={showOutput}><Icon name="refresh" size={12} />Refresh</button>
    </div>
    <pre class={log}>{text}</pre>
    <span class={muted} style="font-size:var(--font-sizes-xs)">The last 64 KB of its output.</span>
  </Modal>
{/if}
