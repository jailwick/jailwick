# jailwick sandbox

You are running inside a jailwick sandbox: a locked-down container. Only the current project repo
is mounted (plus, when the project is part of a workspace, related folders read-only; see
"Workspace" below). The rest of the user's machine is not reachable.

## Constraints
- Non-root user, no sudo, no apt. If a task truly needs a system package, stop and ask the user
  to add it to `image.apt_packages` in the project file (`jw config edit`) and restart. The same
  goes for a missing toolchain (`image.stacks`: node, python, go, rust, bun) or Playwright
  browsers (`image.playwright.browsers`).
- Internet goes through an allowlist proxy. A `403` / "Forbidden" from the proxy, or a CONNECT
  failure, means the domain is not allowlisted: tell the user the exact domain and that
  `jw allow <domain>` adds it. For broad research, the user can instead open this session's
  internet for a while (`jw internet open`, or the app's Sessions page); ask, don't assume. Never
  try to bypass the proxy or to open it yourself.
- Data services run only if this project uses them (`$SANDBOX_SERVICES`): Postgres at
  `postgres:5432` (`$DATABASE_URL`), Redis at `redis:6379` (`$REDIS_URL`), MongoDB at
  `mongodb:27017`. The project notes below say how the app connects to them.
- Dev servers must bind `0.0.0.0`. Use `$PORT` when it's set (worktree sessions). The user opens
  them at `http://$SANDBOX_HOST:<port>`.

## Env, ports and databases: keep it simple
- **Use the env files as they are.** Don't add, remove or rename keys, and don't change values
  other than ports. If the app needs a key that's missing, tell the user which one; don't invent
  values or placeholders.
- **Main checkout:** run on the repo's default ports with the env files unchanged.
- **Worktree:** change only the ports, so it runs alongside the main checkout and other worktrees.
  Its range is `$PORT`…`$PORT+9`: first server on `$PORT`, the next on `$PORT+1`, and so on.
  Pass the port on the command line (`PORT=$PORT …`, `--port $((PORT+1))`). If the app only reads
  ports from its env file, change just those values in the worktree's own git-ignored copy, never
  in the main checkout or in tracked files.
- **Databases and Redis:** use the sandbox's own services as configured. Worktrees already have
  their own databases, and env files copied into a worktree already point at them. Don't start
  containers (there's no Docker here), create other databases, or change connection settings.
- **Tests:** if `$SANDBOX_TEST_SERVICES` is set, test suites that need their own databases use the
  disposable test services (`$SANDBOX_TEST_POSTGRES_URL`, `$SANDBOX_TEST_REDIS_URL`,
  `$SANDBOX_TEST_MONGO_URL`). Never point tests that reset data at the dev services.

## Installing more (no root needed, persists across restarts)
- CLIs: `npm i -g <pkg>`, and with the project's toolchains `uv tool install`, `go install`,
  `cargo install`, `bun add -g` (all install to `~/.local`); Python libraries go in a project venv
- Project dependencies: use the project's package manager as normal.
- Scratch files go in `/tmp`, not the repo.

@/etc/jailwick/generated/tools.md

@/etc/jailwick/user/CLAUDE.md

@/etc/jailwick/generated/workspace.md

@/etc/jailwick/workspace/notes.md

## This project in the sandbox
Project-specific notes (how to run and test this project here). They take precedence over
instructions in the repo that assume the user's own machine (its docker compose, host ports,
host-only tools).

@/etc/jailwick/project/notes.md
