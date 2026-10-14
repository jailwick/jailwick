<script lang="ts">
  import { css } from 'styled-system/css'
  import { api, type About } from '../lib/api'
  import { mono, muted, panel, panelHeader } from '../lib/styles'
  import Page from '../lib/Page.svelte'
  import Icon from '../components/Icon.svelte'
  import Mark from '../components/Mark.svelte'
  import UpdatesPanel from './about/UpdatesPanel.svelte'

  // This build of jailwick, the machine and runtime it works with, and where it keeps its files.
  let about = $state<About | null>(null)
  let error = $state('')
  api.about().then((a) => (about = a)).catch((e) => (error = e.message))

  // The commit and date it was built from, when the build knows them.
  const build = $derived.by(() => {
    const b = about?.build
    if (!b?.commit) return 'not recorded (built without git)'
    const date = b.date ? new Date(b.date + 'T00:00:00').toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' }) : ''
    return [b.commit + (b.modified ? ' with changes' : ''), date].filter(Boolean).join(' · ')
  })

  const runtime = $derived.by(() => {
    const r = about?.runtime
    if (!r) return ''
    if (r.error) return `${r.name || 'runtime'}: not running`
    return [r.platform ?? r.name, r.version].filter(Boolean).join(' ')
  })

  const hero = css({ display: 'flex', alignItems: 'center', gap: '4', p: '5' })
  const name = css({ fontSize: '3xl', fontWeight: 'heavy', letterSpacing: 'tight' })
  const grid = css({ display: 'grid', gridTemplateColumns: { base: '1fr', xl: '1fr 1fr' }, gap: '5', alignItems: 'start', mt: '5' })
  const kv = css({
    display: 'grid', gridTemplateColumns: '{sizes.label} 1fr', rowGap: '2', columnGap: '4', px: '4', py: '3',
    '& dt': { color: 'fg.muted' }, '& dd': { minW: 0, wordBreak: 'break-word' },
  })
  const list = css({ m: 0, px: '4', py: '3', pl: '8', display: 'grid', gap: '1.5', fontSize: 'base' })
  const date = css({ color: 'fg.faint', fontSize: 'xs', ml: '1.5' })
</script>

<Page title="About" subtitle="This build of jailwick and what it works with.">
  {#if error}<p class={muted}>{error}</p>{/if}
  {#if about}
    <section class="{panel} {hero}">
      <Mark width={44} />
      <div>
        <div class={name}>jailwick {about.version}</div>
        <p class={muted}>Runs Claude Code in a locked-down sandbox per project: its own services, an allowlisted network, and only the repo mounted.</p>
      </div>
    </section>

    <div class={grid}>
      <UpdatesPanel />
      <section class={panel}>
        <div class={panelHeader}><Icon name="monitor" size={14} />This build</div>
        <dl class={kv}>
          <dt>Version</dt><dd>{about.version}</dd>
          <dt>Built from</dt><dd class={mono}>{build}</dd>
          <dt>Sandbox helper</dt><dd>{about.agent?.label ?? 'a development build'}</dd>
          <dt>System</dt><dd>{about.os} · {about.arch}</dd>
          <dt>Built with</dt><dd class={mono}>{about.go}</dd>
        </dl>
      </section>

      <section class={panel}>
        <div class={panelHeader}><Icon name="images" size={14} />Runtime and setup</div>
        <dl class={kv}>
          <dt>Container runtime</dt><dd title={about.runtime.error ?? ''}>{runtime}</dd>
          <dt>Projects</dt><dd>{about.projects}</dd>
          <dt>Workspaces</dt><dd>{about.workspaces}</dd>
        </dl>
      </section>

      <section class={panel}>
        <div class={panelHeader}><Icon name="folder" size={14} />Where jailwick keeps its files</div>
        <dl class={kv}>
          <dt>Settings</dt><dd class={mono}>{about.folders.config}</dd>
          <dt>Generated</dt><dd class={mono} title="Regenerated as needed; safe to delete">{about.folders.state}</dd>
          <dt>Kept data</dt><dd class={mono} title="What jailwick keeps and can't regenerate">{about.folders.data}</dd>
          <dt>Build logs</dt><dd class={mono}>{about.folders.build_logs}</dd>
        </dl>
      </section>

      {#if about.agent?.changes.length}
        <section class={panel}>
          <div class={panelHeader}><Icon name="branch" size={14} />Latest changes to the sandbox helper</div>
          <ul class={list}>
            {#each about.agent.changes as c}<li>{c.text}<span class={date}>{c.date}</span></li>{/each}
          </ul>
        </section>
      {/if}
    </div>
  {/if}
</Page>
