# jailwick's layer for one project, generated from its image settings: what it adds to its base
# (jailwick's or the project's own). Steps that change least come first, so a change rebuilds only
# what follows it; downloads are checked and kept in BuildKit cache mounts between builds.
ARG BASE
FROM ${BASE}

USER root
ARG SANDBOX_USER=node
ARG SANDBOX_HOME=/home/node
ENV DEBIAN_FRONTEND=noninteractive

# Prerequisites: on any Debian or Ubuntu base, the tools the steps below use and Node.js for
# installing Claude Code. On jailwick's base, nothing to do.
RUN --mount=type=bind,source=lib.sh,target=/tmp/jw/lib.sh \
    --mount=type=bind,source=prereqs.sh,target=/tmp/jw/prereqs.sh \
    --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt/lists,sharing=locked \
    --mount=type=cache,target=/var/cache/jailwick,sharing=locked \
    --mount=type=cache,target=/root/.npm,sharing=locked \
    NODE_DEFAULT=24 sh /tmp/jw/prereqs.sh

# Playwright: chromium firefox. Right after the prerequisites, before the stacks, so projects with the
# same browsers and Playwright versions share this step whatever their stacks, and a stack change
# doesn't redo it. It runs with the prerequisites' Node.js, not a stack's.
RUN --mount=type=bind,source=lib.sh,target=/tmp/jw/lib.sh \
    --mount=type=bind,source=playwright.sh,target=/tmp/jw/playwright.sh \
    --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt/lists,sharing=locked \
    --mount=type=cache,target=/var/cache/jailwick,sharing=locked \
    --mount=type=cache,target=/root/.npm,sharing=locked \
    BROWSERS="chromium firefox" VERSIONS="1.50.1" sh /tmp/jw/playwright.sh
ENV PLAYWRIGHT_BROWSERS_PATH=/ms-playwright

# Android (forces linux/amd64).
RUN --mount=type=bind,source=lib.sh,target=/tmp/jw/lib.sh \
    --mount=type=bind,source=android.sh,target=/tmp/jw/android.sh \
    --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt/lists,sharing=locked \
    --mount=type=cache,target=/var/cache/jailwick,sharing=locked \
    --mount=type=cache,target=/root/.npm,sharing=locked \
    sh /tmp/jw/android.sh
ENV ANDROID_HOME=/opt/android-sdk \
    ANDROID_SDK_ROOT=/opt/android-sdk \
    PATH=$PATH:/opt/android-sdk/cmdline-tools/latest/bin:/opt/android-sdk/platform-tools

# Node.js 22, corepack (pnpm, yarn), tsx and TypeScript.
RUN --mount=type=bind,source=lib.sh,target=/tmp/jw/lib.sh \
    --mount=type=bind,source=stacks/node.sh,target=/tmp/jw/node.sh \
    --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt/lists,sharing=locked \
    --mount=type=cache,target=/var/cache/jailwick,sharing=locked \
    --mount=type=cache,target=/root/.npm,sharing=locked \
    VERSION=22 sh /tmp/jw/node.sh
ENV PATH=/opt/node/bin:$PATH \
    COREPACK_ENABLE_DOWNLOAD_PROMPT=0

# Python 3.14 and uv. uv copies from its cache: the cache (~/.cache) and the repo's virtual
# environments are on different volumes, where links don't work.
RUN --mount=type=bind,source=lib.sh,target=/tmp/jw/lib.sh \
    --mount=type=bind,source=stacks/python.sh,target=/tmp/jw/python.sh \
    --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt/lists,sharing=locked \
    --mount=type=cache,target=/var/cache/jailwick,sharing=locked \
    --mount=type=cache,target=/root/.npm,sharing=locked \
    VERSION=3.14 sh /tmp/jw/python.sh
ENV UV_PYTHON_INSTALL_DIR=/opt/uv/python \
    UV_LINK_MODE=copy

# Go 1.25. Modules and the build cache persist in ~/.cache, go install puts binaries in
# ~/.local/bin.
RUN --mount=type=bind,source=lib.sh,target=/tmp/jw/lib.sh \
    --mount=type=bind,source=stacks/go.sh,target=/tmp/jw/go.sh \
    --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt/lists,sharing=locked \
    --mount=type=cache,target=/var/cache/jailwick,sharing=locked \
    --mount=type=cache,target=/root/.npm,sharing=locked \
    VERSION=1.25 sh /tmp/jw/go.sh
