# Rust {{.Version}}. Cargo's registry persists in ~/.cache, cargo install puts binaries in
# ~/.local/bin.
RUN {{mounts "stacks/rust.sh"}}VERSION={{.Version}} sh /tmp/jw/rust.sh
ENV PATH=/usr/local/cargo/bin:$PATH \
    RUSTUP_HOME=/usr/local/rustup \
    CARGO_HOME=${SANDBOX_HOME}/.cache/cargo \
    CARGO_INSTALL_ROOT=${SANDBOX_HOME}/.local
