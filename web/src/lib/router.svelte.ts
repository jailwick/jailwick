// Hash routes: #/projects/<name>/<tab>, #/new-project, #/sessions, #/workspaces/<name>, #/network, #/images, #/settings.
// A route can name what to point out on its page after a "?" (#/projects/api/network?hosts=a.com,b.com):
// a notification's click opens the page with the thing it was about picked out (see focus).
// Navigating replaces the page's place in history instead of adding one, so back keys, mouse
// buttons and swipes have nothing to go back to. The app keeps its own short trail for its Back button.
export const route = $state({ parts: parse(), focus: parseFocus() })

const maxTrail = 20
const trail = $state<string[]>([])

/** Whether back() has somewhere to go. */
export const canBack = () => trail.length > 0

function parse(): string[] {
  const h = location.hash.replace(/^#\/?/, '').split('?')[0]
  const parts = h.split('/').filter(Boolean).map(decodeURIComponent)
  return parts.length ? parts : ['projects']
}

function parseFocus(): Record<string, string> {
  const q = location.hash.split('?')[1]
  return q ? Object.fromEntries(new URLSearchParams(q)) : {}
}

/** What the route asks its page to point out under key, once: the page takes it, and it's gone
 * from the route (so the same notification clicked again points it out again). */
export function focus(key: string): string {
  const v = route.focus[key] ?? ''
  if (v) {
    const { [key]: _, ...rest } = route.focus
    route.focus = rest
    const q = new URLSearchParams(rest).toString()
    history.replaceState(history.state, '', hashOf(route.parts) + (q ? '?' + q : ''))
  }
  return v
}

const hashOf = (parts: string[]) => '#/' + parts.map(encodeURIComponent).join('/')

function show(hash: string) {
  history.replaceState(history.state, '', hash)
  route.parts = parse()
  route.focus = parseFocus()
}

// A typed URL or the tray changes the hash: it counts as a step like go().
window.addEventListener('hashchange', (e) => {
  const from = new URL(e.oldURL).hash || hashOf(['projects'])
  if (from.split('?')[0] !== location.hash.split('?')[0]) remember(from.split('?')[0])
  route.parts = parse()
  route.focus = parseFocus()
})

function remember(hash: string) {
  if (trail.at(-1) === hash) return
  trail.push(hash)
  if (trail.length > maxTrail) trail.shift()
}

/** goWith goes to a page with what it should point out (see focus): goWith({ service: 'postgres' },
 * 'projects', 'api', 'logs'). */
export function goWith(point: Record<string, string>, ...parts: string[]) {
  remember(hashOf(route.parts))
  show(hashOf(parts) + '?' + new URLSearchParams(point).toString())
}

export function go(...parts: string[]) {
  const hash = hashOf(parts)
  const here = hashOf(route.parts)
  if (hash === here) return
  route.focus = {}
  remember(here)
  show(hash)
}

/** Back to the page before this one, if any. */
export function back() {
  const hash = trail.pop()
  if (hash) show(hash)
}
