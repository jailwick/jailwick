<script lang="ts">
  import { css } from 'styled-system/css'
  import type { FieldValue, SetupField, SetupOption, SetupValues } from '../lib/api'
  import { badge, chip, input } from '../lib/styles'
  import Icon from './Icon.svelte'
  import ImageCheck from './ImageCheck.svelte'

  // One setting of the guided setup, rendered by its kind. Lists are edited one value per line; a
  // choice is a list of radios. values are the form's other answers, for an action that needs them
  // (checking an image takes the user and home given).
  let {
    field,
    value,
    problem = '',
    values = {},
    onchange,
  }: { field: SetupField; value: FieldValue; problem?: string; values?: SetupValues; onchange: (v: FieldValue) => void } = $props()
  const other = (k: string) => (typeof values[k] === 'string' ? (values[k] as string) : '')

  const id = $derived(`field-${field.key}`)
  const list = $derived(Array.isArray(value) ? value : [])

  function toggle(option: string, on: boolean) {
    onchange(on ? [...list, option] : list.filter((v) => v !== option))
  }

  // Options with groups (long lists, like the presets) are shown as a list under their groups, the
  // first few open and the rest collapsed (their header counts what is ticked); short ones as chips.
  const openGroups = 3
  const chosen = (o: SetupOption) => o.fixed || list.includes(o.value)
  // A disabled option can't be ticked; one already ticked can still be unticked.
  const locked = (o: SetupOption) => o.fixed || (!!o.disabled && !list.includes(o.value))
  const groups = $derived.by(() => {
    const out: [string, SetupOption[]][] = []
    for (const o of field.options ?? []) {
      if (!o.group) return null
      if (out.at(-1)?.[0] !== o.group) out.push([o.group, []])
      out.at(-1)![1].push(o)
    }
    return out
  })

  const wrap = css({ display: 'grid', gap: '1.5' })
  const label = css({ fontWeight: 'semibold', fontSize: 'base' })
  const help = css({ color: 'fg.muted', fontSize: 'sm' })
  const choices = css({ display: 'flex', flexWrap: 'wrap', gap: '2' })
  const choice = chip
  const area = css({ h: 'auto', py: '1.5', fontFamily: 'mono', fontSize: 'sm', resize: 'vertical' })
  const groupList = css({ display: 'grid', gap: '3', mt: '1' })
  const groupName = css({
    display: 'flex', alignItems: 'center', gap: '1.5', w: 'fit-content', py: '0.5', fontSize: '2xs', fontWeight: 'semibold',
    color: 'fg.faint', textTransform: 'capitalize', cursor: 'pointer', userSelect: 'none', listStyle: 'none',
    '&::-webkit-details-marker': { display: 'none' }, _hover: { color: 'fg.muted' },
    '& svg': { transitionProperty: 'transform', transitionDuration: 'fast' },
  })
  const groupBox = css({ '&[open] > summary svg': { transform: 'rotate(90deg)' } })
  const count = css({ fontWeight: 'medium', textTransform: 'none' })
  const row = css({
    display: 'grid', gridTemplateColumns: 'auto 9rem 1fr', alignItems: 'baseline', gap: '2.5', py: '1', px: '2',
    borderRadius: 'control', cursor: 'pointer', _hover: { bg: 'bg.subtle' }, '&:has(input:disabled)': { cursor: 'default' },
  })
  const rowName = css({ fontWeight: 'strong', fontSize: 'base' })
  const bad = css({ color: 'bad', fontSize: 'sm' })
  const note = css({ fontSize: 'md', px: '3', py: '2', borderRadius: 'control', bg: 'warnSubtle', color: 'fg.default' })
  const radios = css({ display: 'grid', gap: '1' })
  const radio = css({
    display: 'grid', gridTemplateColumns: 'auto 1fr', columnGap: '2.5', alignItems: 'baseline', px: '2.5', py: '2',
    borderRadius: 'control', border: '1px solid {colors.border.default}', cursor: 'pointer', _hover: { bg: 'bg.subtle' },
    '&:has(input:checked)': { borderColor: 'selected.border', bg: 'selected.bg' },
  })
  const radioLabel = css({ fontWeight: 'semibold', display: 'inline-flex', gap: '2', alignItems: 'center' })
  const radioHelp = css({ gridColumn: '2', color: 'fg.muted', fontSize: 'sm' })
