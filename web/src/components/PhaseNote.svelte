<script lang="ts">
  import { css } from 'styled-system/css'
  import Icon from './Icon.svelte'
  import { phaseLabel, type Phase } from '../lib/phase'
  // What the sandbox is going through, in place of what it would show.
  let { phase }: { phase: Exclude<Phase, ''> } = $props()
  const note = css({ display: 'inline-flex', alignItems: 'center', gap: '1.5', color: 'fg.muted' })
</script>

<span class={note} role="status"><Icon name={phase === 'paused' ? 'pause' : 'spinner'} size={13} />{phaseLabel[phase]}{phase === 'paused' ? ': resume it to carry on' : ''}</span>
