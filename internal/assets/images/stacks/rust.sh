# Rust stack: rustup with toolchain VERSION ("stable", "1.82", "nightly-2026-01-15"), rustfmt and
# clippy. Toolchains live in /usr/local/rustup, which the sandbox user may add to (a repo's
# rust-toolchain.toml).
# shellcheck source=../project/lib.sh
. /tmp/jw/lib.sh

case "$(jw_arch)" in amd64) t=x86_64-unknown-linux-gnu ;; arm64) t=aarch64-unknown-linux-gnu ;; esac
url="https://static.rust-lang.org/rustup/dist/$t/rustup-init"
sum=$(jw_get "$url.sha256" | cut -d' ' -f1)
jw_resolve_only && echo "$url $sum" && exit 0

install -m 0755 "$(jw_fetch "$url" "$sum")" /tmp/rustup-init
RUSTUP_HOME=/usr/local/rustup CARGO_HOME=/usr/local/cargo /tmp/rustup-init -y --no-modify-path \
	--profile minimal --default-toolchain "$VERSION" --component rustfmt,clippy
rm /tmp/rustup-init
chown -R "$SANDBOX_USER" /usr/local/rustup
