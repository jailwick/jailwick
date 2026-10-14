<script lang="ts">
  import { api, type Image } from '../lib/api'
  import { ago } from '../lib/format'
  import { badge, button, panel, table, mono, muted } from '../lib/styles'
  import Page from '../lib/Page.svelte'
  import { toast } from '../components/Toasts.svelte'

  let images = $state<Image[] | null>(null)
  let error = $state('')
  let pruning = $state(false)

  const unused = (i: Image) => i.projects.length === 0 && !i.containers
  const unusedCount = $derived((images ?? []).filter(unused).length)

  function load() {
    api.images().then((i) => (images = i)).catch((e) => (error = e.message))
  }
  load()

  async function prune() {
    pruning = true
    try {
      const { removed } = await api.pruneImages()
      toast(removed.length ? `Removed ${removed.length} unused image${removed.length === 1 ? '' : 's'}` : 'Nothing to remove', 'ok')
    } catch (e) {
      toast((e as Error).message, 'bad')
    } finally {
      pruning = false
      load()
    }
  }
</script>

<Page title="Images" subtitle="Sandbox and proxy images built by jailwick, the base images projects pull, and what uses them.">
  {#snippet actions()}
    <button class={button()} disabled={pruning || unusedCount === 0} onclick={prune}>
      {pruning ? 'Removing…' : `Remove unused${unusedCount ? ` (${unusedCount})` : ''}`}
    </button>
  {/snippet}
  {#if error}
    <p class={muted}>{error}</p>
  {:else if images === null}
    <p class={muted}>Loading…</p>
  {:else if images.length === 0}
    <section class={panel} style="padding:var(--spacing-7);text-align:center"><p class={muted}>No images yet; the first jw up builds them.</p></section>
  {:else}
    <section class={panel}>
      <table class={table}>
        <thead><tr><th>Image</th><th>Used by</th><th>ID</th><th>Size</th><th>Created</th></tr></thead>
        <tbody>
          {#each images as i (i.Ref + i.ID)}
            <tr>
              <td class={mono}>
                {i.Ref}
                {#if i.pulled}<span class={badge()} title="A project's own image from a registry: kept while a project uses it">pulled</span>{/if}
              </td>
              <td>
                {#if i.projects.length}{i.projects.join(', ')}{:else if i.containers}<span class={muted}>{i.Ref.includes('proxy') ? 'every project (the egress proxy)' : 'a container'}</span>{:else}<span class={badge()}>unused</span>{/if}
              </td>
              <td class="{mono} {muted}">{i.ID}</td>
              <td>{i.Size}</td>
              <td class={muted} style="white-space:nowrap" title={i.Created}>{ago(i.Created) === '–' ? i.Created : `${ago(i.Created)} ago`}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </section>
  {/if}
</Page>
