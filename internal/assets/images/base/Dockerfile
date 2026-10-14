# jailwick base image: Debian with Node.js (which Claude Code installs with), git, gh and the CLI
# tools every project gets. It is shared by every project; what a project adds (its stacks,
# Playwright, Android, apt packages, Claude Code) is jailwick's layer on top (../project).
# Built with root and full network; at runtime the container runs as the non-root user and
# reaches the internet only through the egress proxy.
FROM node:24.21.0-bookworm@sha256:3d27e5c11e5786e309ec3e03f93ae536eb36e6e5eb3714d5eb3300a36157add0

ARG GH_VERSION=2.102.0
ARG AST_GREP_VERSION=0.50.0

ENV DEBIAN_FRONTEND=noninteractive

# apt keeps its downloads for the build's cache mounts, which never end up in the image.
RUN rm -f /etc/apt/apt.conf.d/docker-clean \
 && echo 'Binary::apt::APT::Keep-Downloaded-Packages "true";' > /etc/apt/apt.conf.d/keep-downloads

# python3 and build-essential stay for native modules (node-gyp) and linkers (Rust, cgo).
RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt/lists,sharing=locked \
    apt-get update && apt-get install -y --no-install-recommends \
      git less vim nano tmux curl wget ca-certificates unzip zip rsync xz-utils \
      procps htop lsof iputils-ping dnsutils openssh-client \
      ripgrep fd-find fzf bat tree universal-ctags \
      jq yq sqlite3 xmlstarlet miller parallel moreutils bc file entr socat \
      imagemagick poppler-utils \
      shellcheck hyperfine httpie \
      build-essential python3 \
      postgresql-client redis-tools \
 && ln -s /usr/bin/fdfind /usr/local/bin/fd \
 && ln -s /usr/bin/batcat /usr/local/bin/bat \
 && curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg \
      -o /usr/share/keyrings/githubcli-archive-keyring.gpg \
 && echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" \
      > /etc/apt/sources.list.d/github-cli.list \
 && apt-get update && apt-get install -y --no-install-recommends gh=${GH_VERSION}

RUN --mount=type=cache,target=/root/.npm,sharing=locked \
    npm install -g @ast-grep/cli@${AST_GREP_VERSION}

ENV SHELL=/bin/bash \
    EDITOR=vim

USER node
WORKDIR /home/node
CMD ["bash"]
