import type { FieldValue, SetupDraft, SetupField, SetupValues } from './api'

// A guided setup answered against the server (a new project, a workspace): each change of the
// answers drafts again, and the server sends back the steps, the file and the problems. Answers to
// a draft request that is no longer the latest are dropped.
export class SetupForm {
  draft = $state<SetupDraft | null>(null)
  values = $state<SetupValues>({})
  error = $state('')
  checking = $state(false)
  #initial = $state('') // the values as loaded, to tell whether anything changed
  #seq = 0
  #timer: ReturnType<typeof setTimeout> | undefined

  /** live: each change drafts again a moment after typing stops (a form on one page shows its
   *  problems as they come; a wizard drafts when moving on). */
  constructor(
    private readonly fetchDraft: (values: SetupValues) => Promise<SetupDraft>,
    private readonly live = false,
  ) {}

  /** Whether the answers differ from the ones loaded by start(). */
  get changed() {
    return !!this.draft && JSON.stringify(this.values) !== this.#initial
  }

  /** Starts over: every answer comes from the draft. */
  start() {
    return this.#load(null)
  }

  /** Drafts again with the answers given; answers not given come from the draft. */
  refresh() {
    return this.#load(this.values)
  }

  clear() {
    clearTimeout(this.#timer)
    this.#seq++
    this.draft = null
    this.values = {}
    this.error = ''
    this.checking = false
  }

  /** Sets an answer. A field with resets recomputes others: their answers are dropped, and come back
   *  from the draft a moment after typing stops. */
  set(f: SetupField, v: FieldValue) {
    this.values = { ...this.values, [f.key]: v }
    const resets = f.resets ?? []
    if (!resets.length && !this.live) return
    clearTimeout(this.#timer)
    this.#timer = setTimeout(() => {
      if (resets.length) this.values = Object.fromEntries(Object.entries(this.values).filter(([k]) => !resets.includes(k)))
      this.refresh()
    }, 350)
  }

  valueOf(f: SetupField): FieldValue {
    return this.values[f.key] ?? (f.kind === 'multi' || f.kind === 'list' ? [] : '')
  }

  /** Whether a field applies, given the other answers. A multi field without options has nothing to ask. */
  visible(f: SetupField): boolean {
    if (f.kind === 'multi' && !f.options?.length) return false
    if (!f.show_if) return true
    const v = this.values[f.show_if.key]
    const has = f.show_if.has ?? ''
    const holds = Array.isArray(v) ? (has ? v.includes(has) : v.length > 0) : has ? v === has : !!v
    return holds !== !!f.show_if.not
  }

  /** The problem about a field: problems name it by its label ("Read logins from: ..."). */
  problemOf(f: SetupField): string {
    return this.draft?.problems.find((p) => p.startsWith(f.label + ':')) ?? ''
  }

  async #load(given: SetupValues | null) {
    const n = ++this.#seq
    this.checking = true
    try {
      const d = await this.fetchDraft(given ?? {})
      if (n !== this.#seq) return
      const fresh: SetupValues = Object.fromEntries(d.steps.flatMap((s) => s.fields.map((f) => [f.key, f.value])))
      this.draft = d
      this.error = ''
      this.values = given ? { ...fresh, ...given } : fresh
      if (!given) this.#initial = JSON.stringify(this.values)
    } catch (e) {
      if (n !== this.#seq) return
      this.draft = null
      this.error = (e as Error).message
    } finally {
      if (n === this.#seq) this.checking = false
    }
  }
}