</script>

<div class={wrap}>
  {#if field.kind === 'multi'}
    <span class={label} id={id}>{field.label}</span>
    {#if field.note}<p class={note}>{field.note}</p>{/if}
    {#if groups}
      <div class={groupList} role="group" aria-labelledby={id}>
        {#each groups as [group, opts], i (group)}
          {@const ticked = opts.filter(chosen).length}
          <details class={groupBox} open={i < openGroups}>
            <summary class={groupName}>
              <Icon name="chevron" size={11} />{group}
              <span class={count}>· {ticked ? `${ticked} of ${opts.length} selected` : opts.length}</span>
            </summary>
            {#each opts as o (o.value)}
              <label class={row}>
                <input type="checkbox" checked={chosen(o)} disabled={locked(o)}
                  onchange={(e) => toggle(o.value, e.currentTarget.checked)} />
                <span class={rowName}>{o.value}</span>
                <span class={help}>
                  {o.help}
                  {#if o.fixed}<span class={badge()} title="Always allowed: every sandbox needs it">always</span>{/if}
                  {#if o.tag}<span class={badge({ tone: 'warn' })} title={o.warn}>{o.tag}</span>{/if}
                </span>
              </label>
            {/each}
          </details>
        {/each}
      </div>
    {:else}
      <div class={choices} role="group" aria-labelledby={id}>
        {#each field.options ?? [] as o (o.value)}
          <label class={choice} title={o.fixed ? 'Always included' : o.warn || o.help}>
            <input type="checkbox" checked={chosen(o)} disabled={locked(o)} onchange={(e) => toggle(o.value, e.currentTarget.checked)} />
            {o.value}
            {#if o.tag}<span class={badge({ tone: 'warn' })}>{o.tag}</span>{/if}
          </label>
        {/each}
      </div>
    {/if}
  {:else if field.kind === 'choice'}
    <span class={label} id={id}>{field.label}</span>
    {#if field.note}<p class={note}>{field.note}</p>{/if}
    <div class={radios} role="radiogroup" aria-labelledby={id}>
      {#each field.options ?? [] as o (o.value)}
        <label class={radio}>
          <input type="radio" name={id} checked={value === o.value} disabled={o.disabled} onchange={() => onchange(o.value)} />
          <span class={radioLabel}>
            {o.label || o.value}
            {#if o.tag}<span class={badge({ tone: 'warn' })} title={o.warn}>{o.tag}</span>{/if}
          </span>
          {#if o.help || o.warn}<span class={radioHelp}>{o.help}{o.help && o.warn ? ': ' : ''}{o.warn ?? ''}</span>{/if}
        </label>
      {/each}
    </div>
  {:else if field.kind === 'list'}
    <label class={label} for={id}>{field.label}</label>
    <textarea
      {id}
      class="{input} {area}"
      rows={Math.max(2, list.length + 1)}
      placeholder={field.placeholder ? `${field.placeholder}\n(one per line)` : 'One per line'}
      value={list.join('\n')}
      oninput={(e) => onchange(e.currentTarget.value.split('\n'))}
    ></textarea>
  {:else}
    <label class={label} for={id}>{field.label}</label>
    <input
      {id}
      class={input}
      type={field.kind === 'secret' ? 'password' : 'text'}
      autocomplete="off"
      placeholder={field.placeholder ?? ''}
      list={field.options?.length ? `${id}-options` : undefined}
      value={typeof value === 'string' ? value : ''}
      oninput={(e) => onchange(e.currentTarget.value)}
    />
    {#if field.options?.length}
      <datalist id="{id}-options">{#each field.options as o (o.value)}<option value={o.value}></option>{/each}</datalist>
    {/if}
  {/if}
  {#if problem}<span class={bad} role="alert">{problem}</span>{/if}
  <!-- "Default: …" only matters while the field is empty. -->
  {#if field.help && !(field.help.startsWith('Default:') && typeof value === 'string' && value)}<span class={help}>{field.help}</span>{/if}
  {#if field.action === 'check-image' && typeof value === 'string'}
    <ImageCheck ref={value} user={other('image.user')} home={other('image.home')} asIs={field.key === 'image.name'} />
  {/if}
</div>
