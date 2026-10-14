<script lang="ts">
  import { css } from 'styled-system/css'
  import { api, type UpdateRun } from '../../lib/api'
  import { ago, when } from '../../lib/format'
  import { go } from '../../lib/router.svelte'
  import { updates, loadUpdates } from '../../lib/updates.svelte'
  import { badge, button, code, link, linkButton, mono, muted, panel, panelHeader } from '../../lib/styles'
  import Icon from '../../components/Icon.svelte'
  import { toast } from '../../components/Toasts.svelte'

  // Whether a newer jailwick is out, and updating it: with Homebrew where it came from Homebrew
  // (the CLI and the app together), else from the release page. Nothing restarts by itself.
  const st = $derived(updates.status)
  let checking = $state(false)
  let running = $state(false)
  let result = $state<UpdateRun | null>(null)

  async function check() {
    checking = true
    try {
      const s = await loadUpdates(true)
      if (s.error) toast(`Couldn't check: ${s.error}`, 'bad')
      else if (!s.available) toast(`jailwick ${s.current} is the latest version`, 'ok')
    } catch (e) {
      toast((e as Error).message, 'bad')
    } finally {
      checking = false
    }
  }

  async function update() {
    running = true
    result = null
    try {
      result = await api.runUpdate()
    } catch (e) {
      result = { ok: false, output: '', error: (e as Error).message }
    } finally {
      running = false
    }
  }

  const head = css({ ml: 'auto', textTransform: 'none', letterSpacing: 0, fontWeight: 'normal' })
  const kv = css({
    display: 'grid', gridTemplateColumns: '{sizes.label} 1fr', rowGap: '2', columnGap: '4', px: '4', py: '3',
    '& dt': { color: 'fg.muted' }, '& dd': { minW: 0, wordBreak: 'break-word' },
  })
  const body = css({ display: 'grid', gap: '3', px: '4', pb: '4' })
  const notes = css({
    m: 0, p: '3', maxH: '220px', overflow: 'auto', whiteSpace: 'pre-wrap', fontSize: 'sm',
    bg: 'bg.subtle', borderRadius: 'md', borderWidth: '1px', borderColor: 'border',
  })
  const actions = css({ display: 'flex', gap: '3', alignItems: 'center', flexWrap: 'wrap' })
  const output = css({
    m: 0, p: '3', maxH: '260px', overflow: 'auto', whiteSpace: 'pre-wrap', fontFamily: 'mono', fontSize: 'xs',
    bg: 'bg.subtle', borderRadius: 'md', borderWidth: '1px', borderColor: 'border',
  })
  const ok = css({ color: 'ok' })
  const bad = css({ color: 'bad' })
</script>

<section class={panel}>
  <div class={panelHeader}>
    <Icon name="refresh" size={14} />Updates
    <span class={head}>
      <button class={button({ size: 'sm', variant: 'ghost' })} disabled={checking} onclick={check}>
        <Icon name={checking ? 'spinner' : 'refresh'} size={12} />Check now
      </button>
    </span>
  </div>
  {#if !st}
    <p class="{muted} {css({ px: '4', py: '3' })}">Looking…</p>
  {:else}
    <dl class={kv}>
      <dt>This version</dt><dd>{st.current}</dd>
      <dt>Latest</dt>
      <dd>
        {#if st.latest}
          {st.latest.version}
          {#if st.available}<span class={badge({ tone: 'info' })}>new</span>{:else}<span class={muted}>· up to date</span>{/if}
          <span class={muted}>· released {when(st.latest.published)}</span>
        {:else}
          <span class={muted}>{st.error ? `couldn't check: ${st.error}` : 'not checked yet'}</span>
        {/if}
      </dd>
      <dt>Checked</dt>
      <dd>
        {st.checked ? `${ago(st.checked)} ago` : 'never'}
        {#if st.off}<span class={muted}>· the daily check is off (<button class={linkButton} onclick={() => go('settings')}>Settings</button>)</span>{/if}
        {#if st.error && st.latest}<span class={muted} title={st.error}>· the last check failed</span>{/if}
      </dd>
      <dt>Installed with</dt><dd>{st.install.method === 'homebrew' ? 'Homebrew' : 'a download, or built from source'}</dd>
    </dl>

    {#if st.available && st.latest}
      <div class={body}>
        {#if st.latest.notes}<pre class={notes}>{st.latest.notes}</pre>{/if}
        <div class={actions}>
          {#if st.install.method === 'homebrew'}
            <button class={button({ variant: 'primary' })} disabled={running} onclick={update}>
              <Icon name={running ? 'spinner' : 'refresh'} size={14} />{running ? 'Updating…' : `Update to ${st.latest.version}`}
            </button>
            <span class={muted}>runs <span class={code}>{st.install.command}</span></span>
          {:else}
            <a class={button({ variant: 'primary' })} href={st.latest.url} target="_blank" rel="noreferrer">
              <Icon name="external" size={14} />Download {st.latest.version}
            </a>
          {/if}
          <a class={link} href={st.latest.url} target="_blank" rel="noreferrer">What's new</a>
        </div>
        {#if result}
          {#if result.ok}
            <p class={ok}>Updated. Quit and reopen jailwick to use {st.latest.version}; running sandboxes take the new helper at their next restart.</p>
          {:else}
            <p class={bad}>{result.error}</p>
          {/if}
          {#if result.output}<pre class={output}>{result.output}</pre>{/if}
        {/if}
      </div>
    {:else if st.install.method === 'download'}
      <p class="{muted} {css({ px: '4', pb: '3', fontSize: 'sm' })}">
        Installed with Homebrew (<span class={mono}>brew install jailwick/tap/jailwick</span>), jailwick updates itself from here.
      </p>
    {/if}
  {/if}
</section>
