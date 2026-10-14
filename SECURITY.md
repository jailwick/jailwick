# Security

jailwick runs AI coding agents in a locked-down container per project. This page says what it
protects, what it doesn't, and how to report a problem.

## Reporting a vulnerability

Please report privately, through **Report a vulnerability** on this repository's GitHub
**Security** tab, not in a public issue. Include the jailwick version (`jw version`), your
runtime (`jw doctor`), and the steps or a proof of concept. You'll get an answer within a week,
and a fix or a plan before anything is made public.

Security fixes go to the latest release only.

## What jailwick protects

From the agent and everything it runs (commands, packages it installs, tools), jailwick keeps
away:

- **Your files outside the repo.** Only the repo is mounted, plus, in a workspace, its other repos
  read-only with their secrets (`.env`, keys) hidden.
- **Your host and your network.** No route to the host, your LAN or cloud metadata addresses.
- **The internet.** Only allowlisted sites, through jailwick's proxy (`jw allow`, the Network page).
- **Other projects' sandboxes and services.**
- **The container runtime.** No Docker socket, no root, no Linux capabilities.
- **Your SSH keys and git credentials** on the host.

Some protections can't be configured away: no setting mounts the Docker socket or a broad path, or
runs the sandbox as root or with capabilities.

**`jw verify`** proves these boundaries in a running sandbox, and the app's Verify tab shows the
same checks:

- the identity and mounts;
- no way out except the proxy: no outside name resolution, no public DNS servers, no IPv6;
- nothing on the host or the LAN, and no other sandbox's services;
- that the allowed sites work.

## What it doesn't protect

| Not protected | Why |
|---|---|
| **Files from the repo that your Mac executes** | The repo is mounted read-write, `.git/` included, so git works normally in the sandbox. A session could change git hooks or settings, or editor and agent settings, that later run on your Mac with your rights. jailwick doesn't restrict git: [docs/HOST-EXECUTION.md](docs/HOST-EXECUTION.md) explains how this works, what triggers it, and how to check and protect yourself. |
| **Data sent to allowlisted sites** | Any allowed site (GitHub, npm, anything you add) can receive what the sandbox can read, the repo included. Allow as little as the project needs, preferably for that project only. |
| **Credentials given to the sandbox** | The Claude login, `GH_TOKEN`, database passwords and other secrets are readable by any process in it. Prefer fine-grained tokens limited to the repo. |
| **A session whose internet is open** | While open, it can reach any host on any port. Other sessions stay on the allowlist, but they share the repo, the services and the caches. |
| **Other sessions of the same project** | A project's sessions can message each other, and share one process space: one can see, signal or stop another's processes. They already share the repo, the secrets and the user, so this adds little; a session that reads untrusted content can still pass it on as a message. Claude Code asks before delivering a message to a session in a different permission mode. A private session (`--private`) is kept out. Sessions of different projects never share either. |
| **An image you paste** | Pressing Ctrl+V in a session (or pasting when the clipboard holds no text) copies the clipboard's image, and only an image, into that session for 30 seconds, so Claude can read it. The sandbox can't ask for the clipboard itself: it's read only on your key press, and no text ever comes from it. `JAILWICK_PASTE=off` turns this off. |
| **Everything inside the repo** | A session can change any file in it. Review changes as you would anyone's. |
| **Packages installed in the sandbox** | A malicious package runs with the sandbox's access (see the rows above), not with yours. |
| **The runtime's kernel** | Containers share the kernel of the runtime's VM. A kernel exploit is out of jailwick's reach. |

YOLO mode (`claude.yolo`, Claude without permission prompts) is off by default. It is reasonable
only because of the sandbox, and only within the limits above.

## How jailwick treats data from the sandbox

The CLI and the app read some data the sandbox writes: handoff requests, session names, the
agent's answers, and the hosts the proxy refused. They read it as untrusted input:

- links are never followed;
- reads are bounded;
- control characters are removed before a terminal shows any of it.

[docs/UNTRUSTED-DATA.md](docs/UNTRUSTED-DATA.md) lists every reader and its rules.

## Secrets

- Secrets are kept in your system's keychain, in a file only you can read, or in your
  environment, never in project files.
- They reach a sandbox only when it starts.
- They never appear in logs, `--json` output, `config show` or the app.
- To tell whether a secret changed, jailwick keeps only a short digest of it.

## No telemetry

jailwick sends nothing about you or your use. Telemetry of the tools inside the sandbox is turned
off by default.