ENV PATH=/usr/local/go/bin:$PATH \
    GOPATH=${SANDBOX_HOME}/.local/share/go \
    GOMODCACHE=${SANDBOX_HOME}/.cache/go/mod \
    GOBIN=${SANDBOX_HOME}/.local/bin

# Rust stable. Cargo's registry persists in ~/.cache, cargo install puts binaries in
# ~/.local/bin.
RUN --mount=type=bind,source=lib.sh,target=/tmp/jw/lib.sh \
    --mount=type=bind,source=stacks/rust.sh,target=/tmp/jw/rust.sh \
    --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt/lists,sharing=locked \
    --mount=type=cache,target=/var/cache/jailwick,sharing=locked \
    --mount=type=cache,target=/root/.npm,sharing=locked \
    VERSION=stable sh /tmp/jw/rust.sh
ENV PATH=/usr/local/cargo/bin:$PATH \
    RUSTUP_HOME=/usr/local/rustup \
    CARGO_HOME=${SANDBOX_HOME}/.cache/cargo \
    CARGO_INSTALL_ROOT=${SANDBOX_HOME}/.local

# Bun 1. Its package cache persists in ~/.cache, bun add -g puts binaries in ~/.local/bin.
RUN --mount=type=bind,source=lib.sh,target=/tmp/jw/lib.sh \
    --mount=type=bind,source=stacks/bun.sh,target=/tmp/jw/bun.sh \
    --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt/lists,sharing=locked \
    --mount=type=cache,target=/var/cache/jailwick,sharing=locked \
    --mount=type=cache,target=/root/.npm,sharing=locked \
    VERSION=1 sh /tmp/jw/bun.sh
ENV BUN_INSTALL_CACHE_DIR=${SANDBOX_HOME}/.cache/bun \
    BUN_INSTALL_BIN=${SANDBOX_HOME}/.local/bin \
    BUN_INSTALL_GLOBAL_DIR=${SANDBOX_HOME}/.local/share/bun/global

# The project's apt packages.
RUN --mount=type=bind,source=lib.sh,target=/tmp/jw/lib.sh \
    --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt/lists,sharing=locked \
    --mount=type=cache,target=/var/cache/jailwick,sharing=locked \
    --mount=type=cache,target=/root/.npm,sharing=locked \
    apt-get update -q && apt-get install -y --no-install-recommends libvips-dev ffmpeg

# The sandbox user's home.
RUN --mount=type=bind,source=lib.sh,target=/tmp/jw/lib.sh \
    --mount=type=bind,source=home.sh,target=/tmp/jw/home.sh \
    --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt/lists,sharing=locked \
    --mount=type=cache,target=/var/cache/jailwick,sharing=locked \
    --mount=type=cache,target=/root/.npm,sharing=locked \
    sh /tmp/jw/home.sh

# Claude Code, with jailwick's Node.js whatever the project's. jailwick resolves "latest" to a
# version, so this layer is rebuilt only for a new release. It goes to /usr/local: the npm prefix
# ~/.local is a volume at runtime and would keep serving the first version installed.
ARG CLAUDE_CODE_VERSION=latest
RUN --mount=type=bind,source=lib.sh,target=/tmp/jw/lib.sh \
    --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt/lists,sharing=locked \
    --mount=type=cache,target=/var/cache/jailwick,sharing=locked \
    --mount=type=cache,target=/root/.npm,sharing=locked \
    PATH=/usr/local/bin:$PATH npm install -g --prefix /usr/local "@anthropic-ai/claude-code@${CLAUDE_CODE_VERSION}"

# At runtime the user's own installs (npm -g, uv tool, go install, cargo install, bun add -g) go to
# ~/.local, a volume that keeps them.
ENV NPM_CONFIG_PREFIX=${SANDBOX_HOME}/.local \
    PATH=${SANDBOX_HOME}/.local/bin:$PATH
USER ${SANDBOX_USER}
WORKDIR ${SANDBOX_HOME}
