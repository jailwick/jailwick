<script lang="ts">
  import { css } from 'styled-system/css'

  // jailwick's mark: the green dot in a cell behind four bars, on a transparent background so it
  // sits on whatever is behind it. Day colours in light mode, night colours in dark. Same cage as
  // public/favicon.svg (tools/icons/icons.mjs), without the favicon's tile; keep them in step.
  let { width = 26 }: { width?: number } = $props()

  const wall = css({ fill: '#dbe4de', _dark: { fill: '#0e1712' } })
  const cage = css({ fill: '#2b3d33', _dark: { fill: '#dfe6e1' } })
  const frame = css({ stroke: '#2b3d33', _dark: { stroke: '#dfe6e1' } })
  const bars = [6.47, 9.79, 13.11, 16.43]
</script>

<svg viewBox="2.6 5.35 18.8 13.3" {width} height={(width * 13.3) / 18.8} aria-hidden="true">
  <rect class={wall} x="3.5" y="6.25" width="17" height="11.5" rx="3" />
  <circle cx="12" cy="12.6" r="3.9" fill="#1e9e5a" />
  <g class={cage}>
    {#each bars as x}<rect {x} y="6.25" width="1.1" height="11.5" />{/each}
  </g>
  <rect class={frame} x="3.5" y="6.25" width="17" height="11.5" rx="3" fill="none" stroke-width="1.5" />
</svg>
