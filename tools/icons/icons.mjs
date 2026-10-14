// Draws jailwick's icons: the agent's green dot in a cell, behind four bars.
//
//   node tools/icons/icons.mjs     writes the SVGs below
//   node tools/icons/render.mjs    turns the app and menu bar SVGs into the PNGs the desktop app embeds
//
// - cmd/jailwick-app/icons/app-night.svg, app-day.svg: the app icon (Dock, About box), 3D, for dark
//   and light appearance
// - cmd/jailwick-app/icons/tray.svg: the menu bar icon, a black template that macOS tints
// - web/public/favicon.svg and favicon.svg: a flat mark for small sizes, light and dark in one file
//   (the sidebar logo, web/src/components/Mark.svelte, draws the same cage without the tile)
import { writeFileSync } from 'node:fs'

// App icon geometry on the macOS 1024 grid: window 624x420 centred, frame 32, four bars evenly spaced
const W = 624, H = 420, RX = 84, LINE = 32, X = 512 - W / 2, Y = 512 - H / 2
const CELL = (W - 4 * LINE) / 5
const BARS = [0, 1, 2, 3].map(i => +(X + CELL * (i + 1) + LINE * i).toFixed(1))
const R = 150, CY = 548 // the sphere rests on the window's floor

const stops = (cs) => cs.map(([o, c, a = 1]) => `      <stop offset="${o}" stop-color="${c}"${a < 1 ? ` stop-opacity="${a}"` : ''}/>`).join('\n')

