<script lang="ts">
  import { css } from 'styled-system/css'
  import { api, type BlockedList, type Denied } from '../../lib/api'
  import { when } from '../../lib/format'
  import { poll } from '../../lib/poll.svelte'
  import { focus } from '../../lib/router.svelte'
  import { tick as rendered } from 'svelte'
  import { button, linkButton, mono, muted, panel, panelHeader, pointedOut, table, panelCount as count } from '../../lib/styles'
  import ConfirmBar from '../../components/ConfirmBar.svelte'
  import Icon from '../../components/Icon.svelte'
  import Menu from '../../components/Menu.svelte'
  import SplitButton from '../../components/SplitButton.svelte'
  import { toast } from '../../components/Toasts.svelte'

  // What a project's sandboxes were refused, to allow from here. The server keeps it (the
  // project's records), so it outlives restarts, stops and reinstalls, and shows while the project
  // is stopped too. Ten hosts at first, ten more at a time. Discarding clears hosts from the list
  // until the sandbox asks for them again; blocking keeps them blocked and off the list for good
  // (the project's `blocked`, undone below the list). Both are kept by the server.
  // Ticked hosts get the actions above the list; with none ticked and more than ten listed, they
  // apply to every host listed, not only those shown.
  // A notification's click names the hosts it was about (?hosts=): they come ticked, so the
  // actions above the list act on them, and marked; the list is scrolled to, actions in sight.
  let { name, running, onchange }: { name: string; running: boolean; onchange?: () => void } = $props()

  const page = 10
  let limit = $state(page)
  let showKept = $state(false)
  let keptLimit = $state(page)
  let asking = $state<{ message: string; action: string; danger?: boolean; run: () => void } | null>(null)
  let busy = $state(false)

  // Read again every 5 s while it runs (its proxies log as it goes), every 30 s while it doesn't.
  let blocked = $state.raw<ReturnType<typeof poll<BlockedList>> | null>(null)
  $effect(() => {
    const p = poll(() => api.blocked(name), running ? 5000 : 30000)
    blocked = p
    return () => p.stop()
  })
  const data = $derived(blocked?.state.data ?? (blocked?.state.error ? { hosts: [], discarded: [], kept: [], by: {} } : null))

  const hosts = $derived(data?.hosts ?? [])
  const gone = $derived(data?.discarded ?? []) // discarded, not asked for again since
  const shown = $derived(hosts.slice(0, limit))
  const kept = $derived(data?.kept ?? [])
  const many = $derived(hosts.length > page)

  // Ticked hosts. Only those still listed count: an allowed, discarded or blocked one drops out.
  let ticked = $state<string[]>([])
  const selected = $derived(hosts.filter((b) => ticked.includes(b.Host)))
  const allTicked = $derived(hosts.length > 0 && selected.length === hosts.length)
  function tick(h: string, on: boolean) {
    ticked = on ? [...ticked, h] : ticked.filter((t) => t !== h)
  }

  // The hosts a notification named, until the list has loaded to point them out.
  let wanted = $state<string[]>([])
  let section: HTMLElement | undefined = $state()
  let marked = $state<string[]>([])
  $effect(() => {
    const h = focus('hosts')
    if (h) wanted = h.split(',').filter(Boolean)
  })
  $effect(() => {
    if (!wanted.length || !data) return
    const named = hosts.filter((b) => wanted.includes(b.Host)).map((b) => b.Host)
    wanted = []
    if (!named.length) return // allowed or cleared since
    ticked = named
    marked = named
    limit = Math.max(limit, ...named.map((h) => hosts.findIndex((b) => b.Host === h) + 1))
    void rendered().then(() => section?.scrollIntoView({ block: 'start', behavior: 'smooth' }))
  })

  const selectAll = () => (ticked = hosts.map((b) => b.Host))
  const deselectAll = () => (ticked = [])
  // What the actions above the list act on, and how they name it.
  const target = $derived(selected.length ? selected : hosts)
  const which = $derived(selected.length ? 'selected' : 'all')

  async function act(fn: () => Promise<unknown>, done: string) {
    busy = true
    asking = null
    try {
      await fn()
      toast(done, 'ok')
      await blocked?.refresh()
      onchange?.()
    } catch (e) {
      toast((e as Error).message, 'bad')
    } finally {
      busy = false
    }
  }
  const n = (list: unknown[]) => `${list.length} host${list.length === 1 ? '' : 's'}`

  function allow(list: string[], everywhere: boolean) {
    act(() => api.allowMany(list, everywhere ? '' : name), `${list.length === 1 ? list[0] : n(list)} allowed ${everywhere ? 'for every project' : `for ${name}`}`)
  }
  function discard(list: Denied[]) {
    act(() => api.discard(name, list.map((b) => ({ host: b.Host, last: b.Last }))),
      `${list.length === 1 ? list[0].Host : n(list)} discarded: listed again if the sandbox asks for it again`)
  }
  function bringBack() {
    act(() => api.discard(name, gone.map((b) => ({ host: b.Host })), true), `${n(gone)} back on the list`)
  }
  function block(list: string[]) {
    act(() => api.block(name, list), `${list.length === 1 ? list[0] : n(list)} kept blocked: no longer listed`)
  }
  function unblock(list: string[]) {
    act(() => api.block(name, list, true), `${list.length === 1 ? list[0] : n(list)} listed again when refused`)
  }

  // The broad ones ask first.
  function allowEverywhere(list: string[]) {
    asking = {
      message: `Allow ${which === 'all' ? 'all ' : 'the selected '}${n(list)} for every project? Each sandbox will be able to reach them.`,
      action: 'Allow everywhere',
      run: () => allow(list, true),
    }
  }
  function blockMany(list: string[]) {
    asking = {
      message: `Keep ${which === 'all' ? 'all ' : 'the selected '}${n(list)} blocked? They are no longer listed; you can unblock them below.`,
      action: which === 'all' ? 'Block all' : 'Block selected', danger: true, run: () => block(list),
    }
  }

  const bulk = css({ display: 'flex', gap: '2', alignItems: 'center', flexWrap: 'wrap', px: '4', py: '2.5', borderBottom: '1px solid {colors.border.default}' })
  const emptyRow = css({ px: '4', py: '4', color: 'fg.muted' })
  const footer = css({
    display: 'flex', gap: '3', alignItems: 'center', flexWrap: 'wrap', px: '4', py: '2.5', fontSize: 'md', color: 'fg.muted',
    borderTop: '1px solid {colors.border.default}',
  })
  const confirmWrap = css({ px: '4', pb: '3', '& > *': { mt: '3' } })
  const keptHead = css({ display: 'flex', alignItems: 'center', gap: '3', px: '4', py: '2', bg: 'bg.inset', fontSize: 'md', color: 'fg.muted', borderTop: '1px solid {colors.border.default}' })
  const num = css({ fontVariantNumeric: 'tabular-nums' })
  const bulkActions = css({ ml: 'auto', display: 'flex', gap: '2', alignItems: 'center', flexWrap: 'wrap' })
  const check = css({ w: '1px', pr: '0!', '& input': { cursor: 'pointer', verticalAlign: 'middle' } })
  const rowOn = css({ '& td': { bg: 'bg.subtle' } }) // lighter than the dividers, which stay visible
  const actions = css({ textAlign: 'right', whiteSpace: 'nowrap' })
