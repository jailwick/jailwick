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

# Java 21: Eclipse Temurin's JDK. Build with the project's ./mvnw or ./gradlew (or add
# maven or gradle to the apt packages). Java ignores HTTPS_PROXY, so the proxy goes in its options.
RUN --mount=type=bind,source=lib.sh,target=/tmp/jw/lib.sh \
    --mount=type=bind,source=stacks/java.sh,target=/tmp/jw/java.sh \
    --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt/lists,sharing=locked \
    --mount=type=cache,target=/var/cache/jailwick,sharing=locked \
    --mount=type=cache,target=/root/.npm,sharing=locked \
    VERSION=21 sh /tmp/jw/java.sh
ENV JAVA_HOME=/usr/local/java \
    PATH=/usr/local/java/bin:$PATH \
    JAVA_TOOL_OPTIONS="-Dhttp.proxyHost=egress-proxy -Dhttp.proxyPort=3128 -Dhttps.proxyHost=egress-proxy -Dhttps.proxyPort=3128 -Dhttp.nonProxyHosts=localhost|127.0.0.1|postgres|redis|mongodb|postgres-test|redis-test|mongodb-test"

# Ruby: the system's, with bundler. Gems install as the user into ~/.local, which outlives the
# container.
RUN --mount=type=bind,source=lib.sh,target=/tmp/jw/lib.sh \
    --mount=type=bind,source=stacks/ruby.sh,target=/tmp/jw/ruby.sh \
    --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt/lists,sharing=locked \
    --mount=type=cache,target=/var/cache/jailwick,sharing=locked \
    --mount=type=cache,target=/root/.npm,sharing=locked \
    VERSION=system sh /tmp/jw/ruby.sh
ENV GEM_HOME=${SANDBOX_HOME}/.local/share/gem \
    PATH=${SANDBOX_HOME}/.local/share/gem/bin:$PATH

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