function icon({ note, tile, frame, frameShadow, wall, bar, barShadow, ground, glow, rim, shadow, lift = true, highlight = true }) {
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1024 1024" width="1024" height="1024">
  <!-- ${note} -->
  <defs>
    <linearGradient id="tile" x1="0" y1="0" x2="0" y2="1">
${stops([[0, tile[0]], [1, tile[1]]])}
    </linearGradient>
    <linearGradient id="frame" x1="0" y1="0" x2="0" y2="1">
${stops([[0, frame[0]], [1, frame[1]]])}
    </linearGradient>
    <linearGradient id="wall" x1="0" y1="0" x2="0" y2="1">
${stops([[0, wall[0]], [1, wall[1]]])}
    </linearGradient>
    <radialGradient id="orb" cx="${512 - 60}" cy="${CY - 70}" r="${R * 1.45}" gradientUnits="userSpaceOnUse">
${stops([[0, '#26a662'], [0.45, '#1e9e5a'], [1, '#0e5733']])}
    </radialGradient>
    <linearGradient id="bar" x1="0" y1="0" x2="1" y2="0">
${stops([[0, bar[0]], [0.35, bar[1]], [1, bar[2]]])}
    </linearGradient>${glow ? `
    <radialGradient id="glow" cx="512" cy="${CY}" r="${R * 2.2}" gradientUnits="userSpaceOnUse">
${stops([[0, '#1e9e5a', glow], [1, '#1e9e5a', 0]])}
    </radialGradient>` : ''}
    <clipPath id="window"><rect x="${X}" y="${Y}" width="${W}" height="${H}" rx="${RX}"/></clipPath>
    <filter id="soft" x="-50%" y="-10%" width="200%" height="120%"><feGaussianBlur stdDeviation="9"/></filter>
    <filter id="ground" x="-30%" y="-100%" width="160%" height="300%"><feGaussianBlur stdDeviation="14"/></filter>
    <filter id="inset" x="-10%" y="-10%" width="120%" height="120%">
      <feOffset dy="14"/><feGaussianBlur stdDeviation="12" result="b"/>
      <feComposite in="SourceGraphic" in2="b" operator="arithmetic" k2="-1" k3="1"/>
    </filter>
    <filter id="lift" x="-10%" y="-10%" width="120%" height="130%">
      <feDropShadow dx="0" dy="12" stdDeviation="12" flood-color="${frameShadow[0]}" flood-opacity="${frameShadow[1]}"/>
    </filter>
    <filter id="shadow" x="-10%" y="-10%" width="120%" height="125%">
      <feDropShadow dx="0" dy="14" stdDeviation="18" flood-color="#000" flood-opacity="${shadow}"/>
    </filter>
  </defs>
  <g filter="url(#shadow)">
    <rect x="100" y="100" width="824" height="824" rx="185" fill="url(#tile)"/>
  </g>
  <g clip-path="url(#window)">
    <rect x="${X}" y="${Y}" width="${W}" height="${H}" fill="url(#wall)"/>${glow ? `
    <rect x="${X}" y="${Y}" width="${W}" height="${H}" fill="url(#glow)"/>` : ''}
    <ellipse cx="512" cy="${CY + R - 4}" rx="${R * 0.8}" ry="16" fill="${ground[0]}" opacity="${ground[1]}" filter="url(#ground)"/>
    <circle cx="512" cy="${CY}" r="${R}" fill="url(#orb)"/>
    <g fill="${barShadow[0]}" opacity="${barShadow[1]}" filter="url(#soft)">
${BARS.map(b => `      <rect x="${b + 24}" y="${Y}" width="${LINE}" height="${H + 20}"/>`).join('\n')}
    </g>
    <rect x="${X}" y="${Y}" width="${W}" height="${H}" fill="none" stroke="#000" stroke-opacity=".45" stroke-width="30" filter="url(#inset)"/>
    <g fill="url(#bar)">
${BARS.map(b => `      <rect x="${b}" y="${Y}" width="${LINE}" height="${H}"/>`).join('\n')}
    </g>
  </g>
  <g${lift ? ' filter="url(#lift)"' : ''}>
    <rect x="${X}" y="${Y}" width="${W}" height="${H}" rx="${RX}" fill="none" stroke="url(#frame)" stroke-width="${LINE}"/>
  </g>${highlight ? `
  <rect x="${X - LINE / 2 + 1.5}" y="${Y - LINE / 2 + 1.5}" width="${W + LINE - 3}" height="${H + LINE - 3}" rx="${RX + LINE / 2 - 1.5}" fill="none" stroke="#fff" stroke-opacity=".35" stroke-width="3" clip-path="url(#topHalf)"/>` : ''}${rim ? `
  <rect x="101.5" y="101.5" width="821" height="821" rx="183.5" fill="none" stroke="${rim}" stroke-width="3"/>` : ''}
</svg>
`.replace('<clipPath id="window">', `<clipPath id="topHalf"><rect x="0" y="0" width="1024" height="${Y + 40}"/></clipPath>\n    <clipPath id="window">`)
}

const GREEN = '#1e9e5a'

const app = {
  'app-night': { note: 'jailwick app icon, night: dark tile, lit sphere in a recessed cell, light metal bars casting shadows',
    tile: ['#25352c', '#121b16'], frame: ['#eef2ef', '#b9c5bd'], frameShadow: ['#000', 0.45],
    wall: ['#0d1611', '#16261d'], glow: 0.35,
    bar: ['#ffffff', '#dfe6e1', '#93a49a'], barShadow: ['#000', 0.55], ground: ['#000', 0.6],
    rim: 'rgba(255,255,255,.07)', shadow: 0.3 },
  'app-day': { note: 'jailwick app icon, day: soft green-grey tile and pale cell wall, a flat green cage in one colour (#2b3d33)',
    tile: ['#f5f8f6', '#dde6e0'], frame: ['#2b3d33', '#2b3d33'], frameShadow: ['#0e3a24', 0],
    wall: ['#e4ebe6', '#c9d6cd'],
    bar: ['#2b3d33', '#2b3d33', '#2b3d33'], barShadow: ['#04140b', 0.12], ground: ['#0e3a24', 0.35],
    rim: 'rgba(0,0,0,.06)', shadow: 0.22, lift: false, highlight: false },
}

// The menu bar icon: 44x44 (22pt at 2x), black on transparent. At this size gaps around every bar
// would eat the dot, so the two inner bars cut thin lines through it (and show above and below it),
// while the two outer bars stay whole and the dot is trimmed where it meets them.
function tray() {
  const W = 2.25, CUT = 1.1, GAP = 1.25, R = 8.5, CX = 22, CY = 22.5
  const cell = (31 - 4 * W) / 5
  const bars = [0, 1, 2, 3].map(i => +(6.5 + cell * (i + 1) + W * i).toFixed(2))
  const outer = [bars[0], bars[3]], inner = [bars[1], bars[2]]
  const bar = x => `<rect x="${x}" y="12" width="${W}" height="21"/>`
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 44 44" width="44" height="44">
  <!-- jailwick menu bar icon: a template image, macOS tints it for light and dark menu bars -->
  <defs>
    <mask id="cut" maskUnits="userSpaceOnUse" x="0" y="0" width="44" height="44">
      <rect width="44" height="44" fill="#fff"/>
${inner.map(x => `      <rect x="${+(x + (W - CUT) / 2).toFixed(3)}" y="0" width="${CUT}" height="44" fill="#000"/>`).join('\n')}
${outer.map(x => `      <rect x="${+(x - GAP).toFixed(2)}" y="0" width="${W + 2 * GAP}" height="44" fill="#000"/>`).join('\n')}
    </mask>
    <mask id="clear" maskUnits="userSpaceOnUse" x="0" y="0" width="44" height="44">
      <rect width="44" height="44" fill="#fff"/>
      <circle cx="${CX}" cy="${CY}" r="${R + 1.75}" fill="#000"/>
    </mask>
  </defs>
  <circle cx="${CX}" cy="${CY}" r="${R}" mask="url(#cut)"/>
${outer.map(x => `  ${bar(x)}`).join('\n')}
  <g mask="url(#clear)">
${inner.map(x => `    ${bar(x)}`).join('\n')}
  </g>
  <rect x="4.75" y="10.75" width="34.5" height="23.5" rx="6" fill="none" stroke="#000" stroke-width="3.5"/>
</svg>
`
}

