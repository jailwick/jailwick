# Bun {{.Version}}. Its package cache persists in ~/.cache, bun add -g puts binaries in ~/.local/bin.
RUN {{mounts "stacks/bun.sh"}}VERSION={{.Version}} sh /tmp/jw/bun.sh
ENV BUN_INSTALL_CACHE_DIR=${SANDBOX_HOME}/.cache/bun \
    BUN_INSTALL_BIN=${SANDBOX_HOME}/.local/bin \
    BUN_INSTALL_GLOBAL_DIR=${SANDBOX_HOME}/.local/share/bun/global
