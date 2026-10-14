# Python stack: uv, and Python VERSION ("3.13" or "3.13.1") managed by uv in /opt/uv/python,
# linked as python and python3.
# shellcheck source=../project/lib.sh
. /tmp/jw/lib.sh
UV_VERSION=0.13.0

case "$(jw_arch)" in
amd64) t=x86_64-unknown-linux-gnu sum=1468ebd5a5541121837c5a2817b9972ba6090fa6caa3d142620850a47fb75154 ;;
arm64) t=aarch64-unknown-linux-gnu sum=3ccfb6af6e242433eb552f7d9676abd5412c6595c497e990d9c8cb7b5bd4d2c3 ;;
esac
url="https://github.com/astral-sh/uv/releases/download/$UV_VERSION/uv-$t.tar.gz"
jw_resolve_only && echo "$url $sum" && exit 0

tar -xzf "$(jw_fetch "$url" "$sum")" -C /usr/local/bin --strip-components=1 --no-same-owner "uv-$t/uv" "uv-$t/uvx"
export UV_PYTHON_INSTALL_DIR=/opt/uv/python UV_PYTHON_BIN_DIR=/usr/local/bin UV_CACHE_DIR=/var/cache/jailwick/uv
uv python install "$VERSION"
python=$(UV_PYTHON_PREFERENCE=only-managed uv python find --no-project "$VERSION")
ln -sf "$python" /usr/local/bin/python3
ln -sf "$python" /usr/local/bin/python
chown -R "$SANDBOX_USER" /opt/uv # so `uv python install` works in the sandbox too
