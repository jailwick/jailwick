<script lang="ts">
  import { api, type ProjectDetail } from '../../lib/api'
  import NotesEditor from '../../components/NotesEditor.svelte'

  let { d }: { d: ProjectDetail } = $props()
  const name = $derived(d.name) // d is replaced at every refresh; only another project loads again
</script>

{#key name}
  <NotesEditor
    load={() => api.notes(name)}
    save={(text) => api.saveNotes(name, text)}
    help="How to run and test this project in the sandbox. Read at the start of each session."
    placeholder={`# ${name}\n\n## Run\n...`}
  />
{/key}
