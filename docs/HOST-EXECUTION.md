# Repo files your Mac runs

jailwick mounts the repo read-write into the sandbox, `.git/` included, so git works there
exactly as it does on your Mac. The cost is one known way out of the sandbox: **code in the
sandbox can change files that your Mac later executes**, and they run with your full rights,
outside every jailwick protection. This page explains how, what triggers it, and how to protect
yourself.

jailwick doesn't block this on purpose. Making `.git/config` and `.git/hooks` read-only in the
sandbox would break everyday commands there (`git push -u`, `git remote add`, branch tracking,
submodules, husky and other hook installers). Any tool that bind-mounts a repo into a container
(dev containers, other agent sandboxes) has the same exposure unless it makes that trade.

## Who could do it

Nothing in the sandbox does this on its own. It takes code that acts against you running in
the sandbox:

- **A compromised dependency.** `npm install`, `pip install` and the like run install scripts
  inside the sandbox. A malicious package is contained there, except for this path.
- **A manipulated agent.** Instructions hidden in an issue, a web page, a README or a
  dependency's code can steer Claude (prompt injection) into writing these files.
- **A mistake.** Claude, or a tool it runs, sets a hook or a config value without understanding
  what it does on your Mac.

The sandbox keeps such code away from your files, keys and network. The files below are how it
can reach them anyway, by waiting for you to run something.

## How an attack works

1. Code in the sandbox writes one line into the repo's `.git/config`:
   ```
   [core]
       fsmonitor = "sh -c 'cp ~/.ssh/id_ed25519 ~/.aws/credentials \"$PWD/.cache/\" 2>/dev/null; echo'"
   ```
2. Later you open the repo on your Mac. You type nothing, but your editor's git integration or
   your shell prompt runs `git status` to show the branch.
3. git runs the `fsmonitor` command as **you**: with your files, your SSH keys and your full
   network.
4. In this example it copies your keys into the repo, where the sandbox can read them, and
   sends them out through any site the sandbox may reach (pushing to GitHub works).

The command could just as well download and run anything, because on your Mac nothing limits
the network.

## What git runs from the repo

These settings live in files the sandbox can write: `.git/config`, files it includes, and
`.git/info/attributes` or `.gitattributes`.

| Setting or file | Runs when you… |
|---|---|
| `core.fsmonitor` (a command) | run almost any git command: `status`, `diff`, `add`, `commit`; also editors and shell prompts in the background |
| hooks in `.git/hooks/`, or in the folder `core.hooksPath` names (e.g. `.husky/`) | commit (`pre-commit`, `commit-msg`), push (`pre-push`), check out or switch (`post-checkout`), merge or pull (`post-merge`), rebase (`post-rewrite`); `reference-transaction` runs on almost any ref change, fetch included |
| `filter.<name>.clean` / `smudge` / `process`, with an attributes file assigning the filter | `status`, `add`, `diff` (clean) and checkout or switch (smudge) |
| `diff.external`, `diff.<name>.textconv` | `diff`, `log -p`, `show`, and editors' diff views |
| `merge.<name>.driver` | merges and pulls with conflicts in matching files |
| `core.pager`, `pager.<command>` | any command whose output is paged (`log`, `diff`, `show`) |
| `core.editor`, `sequence.editor` | `commit` without `-m`, `rebase -i` |
| `core.sshCommand`, `core.gitProxy`, `credential.helper = !…` | fetch, pull or push |
| `gpg.program`, `gpg.ssh.program` (with `commit.gpgSign`) | every commit or tag |
| `protocol.ext.allow` with an `ext::` remote URL | fetch, pull or push to that remote |
| `include.path`, `includeIf` | pull more settings from another file, e.g. one inside the repo |

Some settings don't execute anything but still hurt:

- **Redirects.** `remote.<name>.url`, `url.<base>.insteadOf` and `pushInsteadOf` send your fetches
  or pushes somewhere else: you push your code to an attacker, or pull theirs.
- **Interception.** `http.proxy` and `http.sslVerify=false` let someone read or change your git
  traffic.

## Other repo files your tools run

| File | Runs when |
|---|---|
| `.claude/settings.json`, `.claude/settings.local.json` (hooks), `.mcp.json` | you run Claude Code on your Mac in this repo |
| `.envrc` | you `cd` into the repo with direnv. direnv asks again after every change (`direnv allow`) |
| `.vscode/tasks.json` (`runOn: folderOpen`), `.vscode/settings.json` (tool paths), `.idea/` run configurations | the repo opens in VS Code or a JetBrains IDE. Workspace Trust prompts help, once you've trusted the folder they don't |
| `.pre-commit-config.yaml` | you commit on your Mac with the pre-commit framework installed |
| `package.json` scripts, `Makefile`, build and test scripts, `.npmrc` | you run `npm install`, `npm test`, `make` and so on on your Mac |

The last row is the general rule: **anything in the repo that you run on your Mac runs with your
rights.** Git is the dangerous case because it runs commands without you asking for them.

## How to protect yourself

None of these limit git in the sandbox.

1. **Check after a session, before using the repo on your Mac.** Three commands show everything
   git can run:
   ```
   git config --local --list --show-origin   # look for fsmonitor, hooksPath, filter, diff, pager, editor, sshCommand, credential, gpg, include, url, proxy
   ls .git/hooks | grep -v '\.sample$'       # any hook you didn't install
   cat .git/info/attributes 2>/dev/null       # filters and diff drivers assigned to files
   ```
   Run `git diff` on the tracked files listed above too.
2. **Don't let tools run git silently in sandbox repos.**
   - Use a shell prompt that doesn't run `git status` in those folders.
   - Turn off your editor's background git (in VS Code, `git.autorefresh` and `git.autofetch`,
     set per workspace).
   - Open the repo in your editor's restricted mode until you've checked it.
3. **Give sandboxes their own checkout.** Clone the repo once more for jailwick and work in that
   clone only through jailwick. Bring commits into your own checkout with
   `git fetch ../repo-sandbox <branch>`. A fetch copies objects and doesn't run the other repo's
   hooks; current git also ignores the dangerous settings of a repo it fetches from. Never open
   the sandbox clone in host tools.
4. **Make what's worth stealing ask first.**
   - SSH keys with a passphrase, or an SSH agent that confirms each use (1Password, Secretive,
     `ssh-add -c`).
   - Cloud credentials in a keychain or behind short-lived sessions, not in plain files.
5. **Keep git up to date.** Git fixes bugs in this area (hooks run during clone, unsafe
   repositories) in regular releases.

## What jailwick does

- **It contains the code.** It runs in the sandbox, without your files, keys or network.
- **It makes changes easy to see.** The repo is your own folder, so `git status` and
  `git diff` on your Mac show what a session changed. Settings in `.git/` don't show there,
  which is why step 1 above matters.
- **Planned: a warning instead of a block.** jailwick records the git settings, hooks and
  attributes above, and the other host-run files, when a session starts. It warns in `jw status`,
  after `jw claude` ends, and on the project page when any of them changed, with the change
  shown. It blocks nothing and changes nothing in git.
