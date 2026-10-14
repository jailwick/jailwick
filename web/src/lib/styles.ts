import { css, cva } from 'styled-system/css'

// Shared building blocks. Components use these instead of repeating style objects.

export const panel = css({
  bg: 'bg.panel',
  borderRadius: 'panel',
  boxShadow: 'panel',
  overflow: 'hidden',
})

export const panelHeader = css({
  display: 'flex',
  alignItems: 'center',
  gap: '2',
  px: '4',
  py: '2.5',
  borderBottom: '1px solid {colors.border.default}',
  fontWeight: 'semibold',
  fontSize: 'sm',
  color: 'fg.muted',
  textTransform: 'uppercase',
  letterSpacing: 'caps',
})

// Something on the right of a panel header: a count, a status ("found automatically").
export const panelCount = css({ ml: 'auto', color: 'fg.faint', fontWeight: 'medium', textTransform: 'none', letterSpacing: 0 })

export const table = css({
  width: '100%',
  borderCollapse: 'collapse',
  '& th': {
    textAlign: 'left',
    fontWeight: 'medium',
    fontSize: 'xs',
    color: 'fg.muted',
    px: '4',
    py: '2',
    borderBottom: '1px solid {colors.border.default}',
    whiteSpace: 'nowrap',
  },
  '& td': { px: '4', py: '2', borderBottom: '1px solid {colors.border.default}', verticalAlign: 'middle' },
  '& tr:last-child td': { borderBottom: 'none' },
  '& tbody tr:hover td': { bg: 'bg.subtle' },
})
// A table row a notification was about (see focus in router): a bar in warn, the colour of what
// needs attention, on its first cell.
export const pointedOut = css({ '& td:first-child': { boxShadow: 'inset 3px 0 0 {colors.warn}' } })

export const mono = css({ fontFamily: 'mono', fontSize: 'sm' })
// A short piece of code in a sentence: never broken at a hyphen or slash.
export const code = css({ fontFamily: 'mono', fontSize: 'sm', whiteSpace: 'nowrap' })
export const muted = css({ color: 'fg.muted' })
export const faint = css({ color: 'fg.faint' })

export const pageTitle = css({ fontSize: '3xl', fontWeight: 'bold', letterSpacing: 'snug' })

export const button = cva({
  base: {
    display: 'inline-flex',
    alignItems: 'center',
    justifyContent: 'center',
    gap: '1.5',
    borderRadius: 'control',
    fontWeight: 'medium',
    cursor: 'pointer',
    whiteSpace: 'nowrap',
    transitionProperty: 'background, border-color, color', transitionDuration: 'fast',
    border: '1px solid transparent',
    // One disabled look for every variant: neutral, so a disabled primary never reads as active.
    _disabled: { bg: 'bg.subtle', color: 'fg.faint', borderColor: 'border.default', cursor: 'default' },
    _focusVisible: { outline: '2px solid', outlineColor: 'accent.default', outlineOffset: '1px' },
  },
  variants: {
    variant: {
      primary: { bg: 'accent.default', color: 'fg.onAccent', '&:hover:enabled': { bg: 'accent.hover' } },
      secondary: { bg: 'bg.panel', color: 'fg.default', borderColor: 'border.strong', '&:hover:enabled': { bg: 'bg.subtle' } },
      // Icon-only buttons (close, open a terminal): a button with text gets a background (secondary).
      ghost: { bg: 'transparent', color: 'fg.muted', '&:hover:enabled': { bg: 'bg.hover', color: 'fg.default' } },
      danger: { bg: 'bg.panel', color: 'bad', borderColor: 'border.strong', '&:hover:enabled': { bg: 'badHover', borderColor: 'bad' } },
    },
    size: {
      sm: { h: 'control.sm', px: '2.5', fontSize: 'sm' },
      md: { h: 'control.md', px: '3', fontSize: 'base' },
      icon: { h: 'control.icon', w: 'control.icon', p: '0' },
    },
  },
  defaultVariants: { variant: 'secondary', size: 'md' },
})

