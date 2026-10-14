<script lang="ts">
  import { css } from 'styled-system/css'
  import { api, type Membership } from '../../lib/api'
  import { names } from '../../lib/format'
  import { go } from '../../lib/router.svelte'
  import { button, chip, code, input, mono, muted, panel, panelHeader } from '../../lib/styles'
  import Icon from '../../components/Icon.svelte'
  import { toast } from '../../components/Toasts.svelte'

  // A project's side of a workspace: join one whose folder holds it, or leave; which members it
  // sees and its own hiding rules. A restart applies a change (the page lists it as pending).
  let { project, onchange }: { project: string; onchange: () => void } = $props()

  let m = $state<Membership | null>(null)
  let related = $state<string[]>([])
  let exclude = $state('')
  let joining = $state('')
  let saving = $state(false)
  let leaving = $state(false) // asked to leave: confirm first
  let showFolder = $state(true) // joining: its folder becomes a member, seen by the other projects
  // Which members it sees: all of them, or only the ones ticked (none ticked yet still counts).
  let only = $state(false)

  function show(v: Membership) {
    m = v
    related = [...v.related]
    only = v.related.length > 0
    leaving = false
    exclude = v.exclude.join('\n')
    joining = v.candidates[0] ?? ''
  }
  $effect(() => {
    api.membership(project).then(show).catch(() => (m = null))
  })

  const changed = $derived(
    !!m && (related.join() !== m.related.join() || exclude.split('\n').map((s) => s.trim()).filter(Boolean).join() !== m.exclude.join()),
  )

  // Joining: showFolder makes its folder a member (or it keeps to itself). Leaving: removeFolder
  // takes it out. Either way the workspace's other projects see the change after a restart (they
  // list it as pending).
  async function save(workspace: string, keep = true, removeFolder = false) {
    saving = true
    const was = m?.workspace ?? ''
    try {
      const rules = exclude.split('\n').map((s) => s.trim()).filter(Boolean)
      const joins = !!workspace && workspace !== was
      const r = await api.setMembership(project, {
        workspace, related: keep ? related : [], exclude: keep ? rules : [], show_folder: joins && showFolder, remove_folder: removeFolder,
      })
      show(r)
      toast(workspace ? `Saved; a restart of ${project} applies it` : `${project} left ${was}; a restart applies it`, 'ok')
      if (r.restart.length)
        toast(`${names(r.restart)} ${workspace ? 'see' : 'stop seeing'} ${project}'s folder after a restart.`, 'ok')
      onchange()
    } catch (e) {
      toast((e as Error).message, 'bad')
    } finally {
      saving = false
    }
  }

  function toggle(name: string, on: boolean) {
    related = on ? [...related, name] : related.filter((r) => r !== name)
  }

  const body = css({ px: '4', py: '3', display: 'grid', gap: '3' })
  const row = css({ display: 'flex', gap: '2', alignItems: 'center', flexWrap: 'wrap' })
  const label = css({ fontWeight: 'semibold', fontSize: 'md' })
  const radios = css({ display: 'flex', gap: '4', flexWrap: 'wrap', fontSize: 'md', '& label': { display: 'inline-flex', gap: '1.5', alignItems: 'center', cursor: 'pointer' } })
  const confirm = css({ display: 'flex', alignItems: 'center', gap: '2', flexWrap: 'wrap', fontSize: 'md', px: '3', py: '2', borderRadius: 'control', bg: 'badSubtle' })
  const check = css({ display: 'flex', gap: '2', alignItems: 'flex-start', fontSize: 'md', cursor: 'pointer', '& input': { mt: '1' } })
  const help = css({ color: 'fg.muted', fontSize: 'sm' })
  const area = css({ h: 'auto', py: '1.5', fontFamily: 'mono', fontSize: 'sm', resize: 'vertical' })
  const select = css({ h: 'control.icon', px: '2', borderRadius: 'control', border: '1px solid {colors.border.strong}', bg: 'bg.panel' })
