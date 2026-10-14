<script lang="ts">
  import { css } from 'styled-system/css'
  import { phaseLabel, type Phase } from '../lib/phase'
  // unknown: the runtime hasn't answered yet; phase: what it goes through besides running or
  // stopped; problems: containers not doing their work.
  let { on = false, busy = false, unknown = false, phase = '', problems = 0 }:
    { on?: boolean; busy?: boolean; unknown?: boolean; phase?: Phase; problems?: number } = $props()
  const dot = css({ display: 'inline-block', w: 'dot.lg', h: 'dot.lg', borderRadius: 'full', flexShrink: 0 })
  const onC = css({ bg: 'ok', boxShadow: '0 0 0 3px {colors.okSubtle}' })
  const offC = css({ bg: 'transparent', borderWidth: '1.5px', borderStyle: 'solid', borderColor: 'fg.faint' })
  const busyC = css({ bg: 'info', boxShadow: '0 0 0 3px {colors.infoSubtle}' })
  const unknownC = css({ bg: 'fg.faint', opacity: 0.4 })
  const phaseC = css({ bg: 'warn', boxShadow: '0 0 0 3px {colors.warnSubtle}', animation: 'pulse 1.2s ease-in-out infinite' })
  const pausedC = css({ bg: 'transparent', borderWidth: '2.5px', borderStyle: 'solid', borderColor: 'warn' })
  const problemC = css({ bg: 'bad', boxShadow: '0 0 0 3px {colors.badSubtle}' })
  const look = $derived(
    unknown ? [unknownC, 'checking…']
    : phase === 'paused' ? [pausedC, phaseLabel.paused]
    : phase ? [phaseC, phaseLabel[phase]]
    : on && problems ? [problemC, `${problems} service${problems === 1 ? '' : 's'} need${problems === 1 ? 's' : ''} attention`]
    : busy ? [busyC, 'busy']
    : on ? [onC, 'running'] : [offC, 'stopped'],
  )
</script>

<span class="{dot} {look[0]}" title={look[1]}></span>
