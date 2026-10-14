<script lang="ts">
  import type { SetupField } from '../lib/api'
  import type { SetupForm } from '../lib/setupForm.svelte'
  import FieldInput from './FieldInput.svelte'

  // The fields of a setup that apply, given the answers so far, each with its problem.
  let { form, fields }: { form: SetupForm; fields: SetupField[] } = $props()
</script>

{#each fields.filter((f) => form.visible(f)) as f (f.key)}
  <FieldInput field={f} value={form.valueOf(f)} problem={form.problemOf(f)} values={form.values} onchange={(v) => form.set(f, v)} />
{/each}
