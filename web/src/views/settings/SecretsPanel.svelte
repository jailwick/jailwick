<script lang="ts">
  import { css } from 'styled-system/css'
  import { api, type Secrets } from '../../lib/api'
  import { panel, panelHeader, table, mono, muted, badge, button, input } from '../../lib/styles'
  import Icon from '../../components/Icon.svelte'
  import ConfirmBar from '../../components/ConfirmBar.svelte'
  import { toast } from '../../components/Toasts.svelte'

  // Secrets for all projects or one: set (the value goes to the secret store and is never shown
  // again) and removed. Sandboxes get them as environment variables at their next restart.
  let { projects }: { projects: string[] } = $props()

  let s = $state<Secrets | null>(null)
  const load = () => api.secrets().then((v) => (s = v)).catch((e) => toast((e as Error).message, 'bad'))
  load()

  let name = $state('')
  let value = $state('')
  let scope = $state('') // '' for all projects
  let saving = $state(false)
  let removing = $state<{ name: string; project?: string } | null>(null)

  const replacesLogin = $derived(!!s?.overrides.includes(name.trim().toUpperCase()))

  async function set(e: SubmitEvent) {
    e.preventDefault()
    saving = true
    try {
      await api.setSecret(name.trim(), value, scope)
      toast(`${name.trim().toUpperCase()} saved for ${scope || 'all projects'}; sandboxes get it at their next restart`, 'ok')
      name = value = ''
      load()
    } catch (err) {
      toast((err as Error).message, 'bad')
    } finally {
      saving = false
    }
  }

  async function remove(r: { name: string; project?: string }) {
    removing = null
    try {
      await api.unsetSecret(r.name, r.project ?? '')
      toast(`${r.name} removed from ${r.project ?? 'all projects'}`, 'ok')
      load()
    } catch (err) {
      toast((err as Error).message, 'bad')
    }
  }

  // Wraps in a narrow panel: name and value share a line, scope and Set the next one if needed.
  const form = css({
    display: 'flex', flexWrap: 'wrap', gap: '2', p: '3', borderBottom: '1px solid {colors.border.default}',
    '& > input': { flex: '1 1 160px', minW: 0 }, '& > select': { flex: '1 1 140px', minW: 0 },
  })
  const note = css({ px: '4', py: '2.5', color: 'fg.muted', fontSize: 'sm' })
  const warn = css({ px: '4', pb: '2.5', color: 'warn', fontSize: 'sm' })
  const emptyRow = css({ px: '4', py: '3', color: 'fg.muted' })
</script>

<section class={panel}>
  <div class={panelHeader}><Icon name="lock" size={14} />Secrets{#if s}<span style="margin-left:auto;text-transform:none;letter-spacing:0">{s.store}</span>{/if}</div>
  {#if s?.read_only}
    <p class={note}>Secrets come from environment variables (<span class={mono}>JAILWICK_SECRET_&lt;NAME&gt;</span>), so they are set there, not here.</p>
  {:else}
    <form class={form} onsubmit={set}>
      <input class={input} list="known-secrets" placeholder="NAME" bind:value={name} autocomplete="off" required />
      <datalist id="known-secrets">{#each s?.known ?? [] as k}<option value={k}></option>{/each}</datalist>
      <input class={input} type="password" placeholder="value (hidden, never shown again)" bind:value={value} autocomplete="off" required />
      <select class={input} bind:value={scope} aria-label="For which projects">
        <option value="">All projects</option>
        {#each projects as p}<option value={p}>{p}</option>{/each}
      </select>
      <button class={button({ variant: 'primary' })} type="submit" disabled={saving || !name.trim() || !value}>{saving ? 'Saving…' : 'Set'}</button>
    </form>
    {#if replacesLogin}
      <p class={warn}>{name.trim().toUpperCase()} replaces Claude's login: chat works, but Remote Control and Claude in Chrome need the login (<span class={mono}>jw login</span>).</p>
    {/if}
  {/if}
  {#if removing}
    <ConfirmBar message={`Remove ${removing.name} from ${removing.project ?? 'all projects'}?`} action="Remove" danger
      onconfirm={() => removing && remove(removing)} oncancel={() => (removing = null)} />
  {/if}
  {#if s && s.secrets.length === 0}
    <div class={emptyRow}>No secrets set.</div>
  {:else if s}
    <table class={table}>
      <thead><tr><th>Name</th><th>For</th><th></th></tr></thead>
      <tbody>
        {#each s.secrets as r (r.name + (r.project ?? ''))}
          <tr>
            <td class={mono}>
              {r.name}
              {#if s.overrides.includes(r.name)}<span class={badge({ tone: 'warn' })} title="Replaces Claude's login: no Remote Control or Claude in Chrome">replaces login</span>{/if}
            </td>
            <td>{r.project ?? 'all projects'}</td>
            <td style="text-align:right">
              {#if !s.read_only}
                <button class={button({ size: 'sm', variant: 'danger' })} onclick={() => (removing = r)}><Icon name="x" size={12} />Remove</button>
              {/if}
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}
  <p class={note}>Values never leave the secret store for this page. Claude's login is done with <span class={mono}>jw login</span>.</p>
</section>
