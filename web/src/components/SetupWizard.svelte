<script lang="ts">
  import { css, cva } from 'styled-system/css'
  import type { Snippet } from 'svelte'
  import type { SetupForm } from '../lib/setupForm.svelte'
  import { button, muted, panel, segment, segmentOn, segmented } from '../lib/styles'
  import Icon from './Icon.svelte'
  import SetupFields from './SetupFields.svelte'
  import SetupSummary from './SetupSummary.svelte'

  // A guided setup, step by step: a first step of the page's own (the folder), the steps the server
  // defines, then a review of the file and the problems. Moving on drafts again, so the review is
  // always of the answers given.
  let {
    form,
    startTitle,
    start,
    review,
    submitLabel,
    submitting = false,
    onsubmit,
  }: {
    form: SetupForm
    startTitle: string
    start: Snippet
    review?: Snippet
    submitLabel: string
    submitting?: boolean
    onsubmit: () => void
  } = $props()

  let step = $state(0) // 0: start, 1..n: the draft's steps, n+1: review
  let asYaml = $state(false) // the review as the file it writes, rather than in words
  const draft = $derived(form.draft)
  const titles = $derived([startTitle, ...(draft?.steps.map((s) => s.title) ?? ['Settings']), 'Review'])
  const last = $derived(titles.length - 1)
  const current = $derived(draft && step > 0 && step < last ? draft.steps[step - 1] : null)

  // A new start (another folder) begins again at the first step.
  $effect(() => {
    if (!draft) step = 0
  })

  // The file as the review shows it: passwords masked (the steps mask them too), and an empty
  // stack version said in words rather than as "".
  function reviewed(yaml: string) {
    let inList = false
    return yaml
      .split('\n')
      .map((l) => {
        const m = l.match(/^(\s*[\w.-]*password\w*:)(\s*)(.*)$/i)
        if (m) {
          inList = m[3] === ''
          return inList ? l : `${m[1]}${m[2]}••••••`
        }
        if (inList && /^\s*- /.test(l)) return l.replace(/- .*/, '- ••••••')
        inList = false
        return l.replace(/^(\s+[\w.-]+): ""$/, '$1: ""  # the default version')
      })
      .join('\n')
  }
  const fieldsShown = $derived(current ? current.fields.filter((f) => form.visible(f)).length : 0)

  async function goTo(n: number) {
    if (n > step && step > 0) await form.refresh() // problems and the file of the answers given
    step = n
  }

  const layout = css({ display: 'grid', gridTemplateColumns: { base: '1fr', lg: '200px 1fr' }, gap: '6', alignItems: 'start' })
  const steps = css({ display: 'grid', gap: '1', listStyle: 'none', m: 0, p: 0 })
  // Each label's first line sits level with its number, and two-line labels keep the same gap.
  const stepItem = css({
    display: 'flex', alignItems: 'flex-start', gap: '2.5', w: '100%', minH: 'row', px: '2', py: '1.5', borderRadius: 'control',
    color: 'fg.muted', textAlign: 'left', lineHeight: 'row', _disabled: { cursor: 'default', opacity: 0.6 },
    '&:not(:disabled)': { cursor: 'pointer', _hover: { bg: 'bg.hover' } },
  })
  const stepOn = css({ bg: 'selected.bg', color: 'selected.fg', fontWeight: 'semibold', '&:not(:disabled)': { _hover: { bg: 'selected.bg' } } })
  // Done: a check; the current one: filled in the text colour; ahead: grey.
  const num = cva({
    base: { display: 'grid', placeItems: 'center', w: 'badge', h: 'badge', borderRadius: 'full', fontSize: '2xs', fontWeight: 'semibold', flex: 'none' },
    variants: {
      at: {
        done: { bg: 'okSubtle', color: 'ok' },
        current: { bg: 'selected.fg', color: 'bg.panel' },
        ahead: { bg: 'bg.subtle', color: 'fg.muted' },
      },
    },
  })
  const body = css({ p: '5', display: 'grid', gap: '5' })
  const reviewHead = css({ display: 'flex', alignItems: 'center', gap: '3', flexWrap: 'wrap', justifyContent: 'space-between' })
  const h2 = css({ fontSize: 'xl', fontWeight: 'bold' })
  const actions = css({ display: 'flex', gap: '2', justifyContent: 'flex-end', pt: '1' })
  const note = css({ fontSize: 'md', display: 'flex', gap: '2', alignItems: 'baseline' })
  const yaml = css({
    m: 0, p: '3', borderRadius: 'control', bg: 'bg.inset', border: '1px solid {colors.border.default}',
    overflowX: 'auto', fontFamily: 'mono', fontSize: 'sm', lineHeight: 'code',
  })
  const problems = css({ m: 0, pl: '4', color: 'bad', display: 'grid', gap: '1' })
</script>

<div class={layout}>
  <ol class={steps} aria-label="Steps">
    {#each titles as t, i}
      <li>
        <button class="{stepItem} {i === step ? stepOn : ''}" disabled={i > step || (i > 0 && !draft)} onclick={() => goTo(i)}>
          <span class={num({ at: i < step ? 'done' : i === step ? 'current' : 'ahead' })}>
            {#if i < step}<Icon name="check" size={11} />{:else}{i + 1}{/if}
          </span>{t}
        </button>
      </li>
    {/each}
  </ol>

  <section class="{panel} {body}">
    {#if step === 0}
      {@render start()}
      {#each draft?.hints ?? [] as h}<p class={note}><Icon name="alert" size={13} />{h}</p>{/each}
    {:else if current}
      <h2 class={h2}>{current.title}</h2>
      <SetupFields {form} fields={current.fields} />
      {#if fieldsShown === 0}<p class={muted}>Nothing to choose in this step for this folder: go on to the next one.</p>{/if}
    {:else if draft}
      <div class={reviewHead}>
        <h2 class={h2}>Review</h2>
        <span class={segmented} role="radiogroup" aria-label="Show the review as">
          <button class="{segment} {asYaml ? '' : segmentOn}" role="radio" aria-checked={!asYaml} onclick={() => (asYaml = false)}>Summary</button>
          <button class="{segment} {asYaml ? segmentOn : ''}" role="radio" aria-checked={asYaml} onclick={() => (asYaml = true)}>YAML</button>
        </span>
      </div>
      {#if draft.problems.length}
        <ul class={problems}>{#each draft.problems as p}<li>{p}</li>{/each}</ul>
      {/if}
      {#if asYaml}
        <pre class={yaml}>{reviewed(draft.yaml)}</pre>
      {:else}
        <SetupSummary {form} steps={draft.steps} onedit={(i) => goTo(i + 1)} />
      {/if}
      {#if review}<div class={muted} style="font-size:var(--font-sizes-sm)">{@render review()}</div>{/if}
    {/if}

    <div class={actions}>
      {#if step > 0}<button class={button()} onclick={() => goTo(step - 1)}>Back</button>{/if}
      {#if step < last}
        <button class={button({ variant: 'primary' })} disabled={!draft || form.checking} onclick={() => goTo(step + 1)}>Next</button>
      {:else}
        <button class={button({ variant: 'primary' })} disabled={submitting || !draft || draft.problems.length > 0} onclick={onsubmit}>
          <Icon name="check" size={13} />{submitting ? 'Saving…' : submitLabel}
        </button>
      {/if}
    </div>
  </section>
</div>
