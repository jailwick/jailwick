<script lang="ts">
  import { css } from 'styled-system/css'
  import { go } from '../lib/router.svelte'
  import { linkButton, mono, muted, panel } from '../lib/styles'
  import Page from '../lib/Page.svelte'
  import Icon from '../components/Icon.svelte'

  // How to use jailwick, flow by flow: what to do in the app and the matching command.
  type Section = { icon: string; title: string; page?: [string, string]; steps: string[]; cli?: string }
  const sections: Section[] = [
    {
      icon: 'plus', title: 'Add a project', page: ['Projects', 'new-project'],
      steps: [
        'New project, then choose the repo’s folder.',
        'Answers are prefilled from the repo: its services and database logins (from its env files), toolchains, the sites it needs.',
        'Check each step, then Create project on the review. It is saved in jailwick’s settings, not in the repo.',
      ],
      cli: 'jw project add',
    },
    {
      icon: 'play', title: 'Start it and open a session', page: ['Projects', 'projects'],
      steps: [
        'Start on the project’s page. The first start sets up the sandbox image: a few minutes; Details shows its progress.',
        'New session opens Claude in your terminal, in the sandbox. Shell opens a shell there.',
        'Open session: one session that may reach the whole internet until it ends; the others stay on the allowlist.',
        'A project’s sessions can message each other. Private (Sessions tab, or --private) keeps one out.',
        'Ctrl+V pastes a copied image or screenshot into a session, as outside the sandbox.',
      ],
      cli: 'jw claude',
    },
    {
      icon: 'branch', title: 'Work on several things at once',
      steps: [
        'A worktree is its own branch, ports, database copies and Redis database, next to the main checkout.',
        'Create one from a terminal; the project page lists them and opens sessions in them.',
      ],
      cli: 'jw wt <name>',
    },
    {
      icon: 'network', title: 'Let the sandbox reach a site', page: ['Network', 'network'],
      steps: [
        'Sandboxes reach only allowlisted sites. Blocked requests are listed on the Network page.',
        'Allow a single site, or a preset (a group of sites, e.g. a package registry), for every project or one.',
        'For research, open one session to the whole internet for a while instead.',
      ],
      cli: 'jw allow <domain>',
    },
    {
      icon: 'workspaces', title: 'Group related repos', page: ['Workspaces', 'workspaces'],
      steps: [
        'New workspace, then choose the folder that holds the repos (e.g. one with api/ and web/).',
        'Each project in it sees the other repos read-only, with their secrets (.env, keys) hidden.',
        'Sessions leave each other requests in the handoff folder; the workspace page lists them.',
      ],
      cli: 'jw workspace add <name> <folder>',
    },
    {
      icon: 'restart', title: 'Apply changes',
      steps: [
        'A change of settings reaches a running sandbox at its next restart. The project page lists what is pending; Details shows each change.',
        'Restart ends the project’s sessions, so it asks first while some run.',
      ],
      cli: 'jw restart',
    },
    {
      icon: 'inbox', title: 'Tell Claude how the project works',
      steps: [
        'The Notes tab of a project: how to run and test it in the sandbox. Every session reads it at the start.',
        'A workspace has notes too, for what all its sessions should know.',
      ],
      cli: 'jw notes',
    },
    {
      icon: 'settings', title: 'Logins, secrets and defaults', page: ['Settings', 'settings'],
      steps: [
        'Settings: the Claude login (one for all projects, or per project), secrets such as GH_TOKEN, and general settings such as the runtime and the terminal sessions open in.',
        'Secrets stay out of the project files; sandboxes get them at start.',
      ],
      cli: 'jw secret set NAME',
    },
    {
      icon: 'alert', title: 'When something is wrong',
      steps: [
        'A failed start keeps its error on the project page; Details shows the last lines and the log file.',
        'Check the setup with doctor, and a running sandbox’s isolation with verify.',
      ],
      cli: 'jw doctor · jw verify',
    },
  ]

  const grid = css({ display: 'grid', gridTemplateColumns: { base: '1fr', xl: '1fr 1fr' }, gap: '5', alignItems: 'start' })
  const card = css({ p: '4', display: 'grid', gap: '2.5' })
  const head = css({ display: 'flex', alignItems: 'center', gap: '2', fontWeight: 'bold', fontSize: 'lg', '& svg': { color: 'fg.muted' } })
  const steps = css({ m: 0, pl: '5', display: 'grid', gap: '1.5', fontSize: 'base', lineHeight: 'relaxed' })
  const foot = css({ display: 'flex', gap: '3', alignItems: 'center', flexWrap: 'wrap', fontSize: 'sm' })
</script>

<Page title="Guide" subtitle="How to use jailwick: each flow in the app, and the matching command.">
  <div class={grid}>
    {#each sections as s}
      <section class="{panel} {card}">
        <div class={head}><Icon name={s.icon} size={15} />{s.title}</div>
        <ol class={steps}>{#each s.steps as step}<li>{step}</li>{/each}</ol>
        <div class={foot}>
          {#if s.page}<button class={linkButton} onclick={() => go(s.page![1])}>Open {s.page[0]}</button>{/if}
          {#if s.cli}<span class={muted}>In a terminal: <span class={mono}>{s.cli}</span></span>{/if}
        </div>
      </section>
    {/each}
  </div>
</Page>
