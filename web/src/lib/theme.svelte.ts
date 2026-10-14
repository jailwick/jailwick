// Light / dark / system theme. public/theme.js applies it before the first paint.
export type ThemePref = 'system' | 'light' | 'dark'

const KEY = 'jailwick.theme'
const media = window.matchMedia('(prefers-color-scheme: dark)')

function read(): ThemePref {
  try {
    const v = localStorage.getItem(KEY)
    if (v === 'light' || v === 'dark') return v
  } catch {}
  return 'system'
}

export const theme = $state({ pref: read() })

function apply() {
  const dark = theme.pref === 'dark' || (theme.pref === 'system' && media.matches)
  document.documentElement.setAttribute('data-theme', dark ? 'dark' : 'light')
}

export function setTheme(pref: ThemePref) {
  theme.pref = pref
  try {
    localStorage.setItem(KEY, pref)
  } catch {}
  apply()
}

media.addEventListener('change', apply)
apply()