</script>

<section class={panel} bind:this={section}>
  <div class={panelHeader}>
    <Icon name="alert" size={14} />Blocked requests{#if hosts.length}<span class={count}>{n(hosts)}</span>{/if}
  </div>
  {#if data === null}
    <div class={emptyRow}>Loading…</div>
  {:else}
    {#if hosts.length > 1}
      <div class={bulk}>
        <button class={button({ size: 'sm' })} disabled={allTicked} onclick={selectAll}>Select all ({hosts.length})</button>
        <button class={button({ size: 'sm' })} disabled={!selected.length} onclick={deselectAll}>Deselect all</button>
        {#if selected.length || many}
          <!-- On the right, in line with each row's own buttons. -->
          <span class={bulkActions}>
          <span class={muted}>{selected.length ? `${n(selected)} selected:` : `All ${n(hosts)}:`}</span>
          <SplitButton onclick={() => allow(target.map((b) => b.Host), false)} disabled={busy} title="Allow them for {name}"
            more="More ways to allow them" items={[{ label: `Allow ${which} everywhere`, run: () => allowEverywhere(target.map((b) => b.Host)) }]}>
            <Icon name="check" size={12} />Allow {which}
          </SplitButton>
          <SplitButton onclick={() => discard(target)} disabled={busy} title="Clear them from the list until the sandbox asks for them again"
            more="More ways to clear them" items={[{ label: `Block ${which}`, danger: true, run: () => blockMany(target.map((b) => b.Host)) }]}>
            <Icon name="x" size={12} />Discard {which}
          </SplitButton>
          </span>
        {/if}
      </div>
    {/if}
    {#if asking}
      <div class={confirmWrap}>
        <ConfirmBar message={asking.message} action={asking.action} danger={asking.danger} onconfirm={asking.run} oncancel={() => (asking = null)} />
      </div>
    {/if}
    {#if hosts.length === 0}
      <div class={emptyRow}>
        {gone.length ? 'Nothing new since you discarded the list.' : `Nothing blocked: every host the sandbox asked for is on the allowlist${kept.length ? ', or kept blocked' : ''}.`}
        {#if !running}<span> The sandbox is stopped: what it was refused before stays listed here.</span>{/if}
      </div>
    {:else}
      <table class={table}>
        <thead>
          <tr>
            <th class={check}>
              <input type="checkbox" aria-label={allTicked ? 'Deselect all' : 'Select all'} checked={allTicked}
                indeterminate={selected.length > 0 && !allTicked} onchange={(e) => (e.currentTarget.checked ? selectAll() : deselectAll())} />
            </th>
            <th>Host</th><th>Requests</th><th>Last</th><th></th>
          </tr>
        </thead>
        <tbody>
          {#each shown as b (b.Host)}
            <tr class="{ticked.includes(b.Host) ? rowOn : ''} {marked.includes(b.Host) ? pointedOut : ''}">
              <td class={check}>
                <input id="pick-{name}-{b.Host}" type="checkbox" checked={ticked.includes(b.Host)} onchange={(e) => tick(b.Host, e.currentTarget.checked)} />
              </td>
              <td class={mono}><label for="pick-{name}-{b.Host}" style="cursor:pointer">{b.Host}</label></td>
              <td class={num}>{b.Count}</td>
              <td class={muted} style="white-space:nowrap">{when(b.Last)}</td>
              <td class={actions}>
                <button class={button({ size: 'sm' })} disabled={busy} onclick={() => allow([b.Host], false)}>Allow for {name}</button>
                <Menu title="More for {b.Host}" disabled={busy} items={[
                  { label: 'Allow everywhere', run: () => allow([b.Host], true) },
                  'separator',
                  { label: 'Discard', hint: 'until asked again', run: () => discard([b]) },
                  { label: 'Block', hint: 'never list it', danger: true, run: () => block([b.Host]) },
                ]} class={button({ size: 'sm' })} />
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}
    {#if hosts.length > limit || kept.length || gone.length}
      <div class={footer}>
        {#if hosts.length > limit}
          <button class={button({ size: 'sm' })} onclick={() => (limit += page)}>
            <Icon name="chevronDown" size={12} />Show {Math.min(page, hosts.length - limit)} more
          </button>
          <span>{shown.length} of {hosts.length} shown</span>
        {/if}
        {#if gone.length}
          <span style="margin-left:auto">
            {n(gone)} discarded ·
            <button class={linkButton} onclick={bringBack}>Bring back</button>
          </span>
        {/if}
        {#if kept.length}
          <span style={gone.length ? '' : 'margin-left:auto'}>
            {n(kept)} kept blocked ·
            <button class={linkButton} onclick={() => (showKept = !showKept)}>{showKept ? 'Hide' : 'Show'}</button>
          </span>
        {/if}
      </div>
      {#if showKept && kept.length}
        <div class={keptHead}>
          Kept blocked: refused, and not listed or notified about.
          {#if kept.length > 1}
            <button class={button({ size: 'sm' })} style="margin-left:auto" disabled={busy} onclick={() => unblock(kept)}>Unblock all</button>
          {/if}
        </div>
        <table class={table}>
          <tbody>
            {#each kept.slice(0, keptLimit) as h (h)}
              <tr>
                <td class={mono}>{h}</td>
                <td class={actions}>
                  <button class={button({ size: 'sm' })} disabled={busy} onclick={() => unblock([h])}>Unblock</button>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
        {#if kept.length > keptLimit}
          <div class={footer}>
            <button class={button({ size: 'sm' })} onclick={() => (keptLimit += page)}>
              <Icon name="chevronDown" size={12} />Show {Math.min(page, kept.length - keptLimit)} more
            </button>
            <span>{keptLimit} of {kept.length} shown</span>
          </div>
        {/if}
      {/if}
    {/if}
  {/if}
</section>
