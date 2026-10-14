<script lang="ts">
  import { api } from '../../lib/api'
  import NotesEditor from '../../components/NotesEditor.svelte'

  let { name }: { name: string } = $props()
</script>

<NotesEditor
  load={() => api.workspaceNotes(name)}
  save={(text) => api.saveWorkspaceNotes(name, text)}
  help="What every session of the workspace should know: how the repos fit together, who owns what."
  placeholder={`# ${name}\n\n## How the repos fit together\n...`}
/>
