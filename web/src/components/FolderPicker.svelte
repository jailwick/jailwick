<script lang="ts">
  import { api, ApiError } from '../lib/api'
  import { button, input, mono } from '../lib/styles'
  import Icon from './Icon.svelte'
  import { toast } from './Toasts.svelte'

  // A folder of this machine: typed, or chosen in the system's dialog. Without a dialog (501, e.g.
  // a browser on another machine) the path is typed instead. onchange gets each new path; picked
  // is true when it came from the dialog (no need to wait for typing to stop).
  let {
    value = $bindable(''),
    title,
    placeholder = '~/code/my-app',
    onchange,
  }: { value?: string; title: string; placeholder?: string; onchange: (path: string, picked: boolean) => void } = $props()

  let picking = $state(false)
  let noPicker = $state(false)

  async function choose() {
    picking = true
    try {
      const { path } = await api.pickFolder(value.trim(), title)
      if (!path) return
      value = path
      onchange(path, true)
    } catch (e) {
      if (e instanceof ApiError && e.status === 501) noPicker = true
      toast((e as Error).message, 'bad')
    } finally {
      picking = false
    }
  }
</script>

<div style="display:flex;gap:var(--spacing-2)">
  <input
    id="dir"
    class="{input} {mono}"
    style="flex:1;min-width:0"
    {placeholder}
    autocomplete="off"
    bind:value
    oninput={() => onchange(value, false)}
  />
  {#if !noPicker}
    <button class={button()} disabled={picking} onclick={choose}><Icon name="folder" size={13} />Choose…</button>
  {/if}
</div>
