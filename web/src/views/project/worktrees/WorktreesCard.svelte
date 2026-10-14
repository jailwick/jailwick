<script lang="ts">
  import { css } from 'styled-system/css'
  import { api, devBase, type ProjectDetail, type Worktree, type WorktreeJob } from '../../../lib/api'
  import { copyText } from '../../../lib/clipboard'
  import { go } from '../../../lib/router.svelte'
  import { badge, button, link, mono, muted, panel, panelHeader, table, panelCount as count } from '../../../lib/styles'
  import Icon from '../../../components/Icon.svelte'
  import Menu from '../../../components/Menu.svelte'
  import { toast } from '../../../components/Toasts.svelte'
  import ChangesButton from '../ChangesButton.svelte'
  import RunControl from '../RunControl.svelte'
  import NewWorktree from './NewWorktree.svelte'
  import RemoveWorktree from './RemoveWorktree.svelte'
  import WorktreeOutput from './WorktreeOutput.svelte'
  import type { Phase } from '../../../lib/phase'

  // The main checkout and the worktrees: each with its ports, databases, app run and changes; a
  // session (or shell) opens in your terminal. Worktrees are made here (New worktree, in the
  // background, its output a click away) and removed from a row's menu, said before it's done.
  let { d, urls, phase = '', onchange }: { d: ProjectDetail; urls: boolean; phase?: Phase; onchange: () => void } = $props()
  // The sandbox is up and no start, restart or stop is under way.
  const ready = $derived(d.running && !phase)

  let creating = $state(false)
  let removing = $state<Worktree | null>(null)
  let removeOpen = $state(false)
  let output = $state<WorktreeJob | null>(null)
  let outputOpen = $state(false)

  // Jobs of worktrees not listed yet (being made, or failed), and the problem of those that are.
  const listed = $derived(new Set(d.worktrees.map((w) => w.name)))
  const pending = $derived(d.worktree_jobs.filter((j) => !listed.has(j.name)))
  const jobOf = (name: string) => d.worktree_jobs.find((j) => j.name === name)

  async function open(wt: string, command: 'claude' | 'shell') {
    try {
      await api.terminal(d.name, { worktree: wt, command })
      toast(`Opened ${command === 'claude' ? 'a session' : 'a shell'} in ${wt}`, 'ok')
    } catch (e) {
      toast((e as Error).message, 'bad')
    }
  }
  async function copy(path: string) {
    if (await copyText(path)) toast(`Copied ${path}`, 'ok')
    else toast("Couldn't copy the path", 'bad')
  }
  async function dismiss(name: string) {
    await api.dismissWorktreeJob(d.name, name).catch(() => {})
    onchange()
  }
  async function retry(j: WorktreeJob) {
    try {
      await api.addWorktree(d.name, j.name, j.base)
      onchange()
    } catch (e) {
      toast((e as Error).message, 'bad')
    }
  }
  function showOutput(j: WorktreeJob) {
    output = j
    outputOpen = true
  }
  function remove(w: Worktree) {
    removing = w
    removeOpen = true
  }

  const emptyRow = css({ px: '4', py: '4', color: 'fg.muted', display: 'flex', alignItems: 'center', gap: '3', flexWrap: 'wrap' })
  const num = css({ fontVariantNumeric: 'tabular-nums', whiteSpace: 'nowrap' })
  const mainRow = css({ display: 'flex', alignItems: 'center', gap: '3', flexWrap: 'wrap', px: '4', py: '2.5', borderBottom: '1px solid {colors.border.default}' })
  const hint = css({ px: '4', py: '2.5', fontSize: 'sm', color: 'fg.muted', borderTop: '1px solid {colors.border.default}' })
  const head = css({ ml: 'auto', display: 'flex', alignItems: 'center', gap: '3', textTransform: 'none', letterSpacing: 0 })
  const actions = css({ textAlign: 'right', whiteSpace: 'nowrap', '& > *': { ml: '1' } })
  const small = css({ fontSize: 'sm' })
  const badgeButton = css({ cursor: 'pointer', border: 'none', _hover: { textDecoration: 'underline' } })
  const errText = css({ color: 'bad', fontSize: 'sm', overflowWrap: 'anywhere' })
</script>

