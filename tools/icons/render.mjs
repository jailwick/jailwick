// Renders the desktop app's icons, with a transparent background:
//   cmd/jailwick-app/icons/*.png          embedded by the app (Dock icon, menu bar icon)
//   cmd/jailwick-app/packaging/darwin/jailwick.icns   the macOS bundle's icon
//   cmd/jailwick-app/packaging/linux/jailwick.png     the Linux desktop entry's icon
// (Windows: `make winres` builds the .exe's icon from app-night.png.)
// Needs Playwright with Chromium: `npx playwright install chromium` once, or set
// PLAYWRIGHT_MODULE to the path of an installed `playwright` package.
//
//   node tools/icons/icons.mjs && node tools/icons/render.mjs
import { readFileSync, writeFileSync } from 'node:fs'
import { createRequire } from 'node:module'

const require = createRequire(import.meta.url)
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright')

const ICONS = 'cmd/jailwick-app/icons'
const PACKAGING = 'cmd/jailwick-app/packaging'

// ICNS entries: type code and pixel size (the @2x types reuse the larger renders).
const ICNS = [
  ['icp4', 16], ['icp5', 32], ['icp6', 64], ['ic07', 128], ['ic08', 256], ['ic09', 512], ['ic10', 1024],
  ['ic11', 32], ['ic12', 64], ['ic13', 256], ['ic14', 512],
]

const browser = await chromium.launch()
const page = await browser.newPage()

// render draws an SVG at size x size and returns the PNG.
async function render(svg, size) {
  await page.setViewportSize({ width: size, height: size })
  await page.setContent(
    `<style>html,body{margin:0;background:transparent}svg{display:block;width:${size}px;height:${size}px}</style>${svg}`,
  )
  return page.screenshot({ omitBackground: true })
}

function save(file, data, size) {
  writeFileSync(file, data)
  console.log(`${file}  ${size}`)
}

for (const [name, size] of [['app-night', 1024], ['app-day', 1024], ['tray', 44]]) {
  save(`${ICONS}/${name}.png`, await render(readFileSync(`${ICONS}/${name}.svg`, 'utf8'), size), `${size}x${size}`)
}

// Bundles and desktop entries have one icon: night, which reads on light and dark.
const night = readFileSync(`${ICONS}/app-night.svg`, 'utf8')
const pngs = new Map()
for (const size of new Set(ICNS.map(([, s]) => s))) pngs.set(size, await render(night, size))
save(`${PACKAGING}/darwin/jailwick.icns`, icns(ICNS.map(([type, size]) => [type, pngs.get(size)])), 'icns')
save(`${PACKAGING}/linux/jailwick.png`, pngs.get(512), '512x512')
await browser.close()

// icns packs PNGs into Apple's icon format: "icns", total length, then (type, length, data) entries.
function icns(entries) {
  const parts = entries.map(([type, png]) => {
    const head = Buffer.alloc(8)
    head.write(type, 0, 'ascii')
    head.writeUInt32BE(8 + png.length, 4)
    return Buffer.concat([head, png])
  })
  const head = Buffer.alloc(8)
  head.write('icns', 0, 'ascii')
  head.writeUInt32BE(8 + parts.reduce((n, p) => n + p.length, 0), 4)
  return Buffer.concat([head, ...parts])
}
