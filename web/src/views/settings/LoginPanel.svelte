<script lang="ts">
  import { css } from 'styled-system/css'
  import { api, type Login, type Settings } from '../../lib/api'
  import { panel, panelHeader, table, mono, muted, input } from '../../lib/styles'
  import Icon from '../../components/Icon.svelte'
  import { toast } from '../../components/Toasts.svelte'

  // Where Claude's login is kept: the app's (one sign-in shared by projects) or a project's own.
  // A default for every project, and each project's choice.
  let { s, onchange }: { s: Settings; onchange: () => void } = $props()
  let busy = $state('')

  const label = (l: Login) => (l === 'app' ? "The app's (shared)" : "Each project's own")

  async function setDefault(login: Login) {
    if (login === s.login_default) return
    busy = '*'
    try {
      await api.saveDefaultLogin(login)
      toast('Saved; restarting a project applies it', 'ok')
      onchange()
    } catch (e) {
      toast((e as Error).message, 'bad')
    } finally {
      busy = ''
    }
  }

  async function setProject(project: string, login: Login | '') {
    busy = project
    try {
      await api.saveClaude(project, { login })
      toast(`Saved for ${project}; its next restart applies it`, 'ok')
      onchange()
    } catch (e) {
      toast((e as Error).message, 'bad')
    } finally {
      busy = ''
    }
  }

  const body = css({ display: 'grid', gap: '2', px: '4', py: '3.5' })
  const opts = css({ display: 'flex', flexWrap: 'wrap', gap: '1.5' })
  const opt = css({
    display: 'inline-flex', alignItems: 'center', px: '2.5', h: 'control.icon', borderRadius: 'control', cursor: 'pointer',
    border: '1px solid {colors.border.strong}', _hover: { bg: 'bg.subtle' }, _disabled: { opacity: 0.5, cursor: 'default' },
  })
  const optOn = css({ borderColor: 'selected.border', bg: 'selected.bg', color: 'selected.fg' })
  const hint = css({ color: 'fg.muted', fontSize: 'sm' })
</script>

<section class={panel}>
  <div class={panelHeader}><Icon name="lock" size={14} />Claude login</div>
  <div class={body}>
    <strong>Default for projects</strong>
    <div class={opts} role="radiogroup" aria-label="Default Claude login">
      {#each ['app', 'project'] as const as l}
        <button class="{opt} {s.login_default === l ? optOn : ''}" role="radio" aria-checked={s.login_default === l}
          disabled={!!busy} onclick={() => setDefault(l)}>{label(l)}</button>
      {/each}
    </div>
    <span class={hint}>
      Sign in with <span class={mono}>jw login</span> in a project: once for every project on the app's login.
      A change applies when a project restarts.
    </span>
  </div>
  {#if s.logins.length}
    <table class={table}>
      <thead><tr><th>Project</th><th>Login</th></tr></thead>
      <tbody>
        {#each s.logins as l (l.project)}
          <tr>
            <td>{l.project}</td>
            <td>
              <select class={input} value={l.login} disabled={busy === l.project} aria-label="Claude login of {l.project}"
                onchange={(e) => setProject(l.project, (e.currentTarget as HTMLSelectElement).value as Login | '')}>
                <option value="">Default ({s.login_default === 'app' ? "the app's" : 'its own'})</option>
                <option value="app">The app's (shared)</option>
                <option value="project">Its own</option>
              </select>
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}
</section>
