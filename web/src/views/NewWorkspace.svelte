<script lang="ts">
  import { css } from 'styled-system/css'
  import { api } from '../lib/api'
  import { needRestart } from '../lib/restarts.svelte'
  import { go } from '../lib/router.svelte'
  import { SetupForm } from '../lib/setupForm.svelte'
  import { mono, muted } from '../lib/styles'
  import Page from '../lib/Page.svelte'
  import FolderPicker from '../components/FolderPicker.svelte'
  import SetupWizard from '../components/SetupWizard.svelte'
  import { toast } from '../components/Toasts.svelte'

  // The guided "new workspace" flow: the folder that holds the repos, then the server's steps
  // (members, projects, sharing), then a review.
  let { oncreated }: { oncreated: () => void } = $props()

  let root = $state('')
  const form = new SetupForm((values) => api.draftWorkspace({ root }, values))
  let creating = $state(false)

  let timer: ReturnType<typeof setTimeout> | undefined
  function folderChanged(_: string, picked: boolean) {
    clearTimeout(timer)
    form.clear()
    if (root.trim()) timer = setTimeout(() => form.start(), picked ? 0 : 350)
  }

  async function create() {
    creating = true
    try {
      const { name = '', changed } = await api.createWorkspace(root, form.values)
      needRestart(changed)
      toast(`Created ${name}`, 'ok')
      oncreated()
      go('workspaces', name)
    } catch (e) {
      toast((e as Error).message, 'bad')
      await form.refresh()
    } finally {
      creating = false
    }
  }

  const bad = css({ color: 'bad' })
</script>

<Page title="New workspace" subtitle="Group related repos: each project sees the others read-only, with secrets hidden.">
  <SetupWizard {form} startTitle="Folder" submitLabel="Create workspace" submitting={creating} onsubmit={create}>
    {#snippet start()}
      <h2 style="font-size:var(--font-sizes-xl);font-weight:var(--font-weights-bold)">Which folder holds the repos?</h2>
      <div style="display:grid;gap:var(--spacing-1\.5)">
        <label for="dir" style="font-weight:var(--font-weights-semibold)">Folder</label>
        <FolderPicker bind:value={root} title="Choose the folder that holds the repos" placeholder="~/code/shop" onchange={folderChanged} />
        <span class={muted} style="font-size:var(--font-sizes-sm)">
          {#if form.checking}Checking…{:else if form.error}<span class={bad}>{form.error}</span>{:else if form.draft}Found {form.draft.dir}.{:else}The folder above the repos, e.g. the one with api/ and web/ in it.{/if}
        </span>
      </div>
    {/snippet}
    {#snippet review()}
      Saved to jailwick's config folder. Running projects that join see the members after a restart; change it
      later on the workspace's Settings tab or with <span class={mono}>jw workspace edit</span>.
    {/snippet}
  </SetupWizard>
</Page>
