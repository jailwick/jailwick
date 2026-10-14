// idleEvery is the least time between loads while the window isn't the active one.
const idleEvery = 15_000

// Keeps a value fresh by calling load every `every` ms while the page is visible, and every
// `idleEvery` ms (at least) while the window isn't the active one: a window behind others is still
// "visible", and its polls cost the runtime work for nobody. Coming back to it loads at once.
//
// The first load doesn't wait for the page to be visible: the desktop app's window loads its page
// before it is on screen, and would otherwise show nothing until the next tick. A page shown again
// (the window reopened from the menu bar) loads at once.
export function poll<T>(load: () => Promise<T>, every = 3000) {
  const s = $state<{ data: T | undefined; error: string; loading: boolean }>({ data: undefined, error: '', loading: true })
  let timer: ReturnType<typeof setTimeout> | undefined
  let stopped = false
  let inflight: Promise<void> | undefined

  async function fetchOnce() {
    try {
      s.data = await load()
      s.error = ''
    } catch (e) {
      s.error = (e as Error).message
    } finally {
      s.loading = false
    }
  }

  // One load at a time; the next is due `every` ms after it ends.
  function tick(force = false): Promise<void> {
    if (!inflight) {
      clearTimeout(timer)
      const run = force || document.visibilityState === 'visible' ? fetchOnce() : Promise.resolve()
      inflight = run.finally(() => {
        inflight = undefined
        if (!stopped) timer = setTimeout(() => void tick(), document.hasFocus() ? every : Math.max(every, idleEvery))
      })
    }
    return inflight
  }

  function onVisible() {
    if (document.visibilityState === 'visible') void tick()
  }
  document.addEventListener('visibilitychange', onVisible)
  window.addEventListener('focus', onVisible)

  void tick(true)
  return {
    get state() {
      return s
    },
    // Loads again now, after any load already under way (which may predate a change).
    refresh: async () => {
      if (inflight) await inflight
      await tick(true)
    },
    stop: () => {
      stopped = true
      clearTimeout(timer)
      document.removeEventListener('visibilitychange', onVisible)
      window.removeEventListener('focus', onVisible)
    },
  }
}