</script>

{#if m && (m.workspace || m.candidates.length)}
  <section class={panel}>
    <div class={panelHeader}><Icon name="workspaces" size={14} />Workspace</div>
    <div class={body}>
      {#if !m.workspace}
        <p class={muted}>Its folder is in a workspace: in it, the project sees the related repos read-only.</p>
        <div class={row}>
          {#if m.candidates.length > 1}
            <select class={select} bind:value={joining}>{#each m.candidates as c}<option value={c}>{c}</option>{/each}</select>
          {/if}
          <button class={button({ variant: 'primary', size: 'sm' })} disabled={saving} onclick={() => save(joining, false)}>Join {joining}</button>
        </div>
        <label class={check}>
          <input type="checkbox" bind:checked={showFolder} />
          <span>Show its folder to the other projects <span class={help}>· they see it read-only, after a restart. Unticked, it keeps to itself: it sees them, they don't see it.</span></span>
        </label>
      {:else}
        <div class={row}>
          <button class={button({ size: 'sm' })} onclick={() => go('workspaces', m!.workspace)}><Icon name="workspaces" size={12} />{m.workspace}</button>
          {#if !leaving}
            <button class={button({ size: 'sm', variant: 'danger' })} style="margin-left:auto" disabled={saving} onclick={() => (leaving = true)}>Leave</button>
          {/if}
        </div>
        {#if leaving}
          <div class={confirm} role="alert">
            Leave {m.workspace}? The project stops seeing its members after a restart. Should the other projects keep seeing its folder?
            <span style="margin-left:auto;display:flex;gap:var(--spacing-2);flex-wrap:wrap">
              <button class={button({ size: 'sm' })} onclick={() => (leaving = false)}>Cancel</button>
              <button class={button({ size: 'sm', variant: 'danger' })} disabled={saving} title="Its folder stays a member: the other projects still see it"
                onclick={() => save('', false)}>Leave, keep the folder shown</button>
              <button class={button({ size: 'sm', variant: 'danger' })} disabled={saving} title="Its folder stops being a member: the other projects no longer see it"
                onclick={() => save('', false, true)}>Leave and hide the folder</button>
            </span>
          </div>
        {/if}
        {#if m.members.length}
          <div>
            <div class={label}>Sees</div>
            <div class={radios} role="radiogroup" aria-label="Members it sees" style="margin-block:var(--spacing-1\.5)">
              <label><input type="radio" name="sees-{project}" checked={!only} onchange={() => ((only = false), (related = []))} />All members</label>
              <label><input type="radio" name="sees-{project}" checked={only} onchange={() => (only = true)} />Only these</label>
            </div>
            {#if only}
              <div class={row}>
                {#each m.members as name (name)}
                  <label class={chip}><input type="checkbox" checked={related.includes(name)} onchange={(e) => toggle(name, e.currentTarget.checked)} />{name}</label>
                {/each}
              </div>
              {#if related.length === 0}<p class={help} style="margin-top:var(--spacing-1)">Tick at least one; none ticked sees them all.</p>{/if}
            {/if}
          </div>
        {/if}
        <div style="display:grid;gap:var(--spacing-1\.5)">
          <label class={label} for="ws-exclude">More hiding rules for this project</label>
          <textarea id="ws-exclude" class="{input} {area}" rows="3" placeholder={'drafts/\n-docs/public'} bind:value={exclude}></textarea>
          <span class={help}>
            One path per line, added to the workspace's rules: <span class={code}>drafts/</span> hides it,
            <span class={code}>-docs/public</span> shows a hidden path again.
          </span>
        </div>
        <div><button class={button({ variant: 'primary', size: 'sm' })} disabled={saving || !changed} onclick={() => save(m!.workspace)}>Save</button></div>
      {/if}
    </div>
  </section>
{/if}
