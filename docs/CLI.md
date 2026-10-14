# Command reference

Generated from the code by `make docs`; don't edit by hand. `jw` and `jailwick` are the same
command. `jw <command> --help` prints the same text in the terminal.

## Global flags

Every command accepts these:

| Flag | Default | Meaning |
|---|---|---|
| `--plain` |  | plain output: no colours or prompts |
| `-p`, `--project` |  | project (default: detected from the current folder) |
| `-v`, `--verbose` |  | show runtime output (builds, compose) |
| `-w`, `--worktree` |  | worktree (default: detected from the current folder) |

## Commands

**Getting started** [`app`](#app) · [`doctor`](#doctor) · [`import`](#import) · [`init`](#init) · [`login`](#login) · [`logout`](#logout) · [`project`](#project) · [`trace`](#trace)

**Sessions** [`claude`](#claude) · [`db`](#db) · [`history`](#history) · [`mongo`](#mongo) · [`redis`](#redis) · [`resume`](#resume) · [`run`](#run) · [`sessions`](#sessions) · [`shell`](#shell) · [`stop-sessions`](#stop-sessions) · [`wt`](#wt)

**Sandboxes** [`destroy`](#destroy) · [`down`](#down) · [`logs`](#logs) · [`open`](#open) · [`ports`](#ports) · [`projects`](#projects) · [`restart`](#restart) · [`stats`](#stats) · [`status`](#status) · [`top`](#top) · [`up`](#up) · [`url`](#url) · [`verify`](#verify)

**Network** [`allow`](#allow) · [`blocked`](#blocked) · [`deny`](#deny) · [`internet`](#internet) · [`proxy`](#proxy)

**Configuration** [`alias`](#alias) · [`config`](#config) · [`handoffs`](#handoffs) · [`hidden`](#hidden) · [`image`](#image) · [`notes`](#notes) · [`path`](#path) · [`secret`](#secret) · [`settings`](#settings) · [`where`](#where) · [`workspace`](#workspace)

---

## Getting started

<a id="app"></a>
### `jw app`

Open jailwick's app: projects, sessions, worktrees, network and logs at a glance.

The desktop app is used when it is installed (macOS: jailwick.app in ~/Applications or
/Applications; elsewhere jailwick-app next to jailwick or on PATH). --browser serves the same app
on 127.0.0.1 with a one-time token instead, and opens it in your browser.

```
jw app [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--browser` |  | serve the app to your browser |
| `--no-open` |  | print the link without opening the browser |
| `--port` |  | port on 127.0.0.1 (default: a free one) |

<a id="doctor"></a>
### `jw doctor`

Check the runtime, configuration, secrets and images

```
jw doctor
```

<a id="import"></a>
### `jw import`

Convert a csb setup (claude-container-setup) into jailwick configuration:
  projects/*.env      → projects/<name>.yaml (tokens go to the secret store, never into YAML)
  workspaces/*.env    → workspaces/<name>.yaml
  config/*/notes.md   → notes (with /etc/sandbox paths updated)
  proxy allowlist     → your allowlist.txt (domains no built-in preset covers)
The csb folder is only read. Existing jailwick projects are kept unless --force.
Imported projects keep csb's --dangerously-skip-permissions (claude.yolo: true).

```
jw import [csb-folder] [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--dry-run` |  | show what would be imported (no secret values) |
| `--force` |  | replace existing jailwick projects and workspaces |
| `--only` |  | import only these projects (and their workspaces) |

<a id="init"></a>
### `jw init`

Set up jailwick: runtime, git identity, Claude login, the jw command

```
jw init
```

<a id="login"></a>
### `jw login`

Claude's login is kept by the app (claude.login: app, the default), shared by every project
that uses it, or by the project itself (claude.login: project, e.g. another account). Signing in
runs claude auth login in the project's sandbox: open the link it prints, then paste the code.

```
jw login
```

<a id="login-status"></a>
#### `jw login status`

Show which Claude login the project uses and who is signed in

```
jw login status
```

<a id="logout"></a>
### `jw logout`

Sign Claude out of the login the project uses (the app login: every project on it)

```
jw logout
```

<a id="project"></a>
### `jw project`

Add, list, show, edit or remove projects

Also: `projects-config`.

<a id="project-add"></a>
#### `jw project add`

Add a repo as a project, step by step:
  1. Project: its name.
  2. Services and database logins: Postgres, Redis, MongoDB, their logins, and forwards so
     localhost:<port> inside the sandbox reaches them.
  3. Toolset: the stacks in the sandbox image (Node.js, Python, Go, Rust, Bun) at the versions the
     repo names, Playwright browsers, apt packages.
  4. Network access: the allowlist presets the repo and its stacks call for (package registries,
     services), ticked; broad ones, which accept data from anyone, are suggested but not ticked.
  5. Worktrees: files copied into new worktrees, setup commands.
Answers are prefilled from the repo's .env files (and .env.example when there's no .env).
Every setting also has a flag; --yes takes the prefilled answers without asking.

```
jw project add [dir] [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--apt` |  | extra apt packages, e.g. libvips-dev (repeatable) |
| `--base` |  | base image |
| `--browsers` |  | playwright browsers: chromium, firefox, webkit |
| `--bun-version` |  | bun version |
| `--copy` |  | files copied into new worktrees, e.g. .env (repeatable) |
| `--dockerfile` |  | dockerfile |
| `--env-file` |  | read logins from |
| `--forward` |  | local forwards, e.g. 5433:postgres:5432 (repeatable) |
| `--go-version` |  | go version |
| `--image` |  | sandbox image: default, base, dockerfile, image |
| `--image-home` |  | home folder |
| `--image-name` |  | image |
| `--image-user` |  | sandbox user |
| `--java-version` |  | java version |
| `--mongo-password` |  | mongodb password |
| `--mongo-user` |  | mongodb user |
| `--name` |  | project name |
| `--node-version` |  | node.js version |
| `--postgres-db` |  | postgres database |
| `--postgres-password` |  | postgres password |
| `--postgres-user` |  | postgres user |
| `--presets` |  | allowlist presets, e.g. npm, pypi |
| `--python-version` |  | python version |
| `--redis-password` |  | redis password |
| `--ruby-version` |  | ruby version |
| `--run` |  | run command |
| `--rust-version` |  | rust version |
| `--services` |  | services: postgres, redis, mongo |
| `--setup` |  | worktree setup commands, e.g. pnpm install (repeatable) |
| `--stacks` |  | stacks: node, python, go, rust, bun, java, ruby |
| `--workspace` |  | workspace (repeatable) |
| `--workspace-folder` |  | its folder in the workspace: show, hide |
| `-y`, `--yes` |  | take the prefilled settings without asking |

<a id="project-edit"></a>
#### `jw project edit`

Edit a project's file in $EDITOR, validated on save

```
jw project edit [name]
```

<a id="project-ls"></a>
#### `jw project ls`

List projects

```
jw project ls
```

<a id="project-rm"></a>
#### `jw project rm`

Remove a project from jailwick. Its sandbox is stopped first, so none is left running
without a project; that refuses while sessions run unless --include-sessions. Its data
(databases, history, its own Claude login) is kept unless --purge, which also removes its images (as jw destroy
--volumes), so adding the project again with the same name finds it. The repo and its notes are
untouched.

```
jw project rm <name> [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--include-sessions` |  | also end running sessions |
| `--purge` |  | also delete its images and data |
| `--yes` |  | don't ask |

<a id="project-show"></a>
#### `jw project show`

Print a project's settings (--resolved: with defaults filled in)

```
jw project show [name] [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--resolved` |  | fill in defaults |

<a id="trace"></a>
### `jw trace`

Without an argument, sum up the trace: each kind of runtime command jailwick ran (docker ps,
exec jailwick-agent status, logs…), how often, how long it took and how much it printed, the
costliest first.

  on     start tracing: jailwick, the app included, logs every runtime command it runs
  off    stop tracing (the log stays)
  clear  empty the log

The log is trace.log in the state folder. JAILWICK_TRACE=1 traces one command, without the switch.

```
jw trace [on|off|clear] [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--minutes` |  | only the last N minutes (default: the whole log) |

---

## Sessions

<a id="claude"></a>
### `jw claude`

Start Claude in the project's sandbox, attached to this terminal. Run it in the repo for the
main checkout, or in .jailwick/worktrees/<name> (or with -w) for that worktree.

RESUME: last (most recent conversation here), pick (choose one), or a conversation id.

```
jw claude [last|pick|ID] [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--chrome` |  | let this session use Claude in Chrome (your signed-in browser), whatever claude.chrome says |
| `--no-chrome` |  | keep Claude in Chrome off for this session, whatever claude.chrome says |
| `--open` |  | let this session reach the whole internet until it ends (others stay on the allowlist) |
| `--private` |  | keep this session out of messages between sessions: it can't see or message the project's other sessions, nor they it |

<a id="db"></a>
### `jw db`

Open psql on the project's database (the worktree's copy in a worktree)

```
jw db [psql args]
```

<a id="history"></a>
### `jw history`

List saved conversations, newest first, with their id, place and first prompt.

The sandbox and the host keep separate histories: a conversation started in one can't be resumed
in the other. --host lists the host's, for a folder (default: here, with subfolders) or --all.

```
jw history [folder] [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--all` |  | with --host: every folder |
| `--folders` |  | count conversations per folder (--host) |
| `--host` |  | conversations of Claude on this machine, not the sandbox |
| `-n`, `--lines` | `20` | how many |
| `--missing` |  | with --host: include folders that no longer exist |

<a id="mongo"></a>
### `jw mongo`

Open mongosh on the project's MongoDB (shared by all worktrees)

```
jw mongo [mongosh args]
```

<a id="redis"></a>
### `jw redis`

Open redis-cli on the project's Redis (the worktree's database in a worktree)

```
jw redis [redis-cli args]
```

<a id="resume"></a>
### `jw resume`

Pick a saved conversation and resume it where it ran (sandbox, or --host)

```
jw resume [folder] [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--all` |  | with --host: every folder |
| `--chrome` |  | let this session use Claude in Chrome (your signed-in browser), whatever claude.chrome says |
| `--host` |  | conversations of Claude on this machine, not the sandbox |
| `-n`, `--lines` | `20` | how many |
| `--missing` |  | with --host: include folders that no longer exist |
| `--no-chrome` |  | keep Claude in Chrome off for this session, whatever claude.chrome says |
| `--open` |  | let this session reach the whole internet until it ends (others stay on the allowlist) |
| `--private` |  | keep this session out of messages between sessions: it can't see or message the project's other sessions, nor they it |

<a id="run"></a>
### `jw run`

Run a command in the sandbox (one argument runs through bash -c)

```
jw run -- COMMAND [ARGS...]
```

<a id="sessions"></a>
### `jw sessions`

List Claude sessions (this project, or --all)

```
jw sessions [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--all` |  | every running sandbox |

<a id="shell"></a>
### `jw shell`

Open a shell in the sandbox (in a worktree folder: with that worktree's environment)

```
jw shell
```

<a id="stop-sessions"></a>
### `jw stop-sessions`

End the Claude sessions of this project (or --all), e.g. before a restart

```
jw stop-sessions [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--all` |  | every running sandbox |

<a id="wt"></a>
### `jw wt`

Create (or reuse) the worktree <name> at .jailwick/worktrees/<name> on branch <name>, give it
its own port range, database copies and Redis database, and start Claude in it.

RESUME: last (most recent conversation here), pick (choose one), or a conversation id.

```
jw wt [name] [last|pick|ID] [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--base` | `HEAD` | branch or commit a new worktree starts from |
| `--chrome` |  | let this session use Claude in Chrome (your signed-in browser), whatever claude.chrome says |
| `--no-chrome` |  | keep Claude in Chrome off for this session, whatever claude.chrome says |
| `--open` |  | let this session reach the whole internet until it ends (others stay on the allowlist) |
| `--private` |  | keep this session out of messages between sessions: it can't see or message the project's other sessions, nor they it |

<a id="wt-ls"></a>
#### `jw wt ls`

List worktrees with their port, databases and Redis database

```
jw wt ls
```

<a id="wt-rm"></a>
#### `jw wt rm`

Remove a worktree and its databases (the branch is kept)

```
jw wt rm [name] [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--force` |  | remove even with uncommitted changes |

<a id="wt-shell"></a>
#### `jw wt shell`

Open a shell in a worktree, with its environment

```
jw wt shell [name]
```

---

## Sandboxes

<a id="destroy"></a>
### `jw destroy`

Remove the sandbox and the images built for its project that no other project uses (the
shared base and proxy stay while others need them). Its data (databases, caches, conversation
history and its own Claude login) is kept unless --volumes, which asks you to type the name first.
The app's Claude login (claude.login: app) is shared by projects and always stays. It refuses while
sessions run unless --include-sessions. A sandbox whose project file is gone is removed the same
way: jw destroy -p <name>.

```
jw destroy [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--include-sessions` |  | also end running sessions |
| `--volumes` |  | also delete its data (databases, caches, history, its own Claude login) |
| `--yes` |  | don't ask for confirmation |

<a id="down"></a>
### `jw down`

Stop the sandbox: its containers and networks are removed; its data (databases, caches,
Claude login and history) and images are kept, so jw up starts it again as it was. It refuses
while sessions run unless --include-sessions (sessions use the sandbox's networks). A sandbox
whose project file is gone is removed the same way: jw down -p <name>.

```
jw down [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--all` |  | stop every sandbox |
| `--include-sessions` |  | also end running sessions |

<a id="logs"></a>
### `jw logs`

Show container logs (all services, or one: claude, egress-proxy, postgres, …)

```
jw logs [service] [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `-f`, `--follow` |  | keep following |
| `-n`, `--tail` | `200` | lines from the end (0: all) |

<a id="open"></a>
### `jw open`

Open a port of the sandbox in the browser

```
jw open [port]
```

<a id="ports"></a>
### `jw ports`

List listening ports in the sandbox, with their command and session

```
jw ports
```

<a id="projects"></a>
### `jw projects`

List projects with their state and sessions

```
jw projects
```

Also: `ls`.

<a id="restart"></a>
### `jw restart`

Apply pending changes by recreating the sandbox (refuses while sessions run)

```
jw restart [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--force` |  | restart even with sessions running (ends them) |

<a id="stats"></a>
### `jw stats`

Live CPU and memory of all running sandboxes

```
jw stats
```

<a id="status"></a>
### `jw status`

Show the sandbox: containers, sessions, worktrees, pending changes

```
jw status
```

<a id="top"></a>
### `jw top`

CPU and memory of the sandbox and each session (one container each)

```
jw top [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--once` |  | print one snapshot |

<a id="up"></a>
### `jw up`

Start the project's sandbox: build missing images, then start the containers. A paused
sandbox resumes where it was.

A running sandbox is never recreated, because that would end its sessions. If the configuration,
image or jailwick changed since it started, up says so; jw restart applies the change.

With a service (egress-proxy, postgres, …), up starts that one again: one that stopped, crashed
or was paused. It is recreated alone (its data stays); the sessions carry on.

```
jw up [service]
```

<a id="url"></a>
### `jw url`

Print the URL of a port in the sandbox (in a worktree: its first port)

```
jw url [port]
```

<a id="verify"></a>
### `jw verify`

Run jailwick's isolation checks inside the sandbox: non-root without capabilities, only the
expected mounts (read-only where they should be), no runtime socket or SSH keys, allowed hosts
reachable through the proxy, everything else and the host blocked, no way around the proxy (no
outside name resolution, no public DNS servers, no IPv6, no other sandbox's services), services
and forwards up. It installs nothing and changes nothing.

```
jw verify [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--all` |  | every running sandbox |
| `--json` |  | print JSON |

---

## Network

<a id="allow"></a>
### `jw allow`

Add domains to the egress allowlist and reload the running proxies. A leading dot (.example.com)
or *.example.com also allows subdomains. By default domains go to your own allowlist
(`allowlist.txt`, all projects); --project adds them to this project only.

--preset adds whole presets (jw proxy presets lists them) to this project's allow_presets.
--check <host> tells whether a host is allowed, and by which entry.

```
jw allow <domain>... [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--check` |  | tell whether a host is allowed |
| `--preset` |  | add presets to this project (repeat, or comma-separate) |
| `--project` |  | only for this project |

<a id="blocked"></a>
### `jw blocked`

Show requests the proxy refused, most frequent first

```
jw blocked [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--all` |  | all running sandboxes |
| `-n`, `--lines` | `30` | how many hosts |

<a id="deny"></a>
### `jw deny`

Remove domains you allowed, or presets from this project (--preset)

```
jw deny <domain>... [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--preset` |  | remove presets from this project (repeat, or comma-separate) |
| `--project` |  | from this project's list |

<a id="internet"></a>
### `jw internet`

Each session has its own egress proxy. Opening it lets that session reach every host and port
for a while (default 15 minutes) or until closed: e.g. so Claude can research. Your machine,
your network and cloud metadata stay blocked, and the other sessions stay on the allowlist.
It closes by itself at the end of the time, when the session ends, or with jw internet close;
connections opened meanwhile end then too.

Without a subcommand: the sessions of this project and whether their internet is open.

```
jw internet
```

<a id="internet-close"></a>
#### `jw internet close`

Put a session back on the allowlist now. Its connections opened meanwhile end.

SESSION is a session id (jw internet lists them), a worktree name, or main for the main
checkout; leave it out when the project has one session.

```
jw internet close [SESSION]
```

<a id="internet-open"></a>
#### `jw internet open`

Let one session reach every host and port for --for (default 15m, at most 24h), or until
jw internet close with --for 0. Opening again sets a new end.

SESSION is a session id (jw internet lists them), a worktree name, or main for the main
checkout; leave it out when the project has one session.

```
jw internet open [SESSION] [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--for` | `15m0s` | how long, e.g. 30m or 2h; 0: until closed |

<a id="proxy"></a>
### `jw proxy`

Manage the egress proxies

<a id="proxy-list"></a>
#### `jw proxy list`

Print the project's full allowlist

```
jw proxy list
```

<a id="proxy-presets"></a>
#### `jw proxy presets`

List the allowlist presets by category (names for allow_presets); add or remove your own

```
jw proxy presets
```

<a id="proxy-presets-add"></a>
##### `jw proxy presets add`

Create a preset of your own: a named group of domains (.example.com includes subdomains), kept
in <config>/presets/<name>.yaml. Projects use it like any preset: jw allow --preset <name>.
Replacing one reloads the running proxies of the projects using it.

```
jw proxy presets add <name> <domain>... [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `-d`, `--description` |  | what the preset is for |

<a id="proxy-presets-rm"></a>
##### `jw proxy presets rm`

Remove a preset of your own (once no project uses it)

```
jw proxy presets rm <name>
```

<a id="proxy-reload"></a>
#### `jw proxy reload`

Make running proxies re-read their allowlists (after editing allowlist.txt by hand)

```
jw proxy reload
```

---

## Configuration

<a id="alias"></a>
### `jw alias`

Add or remove the short command jw

<a id="alias-add"></a>
#### `jw alias add`

Add a short command (default jw) next to the jailwick binary

```
jw alias add [name]
```

<a id="alias-remove"></a>
#### `jw alias remove`

Remove the short command

```
jw alias remove [name]
```

<a id="config"></a>
### `jw config`

Show or edit settings (project, or --global)

| Flag | Default | Meaning |
|---|---|---|

<a id="config-edit"></a>
#### `jw config edit`

Edit the project's file (or --global: config.yaml) in $EDITOR

```
jw config edit
```

<a id="config-path"></a>
#### `jw config path`

Print jailwick's configuration and state folders

```
jw config path
```

<a id="config-show"></a>
#### `jw config show`

Print the project's settings with defaults filled in (or --global)

```
jw config show
```

<a id="handoffs"></a>
### `jw handoffs`

List requests between the workspace's sessions (open first; --all includes done)

```
jw handoffs [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--all` |  | include done requests |

<a id="hidden"></a>
### `jw hidden`

List the paths hidden in the workspace members (as of now; applied at restart)

```
jw hidden
```

<a id="image"></a>
### `jw image`

Build, update or clean up sandbox images

<a id="image-build"></a>
#### `jw image build`

Rebuild the project's images. Steps that didn't change come from the build cache;
--fresh runs every step again and pulls the images they start from, so base packages pick up
security updates.

```
jw image build [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--fresh` |  | no build cache, and pull the images built from again |

<a id="image-check"></a>
#### `jw image check`

Check an image before a build fails on it: Debian or Ubuntu (apt), glibc, amd64 or arm64,
the sandbox user and its home, and for an image used as is (--as-is) Claude Code and git. The
image is pulled first. Without an image, the project's own is checked (its base, or its image
used as is; a Dockerfile is checked when it builds).

```
jw image check [image] [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--as-is` |  | the image is used as is: Claude Code and git must be in it |
| `--home` |  | its home (default /home/<user>) |
| `--platform` |  | linux/amd64 or linux/arm64 (default: this machine's) |
| `--user` |  | the sandbox user (default node) |

<a id="image-default"></a>
#### `jw image default`

Show or set the default base image (config.yaml's image): a Debian or Ubuntu image with the
sandbox user, which jailwick builds its layer on for every project that names no image of its own.
--reset goes back to jailwick's base. Projects pick it up at their next start or restart.

```
jw image default [image] [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--home` |  | its home (default /home/node) |
| `--reset` |  | go back to jailwick's base |
| `--user` |  | the sandbox user in the image (default node) |

<a id="image-ls"></a>
#### `jw image ls`

List images built by jailwick

```
jw image ls
```

<a id="image-prune"></a>
#### `jw image prune`

Remove the images jailwick built that no project uses any more (older settings, earlier
Claude Code versions) and no container is made from. Images in use, other images and volumes are
kept, and so is the runtime's build cache, which makes rebuilds fast: docker builder prune frees it.

```
jw image prune
```

<a id="image-update"></a>
#### `jw image update`

Rebuild the project layer when a newer Claude Code is out

```
jw image update
```

<a id="notes"></a>
### `jw notes`

Notes tell Claude how to run and test this project inside the sandbox. They are read at the
start of each session, so edits apply to the next session without a restart.

```
jw notes [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--workspace` |  | the workspace's notes |

<a id="path"></a>
### `jw path`

Print the project's folder (or a worktree's)

```
jw path [worktree]
```

<a id="secret"></a>
### `jw secret`

Secrets are stored in the macOS Keychain (or a 0600 file, or read from environment variables)
and passed to the sandbox as environment variables. Known ones: CLAUDE_CODE_OAUTH_TOKEN, ANTHROPIC_API_KEY, GH_TOKEN.
Others are passed when listed in the project's secret_env. --global sets a value for all projects.

| Flag | Default | Meaning |
|---|---|---|

<a id="secret-ls"></a>
#### `jw secret ls`

List secret names (never values) for the project and global

```
jw secret ls
```

<a id="secret-set"></a>
#### `jw secret set`

Set a secret (asks for the value when not given)

```
jw secret set NAME [VALUE]
```

<a id="secret-unset"></a>
#### `jw secret unset`

Remove a secret

```
jw secret unset NAME
```

<a id="settings"></a>
### `jw settings`

Claude settings are layers, each overriding the ones before it:

  app       one file for every project
  profiles  named files; a project uses the ones it lists in claude.profiles, in order
  project   one project's own file

Objects merge key by key, lists keep every distinct entry, other values: the later layer wins.
The result is passed to Claude with --settings, so it takes precedence over what /model and
/config save. Changes apply to the next session; no restart needed.

<a id="settings-edit"></a>
#### `jw settings edit`

Edit the project's Claude settings in $EDITOR (--app: every project's, --profile: a profile's)

```
jw settings edit [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--app` |  | the settings every project uses |
| `--profile` |  | a named profile (created if new) |

<a id="settings-ls"></a>
#### `jw settings ls`

List the Claude settings files: app-wide, profiles (with the projects using them) and projects'

```
jw settings ls
```

<a id="settings-show"></a>
#### `jw settings show`

Print the Claude settings the project's sessions get, and the layers they come from

```
jw settings show
```

<a id="where"></a>
### `jw where`

Show the project, worktree and workspace for the current folder

```
jw where
```

<a id="workspace"></a>
### `jw workspace`

A workspace is a folder of related repos (an API, a web app, shared docs). Each project in it
also sees the other members read-only, with secrets hidden, and its sessions can pass requests
to each other through a shared handoff folder.

<a id="workspace-add"></a>
#### `jw workspace add`

Create a workspace from a folder of repos, step by step:
  1. Members: the folders each project sees read-only (default: the git repos under the root).
  2. Projects: the projects under the root that join (default: those in no workspace).
  3. Sharing: the handoff folder (default .handoff), workspace instructions (a CLAUDE.md at the
     root), more hiding rules.
The root defaults to the current project's parent folder. Every setting also has a flag; --yes
takes the prefilled answers without asking.

```
jw workspace add <name> [root] [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--exclude` |  | more hiding rules, e.g. secrets/ (repeatable) |
| `--handoff` |  | handoff folder |
| `--member-path` |  | folders from elsewhere, e.g. ~/code/shared-docs (repeatable) |
| `--members` |  | members, e.g. api, web (repeatable) |
| `--projects` |  | projects in the workspace (repeatable) |
| `--root-notes` |  | workspace instructions |
| `-y`, `--yes` |  | take the prefilled settings without asking |

<a id="workspace-edit"></a>
#### `jw workspace edit`

Edit a workspace's file in $EDITOR

```
jw workspace edit [name]
```

<a id="workspace-ls"></a>
#### `jw workspace ls`

List workspaces

```
jw workspace ls
```

<a id="workspace-rename"></a>
#### `jw workspace rename`

Rename a workspace (its projects and notes move along)

```
jw workspace rename <name> <new-name>
```

<a id="workspace-rm"></a>
#### `jw workspace rm`

Remove a workspace (its projects leave it; repos and the handoff folder are kept)

```
jw workspace rm <name>
```

<a id="workspace-show"></a>
#### `jw workspace show`

Print a workspace's settings

```
jw workspace show [name]
```

---

## Other

<a id="completion"></a>
### `jw completion`

Generate the autocompletion script for jw for the specified shell.
See each sub-command's help for details on how to use the generated script.

<a id="completion-bash"></a>
#### `jw completion bash`

Generate the autocompletion script for the bash shell.

This script depends on the 'bash-completion' package.
If it is not installed already, you can install it via your OS's package manager.

To load completions in your current shell session:

	source <(jw completion bash)

To load completions for every new session, execute once:

#### Linux:

	jw completion bash > /etc/bash_completion.d/jw

#### macOS:

	jw completion bash > $(brew --prefix)/etc/bash_completion.d/jw

You will need to start a new shell for this setup to take effect.

```
jw completion bash
```

| Flag | Default | Meaning |
|---|---|---|
| `--no-descriptions` |  | disable completion descriptions |

<a id="completion-fish"></a>
#### `jw completion fish`

Generate the autocompletion script for the fish shell.

To load completions in your current shell session:

	jw completion fish | source

To load completions for every new session, execute once:

	jw completion fish > ~/.config/fish/completions/jw.fish

You will need to start a new shell for this setup to take effect.

```
jw completion fish [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--no-descriptions` |  | disable completion descriptions |

<a id="completion-powershell"></a>
#### `jw completion powershell`

Generate the autocompletion script for powershell.

To load completions in your current shell session:

	jw completion powershell | Out-String | Invoke-Expression

To load completions for every new session, add the output of the above command
to your powershell profile.

```
jw completion powershell [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--no-descriptions` |  | disable completion descriptions |

<a id="completion-zsh"></a>
#### `jw completion zsh`

Generate the autocompletion script for the zsh shell.

If shell completion is not already enabled in your environment you will need
to enable it.  You can execute the following once:

	echo "autoload -U compinit; compinit" >> ~/.zshrc

To load completions in your current shell session:

	source <(jw completion zsh)

To load completions for every new session, execute once:

#### Linux:

	jw completion zsh > "${fpath[1]}/_jw"

#### macOS:

	jw completion zsh > $(brew --prefix)/share/zsh/site-functions/_jw

You will need to start a new shell for this setup to take effect.

```
jw completion zsh [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--no-descriptions` |  | disable completion descriptions |

<a id="update"></a>
### `jw update`

Ask GitHub for the latest release. When it is newer and jailwick came from Homebrew, update
the CLI and the desktop app (those Homebrew has) with brew upgrade; otherwise say where to
download it. Nothing restarts: reopen the app, and running sandboxes take the new helper at
their next restart (jw restart). jailwick also checks once a day by itself (jw version,
jw doctor, the app); updates: off in config.yaml turns that off.

```
jw update [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--check` |  | only check; don't update |
| `--yes` |  | update without asking |

<a id="version"></a>
### `jw version`

Print the version

```
jw version
```