// A button that reads as a link, for a secondary action inside text ("Details"): the text's own
// colour, underlined, so it works on any banner.
export const linkButton = css({
  display: 'inline', p: 0, bg: 'transparent', border: 'none', color: 'inherit', fontSize: 'inherit', fontWeight: 'strong',
  cursor: 'pointer', textDecoration: 'underline', textUnderlineOffset: '2px', textDecorationColor: 'color-mix(in srgb, currentColor 45%, transparent)',
  _hover: { textDecorationColor: 'currentColor' },
  _focusVisible: { outline: '2px solid', outlineColor: 'accent.default', outlineOffset: '1px', borderRadius: 'xs' },
})

// A link that goes somewhere (another page, a URL).
export const link = css({
  color: 'link', textDecoration: 'none', cursor: 'pointer', _hover: { textDecoration: 'underline', textUnderlineOffset: '2px' },
})

// A choice shown as a chip: a checkbox or radio inside a label. Ticked chips are selected (neutral).
export const chip = css({
  display: 'inline-flex', alignItems: 'center', gap: '1.5', h: 'control.icon', px: '2.5', borderRadius: 'control',
  border: '1px solid {colors.border.strong}', bg: 'bg.panel', cursor: 'pointer', fontSize: 'md', whiteSpace: 'nowrap',
  '&:has(input:checked)': { borderColor: 'selected.border', bg: 'selected.bg', color: 'selected.fg' },
  '&:has(input:disabled)': { cursor: 'default', color: 'fg.faint' },
})

// A choice of a few views or filters, side by side (Summary | YAML): role radiogroup, items radio.
export const segmented = css({ display: 'inline-flex', p: '0.5', gap: '0.5', bg: 'bg.subtle', borderRadius: 'control' })
export const segment = css({
  h: 'control.xs', px: '2.5', borderRadius: 'inner', fontSize: 'sm', fontWeight: 'medium', color: 'fg.muted', cursor: 'pointer',
  _hover: { color: 'fg.default' },
})
export const segmentOn = css({ bg: 'bg.panel', color: 'fg.default', fontWeight: 'semibold', boxShadow: 'panel' })

// Tabs under a page header; the current one is underlined in the text colour.
export const tab = css({
  px: '3', h: 'tab', fontWeight: 'medium', color: 'fg.muted', cursor: 'pointer', borderBottom: '2px solid transparent',
  mb: '-1px', _hover: { color: 'fg.default' },
})
export const tabOn = css({ color: 'selected.fg', borderColor: 'selected.border', fontWeight: 'semibold' })

export const badge = cva({
  base: {
    display: 'inline-flex',
    alignItems: 'center',
    gap: '1',
    h: 'badge',
    px: '2',
    borderRadius: 'full',
    fontSize: '2xs',
    fontWeight: 'strong',
    whiteSpace: 'nowrap',
  },
  variants: {
    tone: {
      neutral: { bg: 'bg.subtle', color: 'fg.muted' },
      ok: { bg: 'okSubtle', color: 'ok' },
      warn: { bg: 'warnSubtle', color: 'warn' },
      bad: { bg: 'badSubtle', color: 'bad' },
      info: { bg: 'infoSubtle', color: 'info' }, // in progress: busy, building
    },
  },
  defaultVariants: { tone: 'neutral' },
})

// A text field. A textarea with it keeps its own height (rows), only a one-line field is fixed.
export const input = css({
  '&:not(textarea)': { h: 'control.md' },
  px: '2.5',
  borderRadius: 'control',
  border: '1px solid {colors.border.strong}',
  bg: 'bg.panel',
  color: 'fg.default',
  fontSize: 'base',
  _focus: { outline: '2px solid', outlineColor: 'accent.default', outlineOffset: '-1px' },
  _placeholder: { color: 'fg.faint' },
})
