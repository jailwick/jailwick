# jailwick

Run AI coding agents (Claude Code first) in a locked-down sandbox per repo:

- **Only the repo is visible.** Related repos can be shown read-only, with their secrets hidden.
- **The internet only through an allowlist.** A proxy lets through the domains you allow; the host,
  your network and everything else are unreachable. Each session has its own proxy, so you can open
  the whole internet to one session for a while (research) and keep the others on the allowlist.
- **No root, no capabilities, no Docker socket, no SSH keys** inside the sandbox.
- **Its own databases** (Postgres, Redis, MongoDB), plus disposable ones for test suites.
- **Parallel sessions**, each in its own container, and in git worktrees with their own ports,
  database copies and Redis database.
- **Checks itself:** `jw verify` proves the isolation on a running sandbox.

```
$ jw project add            # in a repo: detects services and env files
$ jw claude                 # Claude in the sandbox
$ jw wt feat/login          # another session, in a worktree with its own ports and databases
$ jw status
● api  ~/code/api
  └ main           12m  idle
  └ wt feat/login  3m   busy
```

A website and full documentation are coming soon.

## Install

Needs a container runtime: OrbStack (recommended on macOS), Docker Desktop, Colima or Docker
Engine.

There are no packages yet (no Homebrew or apt): build it from source. Needs Go 1.27+ and Node 22+
(to build the app's front end).

```bash
make app install    # jailwick into ~/.local/bin, and the desktop app (below)
jailwick init       # runtime, git identity, Claude login, and the short command jw
```

The desktop app is installed as a regular app for the system you build on:

- **macOS:** `jailwick.app` in `~/Applications` (`APPS=/Applications` for all users), so it is in
  Launchpad and Spotlight and can stay in the Dock.
- **Linux:** `jailwick-app` in `~/.local/bin`, with a desktop entry and icon in `~/.local/share`.
- **Windows:** `make app-windows` (from any OS) builds `bin/jailwick-app.exe` with its icon.

`make` without `app` installs only the CLI; `jw app` then opens the same app in your browser.

## App

`jw app` opens the desktop app: projects, sessions, worktrees, services, logs, the isolation
checks, blocked requests (allow them with one click), allowlist presets (your own too),
workspaces and handoffs, in light or dark. A project's Claude login, Claude in Chrome and settings
profiles, and secrets, can be changed there too.
A menu bar item lists every project with Start, Stop, Restart and New session. `jw app --browser`
serves the same app to your browser on `127.0.0.1`, behind a one-time token.

## Use

| | |
|---|---|
| `jw project add [dir]` | Add a repo, step by step: services and database logins, worktrees and image. Prefilled from its `.env` files (or `.env.example`). The app has the same steps under New project. |
| `jw login` · `jw login status` · `jw logout` | Sign Claude in once for every project on the app login, or a project's own (`claude.login: project`). Remote Control and Claude in Chrome need it. |
| `jw claude [last\|pick\|id]` | Start (or resume) Claude in the sandbox; in a worktree folder, in that worktree. `--chrome` lets one session use Claude in Chrome (off by default). |
| `jw wt <name>` · `jw wt ls` · `jw wt rm <name>` | Worktree sessions with their own resources. |
| `jw shell` · `jw run <cmd>` · `jw db` · `jw redis` · `jw mongo` | Shells and clients, in the current worktree's environment. |
| `jw up` · `jw restart` · `jw down` · `jw destroy` · `jw status` · `jw verify` | Sandbox life cycle. `up` never ends running sessions; it reports pending changes. `down` keeps data and images, `destroy` removes the project's images (`--volumes`: also its data); both need `--include-sessions` while sessions run. |
| `jw allow <domain>` · `jw allow --preset <name>` · `jw blocked` | The allowlist (domains, or whole presets for a project), and what the proxies refused. `jw proxy presets add <name> <domains>` makes a preset of your own. |
| `jw internet open [session] [--for 15m]` · `jw internet close` · `jw claude --open` | Let one session reach the whole internet for a while, or from its start; your machine and network stay blocked. |
| `jw workspace add` · `jw hidden` · `jw handoffs` | Related repos read-only, and requests between their sessions. |
| `jw history` · `jw resume` | Saved conversations (sandbox or `--host`). |
| `jw settings edit [--app \| --profile NAME]` · `jw settings show` | Claude's settings in layers: every project, named profiles (`claude.profiles`), one project. |
| `jw import` | Bring projects over from a csb setup. |
| `jw app` | The desktop app (or `--browser`). |

`jw <command> --help` explains each one.

## Configuration

```
~/.config/jailwick/            ($JAILWICK_HOME)
  config.yaml                  runtime, defaults, global allowlist
  projects/<name>.yaml         one per repo
  workspaces/<name>.yaml       groups of related repos
  notes/projects/<name>/       notes.md for Claude (and optional settings.json / mcp.json)
  claude/settings.json         Claude settings for every project
  claude/profiles/<name>.json  Claude settings profiles, for the projects that list them
  presets/<name>.yaml          allowlist presets of your own
  allowlist.txt                your domains, for every sandbox
~/.local/state/jailwick/       generated files, safe to delete ($JAILWICK_STATE)
```

Secrets (`CLAUDE_CODE_OAUTH_TOKEN`, `ANTHROPIC_API_KEY`, `GH_TOKEN`, …) never go in YAML: `jw secret
set` stores them in the macOS Keychain (or a 0600 file, or environment variables in CI).

Claude signs in with `jw login`, once: the login is kept in an app volume (`jailwick-claude-login`)
that every project with `claude.login: app` (the default) shares, and that removing a project never
deletes. A project with `claude.login: project` keeps its own, for example another account. Each
project's conversations, checkpoints, plans and prompt history stay in its own volume either way.
`CLAUDE_CODE_OAUTH_TOKEN` or `ANTHROPIC_API_KEY`, when set, replace the login: chat works, Remote
Control and Claude in Chrome don't.

The sandbox image is built on first use: a minimal base shared by every project (Debian, Node.js,
git, gh, the usual CLI tools), then a layer per project with its stacks at the versions the repo
names (`image.stacks`: node, python, go, rust, bun, java, ruby), Playwright browsers when chosen, apt packages
and Claude Code. `jw image build --fresh` rebuilds without the cache; `jw image prune` removes the
images no project uses. A project can build on an image of yours instead (a base image, a Dockerfile,
or an image used as is), and `jw image check` tries one first: see [Sandbox images](docs/IMAGES.md).

If `claude --version` in a sandbox stays old after `jw image update`, the sandbox predates Claude
Code moving out of the `~/.local` volume: remove that volume once (`docker volume rm
jailwick-<project>_user-tools`; `jw destroy --volumes` would also delete the databases).

## Docs

A documentation site is coming soon. Until then, these pages cover it:

- [Command reference](docs/CLI.md): every command and flag (generated from the code).
- [Local API](docs/API.md): the JSON API behind the app.
- [Sandbox images](docs/IMAGES.md): stacks, your own base image or Dockerfile, checking one.
- [Runtimes](docs/RUNTIMES.md): supported runtimes and known limits.
- [Network address pools](docs/NETWORK-POOLS.md): when the runtime runs out of room for networks,
  and how to give it more (OrbStack, Docker Desktop, Colima, Linux).
- [Security](SECURITY.md): what the sandbox protects and what it doesn't, and how to report a
  problem; with [repo files your Mac runs](docs/HOST-EXECUTION.md) and
  [data from the sandbox](docs/UNTRUSTED-DATA.md).
- [Releasing](docs/RELEASING.md): building, signing and publishing a release.

## Develop

```bash
make test     # unit tests (race detector)
make lint     # gofmt + go vet
make build    # bin/jailwick, with the Linux agents embedded
make app      # bin/jailwick-app, the desktop app (cgo)
make web      # rebuild the front end (web/ → internal/webui/dist, not committed)
```

`cmd/jailwick` is the CLI, `cmd/jailwick-agent` the helper mounted into every sandbox,
`cmd/jailwick-app` the desktop app. The code is in `internal/`, one package per concern (config,
spec, compose, runtime, sandbox, workspace, server, …). The front end is `web/`: Svelte 5,
TypeScript and Panda CSS; it talks to the local API in `internal/server`.

## License

MIT
