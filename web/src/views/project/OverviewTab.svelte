<script lang="ts">
  import { css } from 'styled-system/css'
  import type { ProjectDetail } from '../../lib/api'
  import { api } from '../../lib/api'
  import { go } from '../../lib/router.svelte'
  import { panel, panelHeader, table, mono, muted, badge, button, link, panelCount as count } from '../../lib/styles'
  import Icon from '../../components/Icon.svelte'
  import WorktreesCard from './worktrees/WorktreesCard.svelte'
  import SplitButton from '../../components/SplitButton.svelte'
  import { toast } from '../../components/Toasts.svelte'
  import PhaseNote from '../../components/PhaseNote.svelte'
  import type { Phase } from '../../lib/phase'

  let { d, urls, phase = '', onchange }: { d: ProjectDetail; urls: boolean; phase?: Phase; onchange: () => void } = $props()

  const grid = css({ display: 'grid', gridTemplateColumns: { base: '1fr', xl: '1fr 1fr' }, gap: '5', alignItems: 'start' })
  const full = css({ gridColumn: '1 / -1' })
  const kv = css({
    display: 'grid', gridTemplateColumns: '{sizes.label} 1fr', rowGap: '2', columnGap: '4', px: '4', py: '3',
    '& dt': { color: 'fg.muted' }, '& dd': { minW: 0, wordBreak: 'break-word' },
  })
  const emptyRow = css({ px: '4', py: '4', color: 'fg.muted' })

  const img = $derived.by(() => {
    const i = d.config.image ?? {}
    if (i.name) return `${i.name} (as is)`
    if (i.dockerfile) return `your Dockerfile ${i.dockerfile} + jailwick layer`
    if (i.base) return `${i.base} + jailwick layer`
    return 'jailwick base'
  })

  function stateTone(s: string, h: string) {
    if (s === 'paused' || s === 'restarting') return 'warn'
    if (s !== 'running') return s === 'exited' ? 'neutral' : 'bad'
    if (h === 'unhealthy') return 'bad'
    if (h === 'starting') return 'warn'
    return 'ok'
  }

  // Rebuild the images now; the page's build banner follows it, a restart then uses them.
  let building = $state(false)
  async function rebuild(fresh: boolean) {
    building = true
    try {
      await api.rebuildImage(d.name, fresh)
      toast(d.running ? 'Image rebuilt: a restart uses it' : 'Image rebuilt: the next start uses it', 'ok')
      onchange()
    } catch (e) {
      toast((e as Error).message, 'bad')
    } finally {
      building = false
    }
  }

</script>

<div class={grid}>
  <section class="{panel} {full}">
    <div class={panelHeader}>
      <Icon name="sessions" size={14} />Sessions<span class={count}>{d.session_list.length} running</span>
    </div>
    <div class={emptyRow} style="display:flex;align-items:center;gap:var(--spacing-3);flex-wrap:wrap">
      {#if phase}<PhaseNote {phase} />{:else if !d.running}Not running.{:else if d.session_list.length === 0}No sessions running.{:else}
        {d.session_list.map((s) => s.worktree || 'main checkout').join(', ')}
      {/if}
      <button class={button({ size: 'sm' })} style="margin-left:auto" onclick={() => go('projects', d.name, 'sessions')}>
        All sessions <Icon name="chevron" size={11} />
      </button>
    </div>
  </section>

  <section class={panel}>
    <div class={panelHeader}><Icon name="database" size={14} />Services<span class={count}>{d.containers.length}</span></div>
    {#if d.containers.length === 0}
      <div class={emptyRow}>No containers yet.</div>
    {:else}
      <table class={table}>
        <thead><tr><th>Service</th><th>State</th></tr></thead>
        <tbody>
          {#each d.containers as c (c.Name)}
            <tr>
              <td class={mono}>{c.Service}</td>
              <td><span class={badge({ tone: stateTone(c.State, c.Health) })}>{c.State}{c.Health ? ` · ${c.Health}` : ''}</span></td>
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}
  </section>

  <!-- What the project is: the facts to look up. Changing it is on the Settings tab. -->
  <section class={panel}>
    <div class={panelHeader}>
      <Icon name="settings" size={14} />Project
      <button class={link} style="margin-left:auto;text-transform:none;letter-spacing:0;font-weight:var(--font-weights-medium)"
        onclick={() => go('projects', d.name, 'settings')}>Settings ›</button>
    </div>
    <dl class={kv}>
      <dt>Repo</dt><dd class={mono}>{d.dir}</dd>
      <dt>Image</dt>
      <dd>
        {img} <span class={muted}>· Claude Code {d.config.image?.claude_code_version}</span>
        <div style="margin-top:var(--spacing-1\.5)">
          <SplitButton onclick={() => rebuild(false)} disabled={building || !!d.op} title="Build the images again now (a restart uses them)"
            more="More ways to rebuild" items={[{ label: 'Rebuild from scratch', hint: 'no cache, pull the base again', run: () => rebuild(true) }]}>
            <Icon name={building ? 'spinner' : 'restart'} size={12} />{building ? 'Rebuilding…' : 'Rebuild image'}
          </SplitButton>
        </div>
      </dd>
      <dt>Resources</dt><dd>{d.config.resources?.memory} memory · {d.config.resources?.cpus} CPUs</dd>
      <dt>Permissions</dt>
      <dd>{d.config.claude?.yolo ? 'Sessions skip permission prompts (YOLO)' : 'Claude asks before acting (default)'}</dd>
      <dt>Files</dt><dd class={mono}>{d.files.config}<br />{d.files.notes}</dd>
    </dl>
  </section>

  <div class={full}><WorktreesCard {d} {urls} {phase} {onchange} /></div>

</div>
