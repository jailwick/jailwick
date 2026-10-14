# Sandbox images

Every project's sandbox runs an image jailwick builds in two layers:

1. **A base.** jailwick's own (Debian, Node.js, git, gh and the usual CLI tools), or one of yours.
2. **A project layer on top.** The stacks at the versions the repo names, Playwright browsers when
   chosen, apt packages, Claude Code, and the sandbox user's home.

Most projects need nothing else: pick stacks in the project's **Settings → Toolset**:

| Stack | Versions | Notes |
|---|---|---|
| node | 26, 24 (default), 22, 20 | npm, pnpm and yarn (corepack), tsx, TypeScript |
| python | 3.14 (default) … 3.11 | with uv |
| go | 1.27 (default) … | |
| rust | stable, beta, nightly, 1.x | rustup, cargo, rustfmt, clippy |
| bun | 1 | |
| java | 25, 21 (default), 17 | Eclipse Temurin's JDK; build with the project's `./mvnw` or `./gradlew`. Maven and Gradle know the sandbox's proxy, and their downloads are kept between sessions. |
| ruby | the system's | Debian's Ruby with bundler (3.1 on jailwick's base); gems go to `~/.local/share/gem`. Another version: a base image (below). |

The wizard ticks the stacks the repo uses (`.nvmrc`, `.python-version`, `go.mod`, `pom.xml`,
`build.gradle`, `.java-version`, `Gemfile`, `.tool-versions`…). When the language you need has no
stack, or your team already has an image, use your own base.

When the repo uses a language no stack installs, the Image step says which and how to get it: the
Debian packages to add on the default base where Debian has them (PHP: `php-cli composer`), or a
base image that has it (Elixir, Erlang, .NET, Swift, Dart, Haskell). Choosing **Your base image**
starts from that suggestion. Clojure and Scala tick the `java` stack.

## Where the image comes from

Set it in the new-project wizard, in a project's **Settings → Image**, or with flags.

| Choice | What you give | What jailwick adds | Flags (`jw project add`) |
|---|---|---|---|
| **The default** | nothing | everything | |
| **Your base image** | an image from a registry, like `eclipse-temurin:21-jdk` | its tools, the stacks, apt packages, Playwright, Claude Code, the home | `--image base --base <image>` |
| **Your Dockerfile** | a Dockerfile in the repo, like `docker/dev.Dockerfile` | the same, on top of what it builds | `--image dockerfile --dockerfile <path>` |
| **An image as is** (advanced) | an image your team builds and keeps up | nothing | `--image image --image-name <image>` |

"The default" is jailwick's base, unless you set a default image for every project (see below).

In the project file (`jw project edit`) these are `image.base`, `image.dockerfile` (with an
optional `image.context`, the build folder; by default the Dockerfile's) and `image.name`, only one
of them. A relative Dockerfile path is the repo's.

## What a base image must have

- **Debian or Ubuntu**, with `apt`. jailwick installs its tools with apt. Alpine (musl), Fedora,
  distroless and scratch images don't work: pick the image's `-bookworm`, `-jammy` or `-noble` tag.
- **glibc.** Node.js and the stacks are glibc builds.
- **amd64 or arm64.** By default, this machine's (`image.platform` picks one).
- **The sandbox user.** jailwick doesn't create users. It runs sessions as `node` (home
  `/home/node`) unless you set **Sandbox user** and **Home folder** (`image.user`, `image.home`).
  Use a user the image has (Ubuntu 24.04 images have `ubuntu`, home `/home/ubuntu`), or add one in
  your Dockerfile:

  ```dockerfile
  FROM ruby:3.3-bookworm
  RUN useradd --create-home app
  ```

  `jw image check` tells you whether the user is there.

An **image used as is** must also have Claude Code (`claude` on the PATH), git and the user's home.
Stacks, apt packages and Playwright are ignored for it.

## Check an image before building

**Check image**, next to the image in the wizard and in Settings, or `jw image check`, pulls the
image and tries it. It looks at the system (Debian or Ubuntu), the C library, the architecture,
the user and the home; for an image used as is, also Claude Code and git.

```
jw image check                               # this project's image
jw image check eclipse-temurin:21-jdk --user app --home /home/app
jw image check ghcr.io/acme/dev-sandbox:2 --as-is
```

When it can't try the image, it says why: the image or tag doesn't exist, the registry wants a login
(`docker login`), there's no build for this machine's architecture, or the image has no shell. A
build on a base without the sandbox user also stops with that reason.

Stack versions are checked as you type them: a version that doesn't exist yet is refused ("node:
34 doesn't exist yet (the newest is 26)").

## A default image for every project

**Settings → Default image** in the app, `jw image default`, or `image:` in `config.yaml` (`jw
config edit --global`) sets the base every project builds on unless it names its own:

```
jw image default eclipse-temurin:21-jdk --user app --home /home/app
jw image default            # show it
jw image default --reset    # back to jailwick's base
```

Projects pick up a change at their next start or restart.

## Examples

**Java with Maven or Gradle.** The `java` stack is enough for most projects. For a JDK it doesn't
offer, **Your base image** `eclipse-temurin:21-jdk` (Ubuntu): if `jw image check
eclipse-temurin:21-jdk --user ubuntu --home /home/ubuntu` passes, set that user and home; otherwise
add a user in a Dockerfile (`FROM eclipse-temurin:21-jdk`, then `RUN useradd --create-home app`)
and choose **Your Dockerfile** with the sandbox user `app`. Tick the `maven` allowlist preset
(Settings → Network access).

**Ruby at a version of your choice.** The `ruby` stack is the system's Ruby. For another version,
`ruby:3.3-bookworm` (it has no user: the Dockerfile above, `FROM ruby:3.3-bookworm`, with **Your
Dockerfile** and the sandbox user `app`). Tick `rubygems`.

**PHP.** `php:8.3-cli-bookworm`, a user added the same way, and the `packagist` preset.

**Your own image, as is.** For a team that ships one image with everything in it, Claude Code
included: **An image as is**, then `jw image check <image> --as-is`.

## Rebuilding

- Changing the image or the toolset rebuilds the project layer at the next start or restart.
- **Rebuild image** (the project's Overview, Project card) or `jw image build` rebuilds now; **Rebuild
  from scratch** or `--fresh` without the cache, pulling the base again. The building banner names
  the base it builds on.
- The Images page lists the base images projects pull too, marked "pulled".
- A Dockerfile is rebuilt when it changes. Changes to files it copies need `jw image build`.
- `jw image prune` removes the images jailwick built that no project uses.