// The small mark on a 24 grid: flat, day colours by default, night colours in dark mode.
export const MARK = {
  day: { tile: '#f2f6f3', edge: 'rgba(0,0,0,.1)', wall: '#dbe4de', cage: '#2b3d33' },
  night: { tile: '#1f2c25', edge: 'rgba(255,255,255,.08)', wall: '#0e1712', cage: '#dfe6e1' },
}
function mark() {
  const { day, night } = MARK
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" width="64" height="64">
  <!-- jailwick mark: the green dot in a cell behind four bars -->
  <style>
    .tile { fill: ${day.tile}; stroke: ${day.edge}; }
    .wall { fill: ${day.wall}; }
    .cage { fill: ${day.cage}; }
    .frame { stroke: ${day.cage}; }
    @media (prefers-color-scheme: dark) {
      .tile { fill: ${night.tile}; stroke: ${night.edge}; }
      .wall { fill: ${night.wall}; }
      .cage { fill: ${night.cage}; }
      .frame { stroke: ${night.cage}; }
    }
  </style>
${MARK_BODY}
</svg>
`
}
// Mark.svelte draws the same cage (without the tile); keep the two in step.
const MARK_BODY = `  <rect class="tile" x=".5" y=".5" width="23" height="23" rx="5.25" stroke-width="1"/>
  <rect class="wall" x="3.5" y="6.25" width="17" height="11.5" rx="3"/>
  <circle cx="12" cy="12.6" r="3.9" fill="${GREEN}"/>
  <g class="cage">
    <rect x="6.47" y="6.25" width="1.1" height="11.5"/>
    <rect x="9.79" y="6.25" width="1.1" height="11.5"/>
    <rect x="13.11" y="6.25" width="1.1" height="11.5"/>
    <rect x="16.43" y="6.25" width="1.1" height="11.5"/>
  </g>
  <rect class="frame" x="3.5" y="6.25" width="17" height="11.5" rx="3" fill="none" stroke-width="1.5"/>`

for (const [name, v] of Object.entries(app)) writeFileSync(`cmd/jailwick-app/icons/${name}.svg`, icon(v))
writeFileSync('cmd/jailwick-app/icons/tray.svg', tray())
writeFileSync('web/public/favicon.svg', mark())
writeFileSync('favicon.svg', mark())