<section class={panel}>
  <div class={panelHeader}>
    <Icon name="branch" size={14} />Worktrees
    <span class={head}>
      <span class={count} style="margin-left:0">{d.worktrees.length}</span>
      <button class={button({ size: 'sm' })} disabled={!ready} title={ready ? 'A worktree with its own ports and databases' : phase ? 'Wait for the sandbox' : 'Start the sandbox first'}
        onclick={() => (creating = true)}><Icon name="plus" size={12} />New worktree</button>
    </span>
  </div>
  <div class={mainRow}>
    <strong>Main checkout</strong>
    {#if d.config.run}<RunControl {d} port={3000} {onchange} />{/if}
    <ChangesButton project={d.name} disabled={!ready} />
    {#if d.config.run}<span class={muted} style="margin-left:auto">Run: <span class={mono}>{d.config.run}</span></span>{/if}
  </div>
  {#if d.worktrees.length === 0 && pending.length === 0}
    <div class={emptyRow}>
      No worktrees. Each gets its own folder, branch, ports and databases, for a session to work beside the main checkout.
      {#if ready}<button class={button({ size: 'sm' })} onclick={() => (creating = true)}><Icon name="plus" size={12} />New worktree</button>{/if}
    </div>
  {:else}
    <div style="overflow-x:auto">
    <table class={table}>
      <thead><tr><th>Worktree</th><th>Ports</th><th>Databases</th>{#if d.config.run}<th>App</th>{/if}<th></th></tr></thead>
      <tbody>
        {#each d.worktrees as w (w.name)}
          {@const job = jobOf(w.name)}
          <tr>
            <td style="white-space:nowrap">
              <strong>{w.name}</strong>
              {#if w.missing}<span class={badge({ tone: 'warn' })}>folder missing</span>
              {:else if w.branch !== w.name}<div class="{muted} {small}">on {w.branch}</div>{/if}
              {#if job?.setup_failed}
                <div><button class="{badge({ tone: 'warn' })} {badgeButton}" title="A setup command failed: see its output" onclick={() => showOutput(job)}>setup failed</button></div>
              {/if}
            </td>
            {#if w.unmanaged}
              <td colspan="2"><span class={badge({ tone: 'warn' })} title="Made outside jailwick: it has no ports or databases of its own">no slot</span> <span class={muted}>uses the main checkout's</span></td>
            {:else}
              <td class={num}>
                {#if urls && devBase(d, w.name)}<a class={link} href="{devBase(d, w.name)}:{w.port}" target="_blank" rel="noreferrer">{w.port}</a>{:else}{w.port}{/if}–{w.port + 9}
              </td>
              <td>
                <span class={mono}>{w.dbs?.join(', ') || '–'}</span>
                <div class="{muted} {small}">Redis database {w.redis_db}</div>
              </td>
            {/if}
            {#if d.config.run}<td>{#if !w.unmanaged && !w.missing}<RunControl {d} worktree={w.name} port={w.port} {onchange} />{/if}</td>{/if}
            <td class={actions}>
              {#if !w.unmanaged && !w.missing}<ChangesButton project={d.name} worktree={w.name} disabled={!ready} />{/if}
              {#if !w.missing}
                <button class={button({ size: 'sm' })} disabled={!ready} onclick={() => open(w.name, 'claude')}><Icon name="terminal" size={12} />Session</button>
              {/if}
              <Menu title="More for {w.name}" disabled={!ready} class={button({ size: 'sm' })} items={[
                ...(w.missing ? [] : [
                  { label: 'Shell', hint: 'in your terminal', run: () => open(w.name, 'shell') },
                  { label: 'Copy path', run: () => copy(w.dir) },
                ]),
                ...(w.unmanaged ? [] : ['separator' as const, { label: w.missing ? 'Forget…' : 'Remove…', danger: true, run: () => remove(w) }]),
              ]} />
            </td>
          </tr>
        {/each}
        {#each pending as j (j.name)}
          <tr>
            <td style="white-space:nowrap">
              <strong>{j.name}</strong>
              <div class="{muted} {small}">from {j.base}</div>
            </td>
            <td colspan={d.config.run ? 3 : 2}>
              {#if j.running}
                <span class={badge({ tone: 'info' })}><Icon name="spinner" size={11} />creating</span>
                <span class="{muted} {small}">copying databases, running the setup commands…</span>
              {:else}
                <span class={badge({ tone: 'bad' })}>couldn't create it</span> <span class={errText}>{j.error}</span>
              {/if}
            </td>
            <td class={actions}>
              <button class={button({ size: 'sm' })} onclick={() => showOutput(j)}>Output</button>
              {#if !j.running}
                <button class={button({ size: 'sm' })} disabled={!ready} onclick={() => retry(j)}>Try again</button>
                <button class={button({ size: 'sm' })} onclick={() => dismiss(j.name)}>Dismiss</button>
              {/if}
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
    </div>
  {/if}
  {#if !d.config.run}
    <div class={hint}>
      No run command yet: add one (like <span class={mono}>pnpm dev</span>) in <button class={link} onclick={() => go('projects', d.name, 'settings')}>Settings</button> to get Run
      buttons, each worktree on its own ports.
    </div>
  {/if}
</section>

<NewWorktree {d} bind:open={creating} {onchange} />
{#if removing}<RemoveWorktree {d} wt={removing} bind:open={removeOpen} {onchange} />{/if}
{#if output}
  <WorktreeOutput project={d.name} job={d.worktree_jobs.find((j) => j.name === output!.name) ?? { ...output, running: false }} bind:open={outputOpen}
    ondismiss={() => { outputOpen = false; dismiss(output!.name) }} />
{/if}
