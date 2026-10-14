<script lang="ts">
  import { css } from 'styled-system/css'
  import { api, ApiError, type Handoff, type WorkspaceView } from '../../lib/api'
  import { copyText } from '../../lib/clipboard'
  import { focus, go } from '../../lib/router.svelte'
  import { panel, panelHeader, table, mono, muted, badge, button } from '../../lib/styles'
  import Icon from '../../components/Icon.svelte'
  import Menu, { type MenuItem } from '../../components/Menu.svelte'
  import Modal from '../../components/Modal.svelte'
  import { toast } from '../../components/Toasts.svelte'

  // What a workspace is now: its root, projects, members, the requests between its sessions and the
  // paths hidden in its members (as one of its projects sees them).
  let { name }: { name: string } = $props()

  let w = $state<WorkspaceView | null>(null)
  let error = $state('')
  let project = $state('') // whose view of the hidden paths; "" for the first
  const load = () => api.workspace(name, project).then((v) => (w = v)).catch((e) => (error = e.message))
  $effect(() => {
    error = ''
    void load()
  })

  const filters = [
    { id: 'open', label: 'Open', keep: (h: Handoff) => !h.Done && h.Status !== 'answered' },
    { id: 'answered', label: 'Answered', keep: (h: Handoff) => !h.Done && h.Status === 'answered' },
    { id: 'all', label: 'All', keep: () => true },
  ]
  let filter = $state('open')
  const handoffs = $derived(w?.handoffs.filter(filters.find((f) => f.id === filter)!.keep) ?? [])
  const openCount = $derived(w?.handoffs.filter((h) => !h.Done).length ?? 0)

  // The request opened in the modal.
  let shown = $state<{ meta: Handoff; body: string; path: string } | null>(null)

  // A request's text as prose: paragraphs, with `code` set apart. Text nodes only: the text comes
  // from a sandbox, so none of it becomes markup.
  const paragraphs = (body: string) => body.trim().split(/\n{2,}/).map((p) => p.split(/(`[^`\n]+`)/))
  let modal = $state(false)
  async function show(h: Handoff) {
    try {
      shown = await api.handoff(name, h.File)
      modal = true
    } catch (e) {
      error = (e as Error).message
    }
  }

  // Keeping the books: a request's status (done moves it to done/), its file, deleting it. Each
  // change is made on the file as read; one a session changed since is refused, and the list
  // (and the open request) read again.
  let busy = $state(false)
  let deleting = $state<Handoff | null>(null)
  let deleteOpen = $state(false)
  async function act(run: () => Promise<{ file?: string } | unknown>, done: string, after?: (file?: string) => void) {
    busy = true
    try {
      const r = (await run()) as { file?: string } | undefined
      toast(done, 'ok')
      await load()
      after?.(r?.file)
    } catch (e) {
      toast((e as Error).message, 'bad')
      if (e instanceof ApiError && (e.status === 409 || e.status === 404)) {
        await load()
        if (modal && shown) shown = await api.handoff(name, shown.meta.File).catch(() => null)
      }
    } finally {
      busy = false
    }
  }
  const statusOf = (h: Handoff) => (h.Done ? 'done' : h.Status || 'open')
  function setStatus(h: Handoff, status: 'open' | 'answered' | 'done') {
    const words = { open: 'Reopened', answered: 'Marked answered', done: 'Marked done' }[status]
    void act(() => api.setHandoffStatus(name, h, status), `${words}: ${h.Subject || 'the request'}`, async (file) => {
      if (modal && shown?.meta.File === h.File && file) shown = await api.handoff(name, file).catch(() => null)
    })
  }
  function openFile(h: Handoff, reveal: boolean) {
    api.openHandoff(name, h.File, reveal).catch((e) => toast((e as Error).message, 'bad'))
  }
  async function copyPath(h: Handoff) {
    if (await copyText(h.File)) toast(`Copied ${h.File}`, 'ok')
    else toast("Couldn't copy the path", 'bad')
  }
  function askDelete(h: Handoff) {
    deleting = h
    deleteOpen = true
  }
  function del() {
    const h = deleting!
    deleteOpen = false
    void act(() => api.deleteHandoff(name, h), `Deleted: ${h.Subject || 'the request'}`, () => {
      if (shown?.meta.File === h.File) modal = false
    })
  }
  // The actions on a request, for its row's menu and its dialog.
  function actions(h: Handoff): MenuItem[] {
    const s = statusOf(h)
    return [
      ...(s === 'done' ? [{ label: 'Reopen', hint: 'back to its inbox', run: () => setStatus(h, 'open') }]
        : [{ label: 'Mark done', hint: 'moves it to done/', run: () => setStatus(h, 'done') },
          s === 'open' ? { label: 'Mark answered', run: () => setStatus(h, 'answered') } : { label: 'Mark open', run: () => setStatus(h, 'open') }]),
      'separator',
      { label: 'Open file', run: () => openFile(h, false) },
      { label: 'Show in Finder', run: () => openFile(h, true) },
      { label: 'Copy path', run: () => copyPath(h) },
      'separator',
      { label: 'Delete…', danger: true, run: () => askDelete(h) },
    ]
  }

  // A notification's click names the request it was about (?handoff=<file>): it opens, once the
  // workspace has loaded.
  let wanted = $state('')
  $effect(() => {
    const f = focus('handoff')
    if (f) wanted = f
  })
  $effect(() => {
    if (!wanted || !w) return
    const h = w.handoffs.find((x) => x.File === wanted)
    wanted = ''
    if (h) void show(h)
  })

  const grid = css({ display: 'grid', gridTemplateColumns: { base: '1fr', xl: '1fr 1fr' }, gap: '5', alignItems: 'start' })
  const full = css({ gridColumn: '1 / -1' })
  const kv = css({ display: 'grid', gridTemplateColumns: '{sizes.label} 1fr', rowGap: '2', px: '4', py: '3', '& dt': { color: 'fg.muted' } })
  const emptyRow = css({ px: '4', py: '4', color: 'fg.muted' })
  const hiddenList = css({ maxH: '260px', overflowY: 'auto', px: '4', py: '2', fontFamily: 'mono', fontSize: 'sm', '& div': { py: '0.5' } })
  const right = css({ ml: 'auto', display: 'flex', gap: '1.5', alignItems: 'center', textTransform: 'none', letterSpacing: 0 })
  const row = css({ cursor: 'pointer', _hover: { bg: 'bg.subtle' } })
  const projectChip = css({
    display: 'inline-flex', alignItems: 'center', h: 'control.xs', px: '2', mr: '1', borderRadius: 'control', fontSize: 'sm', fontWeight: 'strong',
    border: '1px solid {colors.border.strong}', bg: 'bg.panel', color: 'fg.default', cursor: 'pointer', _hover: { bg: 'bg.subtle' },
  })
  const nameCell = css({ display: 'inline-flex', alignItems: 'center', gap: '1.5', whiteSpace: 'nowrap', '& svg': { color: 'fg.muted' } })
  // Open / Answered / All: one choice of three, as a segmented control.
  const seg = css({ display: 'inline-flex', p: '0.5', gap: '0.5', bg: 'bg.subtle', borderRadius: 'control' })
  const segBtn = css({
    h: 'control.xs', px: '2.5', borderRadius: 'inner', fontSize: 'sm', fontWeight: 'medium', color: 'fg.muted', cursor: 'pointer',
    _hover: { color: 'fg.default' },
  })
  const segOn = css({ bg: 'bg.panel', color: 'fg.default', fontWeight: 'semibold', boxShadow: 'panel' })
  const prose = css({ display: 'grid', gap: '2.5', fontSize: 'base', lineHeight: 'code', '& p': { m: 0, whiteSpace: 'pre-wrap' } })
  const code = css({ fontFamily: 'mono', fontSize: 'sm', bg: 'bg.subtle', px: '1', borderRadius: 'sm' })
  const warnNote = css({ display: 'flex', gap: '2', alignItems: 'baseline', mb: '4', px: '3', py: '2', borderRadius: 'control', bg: 'warnSubtle', fontSize: 'md' })
  const metaList = css({ display: 'grid', gridTemplateColumns: '{sizes.labelNarrow} 1fr', rowGap: '1', fontSize: 'md', '& dt': { color: 'fg.muted' } })
  const select = css({ h: 'control.compact', px: '1.5', borderRadius: 'control', border: '1px solid {colors.border.strong}', bg: 'bg.panel', fontSize: 'sm' })
  const modalActions = css({ display: 'flex', gap: '2', alignItems: 'center', flexWrap: 'wrap', mt: '5', pt: '3', borderTop: '1px solid {colors.border.default}' })
  const modalRight = css({ ml: 'auto', display: 'flex', gap: '2', flexWrap: 'wrap' })
  const confirmBody = css({ display: 'grid', gap: '3', fontSize: 'md', lineHeight: 'normal' })
  const confirmFoot = css({ display: 'flex', gap: '2', justifyContent: 'flex-end', mt: '4' })
  const tone = (s: string) => (s === 'open' ? 'warn' : s === 'answered' ? 'ok' : 'neutral')
</script>

{#if error}<p class={muted}>{error}</p>{/if}
{#if w?.error}<p class={warnNote}><Icon name="alert" size={13} />{w.error}</p>{/if}
{#if w}
  <div class={grid}>
    <section class={panel}>
      <div class={panelHeader}><Icon name="workspaces" size={14} />{name}</div>
      <dl class={kv}>
        <dt>Root</dt><dd class={mono}>{w.config.root}</dd>
        <dt>Projects</dt>
        <dd>{#each w.projects as p}<button class={projectChip} title="Open {p}" onclick={() => go('projects', p)}>{p}</button>{:else}<span class={muted}>none</span>{/each}</dd>
        <dt>Instructions</dt>
        <dd>{#if w.config.root_notes}<span class={mono}>{w.config.root_notes}</span> <span class={muted}>· every session reads it</span>{:else}<span class={muted}>none</span>{/if}</dd>
        <dt>Handoffs</dt>
        <dd>{#if w.config.handoff}<span class={mono}>{w.config.handoff}</span> <span class={muted}>· where sessions leave requests for each other</span>{:else}<span class={muted}>off</span>{/if}</dd>
      </dl>
    </section>
    <section class={panel}>
      <div class={panelHeader}><Icon name="lock" size={14} />Members (read-only)</div>
      {#if w.members.length === 0}
        <div class={emptyRow}>None: add some in Settings.</div>
      {:else}
        <table class={table}>
          <tbody>
            {#each w.members as m (m.name)}
              <tr>
                <td><span class={nameCell}><Icon name="folder" size={12} />{m.name}</span></td>
                <td class="{mono} {muted}">{m.dir}</td>
                <td style="text-align:right">
                  {#if m.project}
                    <button class={projectChip} title="Open {m.project}" onclick={() => go('projects', m.project!)}>{m.project}</button>
                  {:else}
                    <button class={button({ size: 'sm' })} title="Add this folder as a project of its own" onclick={() => go('new-project', m.dir)}>
                      <Icon name="plus" size={12} />Add as project
                    </button>
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      {/if}
    </section>
    <section class="{panel} {full}">
      <div class={panelHeader}>
        <Icon name="inbox" size={14} />Handoffs
        <span class={right}>
          <span class={muted}>{openCount} open</span>
          <span class={seg} role="radiogroup" aria-label="Show">
            {#each filters as f (f.id)}
              <button class="{segBtn} {filter === f.id ? segOn : ''}" role="radio" aria-checked={filter === f.id} onclick={() => (filter = f.id)}>{f.label}</button>
            {/each}
          </span>
        </span>
      </div>
      {#if handoffs.length === 0}
        <div class={emptyRow}>{w.config.handoff ? 'No requests here.' : 'Handoffs are off: set a handoff folder in Settings.'}</div>
      {:else}
        <table class={table}>
          <thead><tr><th>Status</th><th>From</th><th>To</th><th>Subject</th><th>Created</th><th></th></tr></thead>
          <tbody>
            {#each handoffs as h (h.File)}
              <tr class={row} style={h.Done ? 'opacity:.55' : ''} onclick={() => show(h)}>
                <td><span class={badge({ tone: tone(h.Status) })}>{h.Done ? 'done' : h.Status || '–'}</span></td>
                <td>{h.From}</td>
                <td>{h.Inbox}</td>
                <td>{h.Subject}</td>
                <td style="white-space:nowrap">{h.Created}</td>
                <td style="text-align:right">
                  <Menu title="Actions for {h.Subject || 'this request'}" disabled={busy} class={button({ size: 'sm' })} items={actions(h)} />
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      {/if}
    </section>
    <section class="{panel} {full}">
      <div class={panelHeader}>
        <Icon name="eyeoff" size={14} />Hidden in members
        <span class={right}>
          {#if w.projects.length > 1}
            <label class={muted} for="seen-by">as</label>
            <select id="seen-by" class={select} value={w.project} onchange={(e) => (project = e.currentTarget.value)}>
              {#each w.projects as p}<option value={p}>{p}</option>{/each}
            </select>
            <span class={muted}>sees them ·</span>
          {/if}
          <span class={muted}>{w.hidden.length} {w.hidden.length === 1 ? 'path' : 'paths'}</span>
        </span>
      </div>
      {#if w.hidden.length === 0}
        <div class={emptyRow}>Nothing hidden beyond the rules below.</div>
      {:else}
        <div class={hiddenList}>{#each w.hidden as h}<div>{h}</div>{/each}</div>
      {/if}
      <div class={emptyRow} style="font-size:var(--font-sizes-sm)">Rules: <span class={mono}>{w.rules.join(' ')}</span></div>
    </section>
  </div>
{/if}

<Modal bind:open={modal} title={shown?.meta.Subject || 'Request'}>
  {#if shown}
    <dl class={metaList}>
      <dt>From</dt><dd>{shown.meta.From || '–'}</dd>
      <dt>To</dt><dd>{shown.meta.Inbox || '–'}</dd>
      <dt>Status</dt><dd>{shown.meta.Done ? 'done' : shown.meta.Status || '–'}</dd>
      <dt>Created</dt><dd>{shown.meta.Created || '–'}</dd>
      <dt>File</dt><dd class={mono} style="overflow-wrap:anywhere;user-select:all">{shown.path}</dd>
    </dl>
    <div class={prose}>
      {#each paragraphs(shown.body) as parts}
        <p>{#each parts as t}{#if t.startsWith('`') && t.endsWith('`') && t.length > 2}<code class={code}>{t.slice(1, -1)}</code>{:else}{t}{/if}{/each}</p>
      {/each}
    </div>
    {@const s = statusOf(shown.meta)}
    <div class={modalActions}>
      {#if s === 'done'}
        <button class={button({ size: 'sm' })} disabled={busy} onclick={() => setStatus(shown!.meta, 'open')}><Icon name="restart" size={12} />Reopen</button>
      {:else}
        <button class={button({ size: 'sm', variant: 'primary' })} disabled={busy} onclick={() => setStatus(shown!.meta, 'done')}><Icon name="check" size={12} />Mark done</button>
        <button class={button({ size: 'sm' })} disabled={busy} onclick={() => setStatus(shown!.meta, s === 'open' ? 'answered' : 'open')}>
          {s === 'open' ? 'Mark answered' : 'Mark open'}
        </button>
      {/if}
      <span class={modalRight}>
        <button class={button({ size: 'sm' })} onclick={() => openFile(shown!.meta, false)}>Open file</button>
        <button class={button({ size: 'sm' })} onclick={() => openFile(shown!.meta, true)}>Show in Finder</button>
        <button class={button({ size: 'sm', variant: 'danger' })} disabled={busy} onclick={() => askDelete(shown!.meta)}>Delete…</button>
      </span>
    </div>
  {/if}
</Modal>

<Modal bind:open={deleteOpen} title="Delete this request?">
  <div class={confirmBody}>
    <p><strong>{deleting?.Subject || 'The request'}</strong> <span class={muted}>· from {deleting?.From || '–'} to {deleting?.Inbox || '–'}</span></p>
    <p>Its file is deleted, with any reply in it. To keep it as a record, mark it done instead: it moves to <span class={mono}>done/</span>.</p>
  </div>
  <div class={confirmFoot}>
    <button class={button()} onclick={() => (deleteOpen = false)}>Cancel</button>
    <button class={button({ variant: 'danger' })} onclick={del}>Delete</button>
  </div>
</Modal>
