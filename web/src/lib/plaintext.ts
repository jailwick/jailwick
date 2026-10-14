// The app's fields hold commands, paths, names, hosts and keys, not prose. The system's spelling
// correction, text replacement, capitalisation and word predictions would change them as they're
// typed (libuv becomes Lib), so every text field has them off: set here for the whole app, on
// the fields there are and on every field a view adds later (wizards, dialogs).
const off: Record<string, string> = {
  spellcheck: 'false',
  autocorrect: 'off', // Safari, and the desktop app's web view on macOS
  autocapitalize: 'off',
  autocomplete: 'off',
  writingsuggestions: 'false', // inline predictions
}

// Input types that take typed text (checkboxes, radios, buttons don't).
const textTypes = new Set(['text', 'search', 'url', 'email', 'password', 'number', 'tel'])

function plain(el: Element) {
  if (el instanceof HTMLTextAreaElement || (el instanceof HTMLInputElement && textTypes.has(el.type))) {
    for (const [k, v] of Object.entries(off)) el.setAttribute(k, v)
  }
}

function plainAll(node: Node) {
  if (!(node instanceof Element)) return
  plain(node)
  node.querySelectorAll('input, textarea').forEach(plain)
}

// plainTextFields keeps every text field under root free of corrections.
export function plainTextFields(root: HTMLElement) {
  plainAll(root)
  new MutationObserver((records) => {
    for (const r of records) {
      r.addedNodes.forEach(plainAll)
      if (r.type === 'attributes') plain(r.target as Element) // an input whose type changed
    }
  }).observe(root, { childList: true, subtree: true, attributes: true, attributeFilter: ['type'] })
}
