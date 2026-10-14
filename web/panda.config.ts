import { defineConfig } from '@pandacss/dev'

// jailwick's design tokens. Semantic colours switch with data-theme on <html> (light / dark),
// which follows the system unless the user picks one.
//
// Each colour has one job. accent (orange): primary actions and focus, nothing else. selected:
// the current tab, nav item or chip (neutral). link: navigation. info: something in progress
// (busy, building). warn: needs attention. ok / bad: healthy / failed.
export default defineConfig({
  preflight: true,
  presets: ['@pandacss/preset-base', '@pandacss/preset-panda'],
  include: ['./src/**/*.{svelte,ts}'],
  exclude: [],
  jsxFramework: undefined,
  outdir: 'styled-system',
  conditions: {
    extend: {
      dark: '[data-theme=dark] &',
      light: '[data-theme=light] &',
    },
  },
  globalCss: {
    'html, body': {
      bg: 'bg.canvas',
      color: 'fg.default',
      fontFamily: 'body',
      fontSize: 'base',
      lineHeight: 'normal',
      WebkitFontSmoothing: 'antialiased',
      height: '100%',
      overflow: 'hidden',
    },
    '#app': { height: '100%' },
    '::selection': { bg: 'accent.subtle' },
    // Checkboxes and radios are selection, not action: neutral, not the accent.
    'input[type=checkbox], input[type=radio]': { accentColor: 'fg.default' },
    'code, pre': { fontFamily: 'mono', fontSize: 'sm' },
    // Thin, light scrollbars. WebKit (the app's webview) uses the pseudo-elements; other
    // engines get scrollbar-width/color, which WebKit would otherwise let override them.
    '::-webkit-scrollbar': { width: '6px', height: '6px' },
    '::-webkit-scrollbar-track, ::-webkit-scrollbar-corner': { bg: 'transparent' },
    '::-webkit-scrollbar-thumb': { bg: 'border.strong', borderRadius: 'full' },
    '::-webkit-scrollbar-thumb:hover': { bg: 'fg.faint' },
    '@supports not selector(::-webkit-scrollbar)': {
      '*': { scrollbarWidth: 'thin', scrollbarColor: '{colors.border.strong} transparent' },
    },
  },
  theme: {
    extend: {
      tokens: {
        fonts: {
          body: { value: '-apple-system, BlinkMacSystemFont, "SF Pro Text", "Segoe UI", Inter, system-ui, sans-serif' },
          mono: { value: '"SF Mono", "JetBrains Mono", Menlo, Consolas, monospace' },
        },
        radii: {
          xs: { value: '2px' }, // focus rings on text
          sm: { value: '4px' }, // inline code, tags
          inner: { value: '5px' }, // a button inside a segmented control
          control: { value: '7px' }, // buttons, inputs, chips
          panel: { value: '10px' }, // panels, banners, dialogs
        },
        // The type scale: every font size in the app is one of these (fontSize: 'sm' in css(),
        // var(--font-sizes-sm) in a style attribute). Change a size here, not in components.
        fontSizes: {
          '2xs': { value: '11px' }, // badges, group labels
          xs: { value: '11.5px' }, // code blocks, table headers, small mono
          sm: { value: '12px' }, // hints and secondary text
          md: { value: '12.5px' }, // banners, dense body text
          base: { value: '13px' }, // body
          lg: { value: '14px' }, // dialog titles
          xl: { value: '15px' }, // section titles
          '2xl': { value: '16px' }, // sub-headings
          '3xl': { value: '20px' }, // page titles
        },
        fontWeights: {
          normal: { value: '400' },
          medium: { value: '500' }, // buttons, tabs, table headers
          strong: { value: '550' }, // badges, names in lists
          semibold: { value: '600' }, // labels, panel headers, emphasis
          bold: { value: '650' }, // titles
          heavy: { value: '700' }, // the brand name
        },
        lineHeights: {
          tight: { value: '1.2' }, // two-line list items
          normal: { value: '1.45' }, // body
          relaxed: { value: '1.5' }, // diffs, prose
          code: { value: '1.6' }, // logs and code blocks
          row: { value: '20px' }, // a line level with a 20px marker
        },
        letterSpacings: {
          tight: { value: '-0.015em' }, // page titles
          snug: { value: '-0.01em' }, // headings, the brand name
          caps: { value: '0.04em' }, // uppercase labels
        },
        durations: {
          fast: { value: '120ms' }, // hover and colour changes
          slow: { value: '300ms' }, // progress
        },
        sizes: {
          badge: { value: '20px' }, // badges, step numbers
          control: {
            xs: { value: '22px' }, // segmented-control buttons, inline chips
            compact: { value: '24px' }, // the theme switcher, small pickers
            sm: { value: '26px' }, // small buttons and selects
            icon: { value: '28px' }, // square icon buttons, choice chips
            md: { value: '30px' }, // buttons, inputs, nav items
          },
          row: { value: '32px' }, // wizard steps
          tab: { value: '34px' }, // tabs
          dot: { sm: { value: '6px' }, md: { value: '7px' }, lg: { value: '8px' } }, // status dots
          sidebar: { value: '208px' },
          label: { value: '150px' }, // the label column of a list of settings
          labelNarrow: { value: '96px' }, // the same, in a dialog
          projectList: { value: '260px' },
          content: { prose: { value: '560px' }, narrow: { value: '760px' }, wide: { value: '1180px' } },
          dialog: { md: { value: '760px' }, wide: { value: '1200px' } },
        },
      },
      semanticTokens: {
        colors: {
          bg: {
            canvas: { value: { base: '#f4f4f6', _dark: '#18181b' } },
            sidebar: { value: { base: '#ebebef', _dark: '#202024' } },
            panel: { value: { base: '#ffffff', _dark: '#232328' } },
            subtle: { value: { base: '#f2f2f5', _dark: '#2b2b31' } },
            hover: { value: { base: '#e6e6eb', _dark: '#2f2f36' } },
            inset: { value: { base: '#fafafb', _dark: '#1d1d21' } },
          },
          fg: {
            default: { value: { base: '#1c1c1f', _dark: '#ededf0' } },
            muted: { value: { base: '#6b6b74', _dark: '#9a9aa5' } },
            faint: { value: { base: '#9a9aa3', _dark: '#6c6c76' } },
            onAccent: { value: '#ffffff' },
          },
          border: {
            default: { value: { base: '#e2e2e8', _dark: '#34343b' } },
            strong: { value: { base: '#cfcfd7', _dark: '#4f4f59' } },
          },
          selected: {
            bg: { value: { base: '#e4e4ea', _dark: '#34343c' } },
            fg: { value: { base: '#1c1c1f', _dark: '#f4f4f6' } },
            border: { value: { base: '#1c1c1f', _dark: '#d4d4da' } },
          },
          link: { value: { base: '#2563eb', _dark: '#7aa7ff' } },
          info: { value: { base: '#1c6fc4', _dark: '#74b4f5' } },
          infoSubtle: { value: { base: '#e8f2fc', _dark: '#16283b' } },
          accent: {
            default: { value: { base: '#e8590c', _dark: '#ff8a3d' } },
            hover: { value: { base: '#d9480f', _dark: '#ff9d5c' } },
            subtle: { value: { base: '#fff1e6', _dark: '#3a2618' } },
            fg: { value: { base: '#c2410c', _dark: '#ffa45c' } },
          },
          ok: { value: { base: '#2f9e44', _dark: '#51cf66' } },
          okSubtle: { value: { base: '#ebfbee', _dark: '#1b3021' } },
          warn: { value: { base: '#946200', _dark: '#ffd36b' } },
          warnSubtle: { value: { base: '#fff6dc', _dark: '#2f2914' } },
          bad: { value: { base: '#e03131', _dark: '#ff6b6b' } },
          badSubtle: { value: { base: '#fff5f5', _dark: '#3b1d1d' } },
          badHover: { value: { base: '#fde4e4', _dark: '#4d2323' } }, // a danger button under the pointer
          backdrop: { value: { base: 'rgba(0,0,0,0.45)', _dark: 'rgba(0,0,0,0.6)' } }, // behind dialogs
        },
        shadows: {
          panel: { value: { base: '0 1px 2px rgba(0,0,0,0.04), 0 0 0 1px rgba(0,0,0,0.04)', _dark: '0 0 0 1px rgba(255,255,255,0.04)' } },
          pop: { value: { base: '0 8px 30px rgba(0,0,0,0.12)', _dark: '0 8px 30px rgba(0,0,0,0.5)' } },
          dialog: { value: { base: '0 20px 60px rgba(0,0,0,0.35)', _dark: '0 20px 60px rgba(0,0,0,0.6)' } },
        },
      },
    },
  },
})
