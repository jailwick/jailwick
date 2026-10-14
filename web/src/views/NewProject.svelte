<script lang="ts">
  import { css } from 'styled-system/css'
  import { onMount, untrack } from 'svelte'
  import { names } from '../lib/format'
  import { api } from '../lib/api'
  import { go } from '../lib/router.svelte'
  import { SetupForm } from '../lib/setupForm.svelte'
  import { mono, muted } from '../lib/styles'
  import Page from '../lib/Page.svelte'
  import FolderPicker from '../components/FolderPicker.svelte'
  import SetupWizard from '../components/SetupWizard.svelte'
  import { toast } from '../components/Toasts.svelte'

  // The guided "new project" flow: a repo folder, then the steps the server defines, then a review.
  // dir: a folder to start with (e.g. a workspace member that isn't a project yet); the parent keys
  // the page by it.
  let { dir: start = '', oncreated }: { dir?: string; oncreated: () => void } = $props()

  let dir = $state(untrack(() => start))
  onMount(() => {
    if (dir) form.start()
  })
  const form = new SetupForm((values) => api.draftProject(dir, values))
  let creating = $state(false)

  let timer: ReturnType<typeof setTimeout> | undefined
  function folderChanged(_: string, picked: boolean) {
    clearTimeout(timer)
    form.clear()
    if (dir.trim()) timer = setTimeout(() => form.start(), picked ? 0 : 350)
  }

  async function create() {
    creating = true
    try {
      const { name, workspace, restart } = await api.createProject(dir, form.values)
      toast(`Added ${name}. Start it, then open a session.`, 'ok')
      // Its folder joined the workspace's members: the running ones list it as pending.
      if (restart.length) toast(`${workspace}'s ${names(restart)} see ${name}'s folder after a restart.`, 'ok')
      oncreated()
      go('projects', name)
    } catch (e) {
      toast((e as Error).message, 'bad')
      await form.refresh()
    } finally {
      creating = false
    }
  }

  const bad = css({ color: 'bad' })
</script>

<Page title="New project" subtitle="Add a repo as a project. Answers are prefilled from its env files.">
  <SetupWizard {form} startTitle="Folder" submitLabel="Create project" submitting={creating} onsubmit={create}>
    {#snippet start()}
      <h2 style="font-size:var(--font-sizes-xl);font-weight:var(--font-weights-bold)">Which repo?</h2>
      <div style="display:grid;gap:var(--spacing-1\.5)">
        <label for="dir" style="font-weight:var(--font-weights-semibold)">Folder</label>
        <FolderPicker bind:value={dir} title="Choose the repo to add to jailwick" onchange={folderChanged} />
        <span class={muted} style="font-size:var(--font-sizes-sm)">
          {#if form.checking}Checking…{:else if form.error}<span class={bad}>{form.error}</span>{:else if form.draft}Found {form.draft.dir}.{:else}The repo's own folder, not one that holds many repos.{/if}
        </span>
      </div>
    {/snippet}
    {#snippet review()}
      Saved to jailwick's config folder, not the repo. Change it later on the project's Settings tab, or with <span class={mono}>jw project edit</span>.
    {/snippet}
  </SetupWizard>
</Page>
