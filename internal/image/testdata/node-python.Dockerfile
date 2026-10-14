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

# Node.js 24, corepack (pnpm, yarn), tsx and TypeScript.
RUN --mount=type=bind,source=lib.sh,target=/tmp/jw/lib.sh \
    --mount=type=bind,source=stacks/node.sh,target=/tmp/jw/node.sh \
    --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt/lists,sharing=locked \
    --mount=type=cache,target=/var/cache/jailwick,sharing=locked \
    --mount=type=cache,target=/root/.npm,sharing=locked \
    VERSION=24 sh /tmp/jw/node.sh
ENV PATH=/opt/node/bin:$PATH \
    COREPACK_ENABLE_DOWNLOAD_PROMPT=0

# Python 3.12 and uv. uv copies from its cache: the cache (~/.cache) and the repo's virtual
# environments are on different volumes, where links don't work.
RUN --mount=type=bind,source=lib.sh,target=/tmp/jw/lib.sh \
    --mount=type=bind,source=stacks/python.sh,target=/tmp/jw/python.sh \
    --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt/lists,sharing=locked \
    --mount=type=cache,target=/var/cache/jailwick,sharing=locked \
    --mount=type=cache,target=/root/.npm,sharing=locked \
    VERSION=3.12 sh /tmp/jw/python.sh
ENV UV_PYTHON_INSTALL_DIR=/opt/uv/python \
    UV_LINK_MODE=copy

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
