<script lang="ts">
  import { css } from 'styled-system/css'
  import { mono, muted, panel, panelHeader, table } from '../lib/styles'
  import Page from '../lib/Page.svelte'
  import Icon from '../components/Icon.svelte'

  // The keys the app answers to, and the quickest ways to do things from a terminal.
  const keys = [
    { keys: ['Esc'], what: 'Close a dialog (Details, a request) or a menu' },
    { keys: ['Tab', '⇧ Tab'], what: 'Move to the next or previous control' },
    { keys: ['Enter', 'Space'], what: 'Press the focused button, tick the focused box' },
    { keys: ['←'], what: 'The Back button next to “jailwick”: the page before this one' },
  ]
  const commands = [
    { cmd: 'jw claude', what: 'Claude in this repo’s sandbox, started if it isn’t running' },
    { cmd: 'jw claude last', what: 'Continue the latest conversation here (pick: choose one)' },
    { cmd: 'jw wt <name>', what: 'Claude in a worktree with its own branch, ports and database copies' },
    { cmd: 'jw shell', what: 'A shell in the sandbox' },
    { cmd: 'jw resume', what: 'Pick any saved conversation and resume it' },
    { cmd: 'jw allow <domain>', what: 'Let every sandbox reach a site (--project: this one only; --preset <name>: a group)' },
    { cmd: 'jw blocked', what: 'What the sandbox tried to reach and was refused' },
    { cmd: 'jw internet open', what: 'One session reaches the whole internet for 15 minutes' },
    { cmd: 'jw status', what: 'The sandbox, its services and its pending changes' },
    { cmd: 'jw restart', what: 'Apply pending changes (refuses while sessions run; --force ends them)' },
    { cmd: 'jw down', what: 'Stop the sandbox; its databases, login, history and images are kept' },
    { cmd: 'jw app', what: 'Open this app' },
    { cmd: 'jw doctor', what: 'Check the runtime, settings, secrets and images' },
  ]
  const flags = [
    { flag: '-p <project>', what: 'Act on a project from anywhere (default: the one of this folder)' },
    { flag: '-w <worktree>', what: 'Act on a worktree of it' },
    { flag: '-v', what: 'Show the runtime’s own output (image builds, compose)' },
  ]

  const grid = css({ display: 'grid', gridTemplateColumns: { base: '1fr', xl: '1fr 1fr' }, gap: '5', alignItems: 'start' })
  const full = css({ gridColumn: '1 / -1' })
  const kbd = css({
    display: 'inline-block', minW: '22px', px: '1.5', py: '0.5', mr: '1', borderRadius: 'inner', textAlign: 'center',
    border: '1px solid {colors.border.strong}', borderBottomWidth: '2px', bg: 'bg.subtle', fontFamily: 'mono', fontSize: 'xs',
  })
  const note = css({ px: '4', py: '3', fontSize: 'md' })
</script>

<Page title="Shortcuts" subtitle="The app’s keys, and the quickest commands in a terminal.">
  <div class={grid}>
    <section class={panel}>
      <div class={panelHeader}><Icon name="monitor" size={14} />In the app</div>
      <table class={table}>
        <tbody>
          {#each keys as k}
            <tr><td style="white-space:nowrap">{#each k.keys as key}<span class={kbd}>{key}</span>{/each}</td><td>{k.what}</td></tr>
          {/each}
        </tbody>
      </table>
      <p class="{note} {muted}">The browser’s back keys, mouse buttons and swipes do nothing here on purpose, so a half-filled form is never lost.</p>
    </section>

    <section class={panel}>
      <div class={panelHeader}><Icon name="settings" size={14} />Flags every command takes</div>
      <table class={table}>
        <tbody>
          {#each flags as f}<tr><td class={mono} style="white-space:nowrap">{f.flag}</td><td>{f.what}</td></tr>{/each}
        </tbody>
      </table>
      <p class="{note} {muted}"><span class={mono}>jw</span> is short for <span class={mono}>jailwick</span>; <span class={mono}>jailwick alias add</span> adds it when it’s missing.</p>
    </section>

    <section class="{panel} {full}">
      <div class={panelHeader}><Icon name="terminal" size={14} />In a terminal, in a project’s folder</div>
      <table class={table}>
        <tbody>
          {#each commands as c}<tr><td class={mono} style="white-space:nowrap">{c.cmd}</td><td>{c.what}</td></tr>{/each}
        </tbody>
      </table>
      <p class="{note} {muted}">Every command explains itself with <span class={mono}>--help</span>.</p>
    </section>
  </div>
</Page>
