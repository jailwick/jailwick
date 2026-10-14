# Node.js stack: VERSION ("22" or "22.11.0") in /opt/node, first on PATH, with corepack (pnpm and
# yarn), tsx and TypeScript. When the base's own Node.js is that version, it is used as is.
# shellcheck source=../project/lib.sh
. /tmp/jw/lib.sh
COREPACK_VERSION=0.36.0
TSX_VERSION=4.23.15
TYPESCRIPT_VERSION=7.0.2

resolved=$(jw_node_resolve "$VERSION")
# shellcheck disable=SC2086 # URL and checksum
set -- $resolved
jw_resolve_only && echo "$1 $2" && exit 0

want=$(basename "$1")
want=${want#node-}
want=${want%%-linux-*}
prefix=/opt/node
if [ "$(/usr/local/bin/node --version 2>/dev/null || true)" = "$want" ]; then
	prefix=/usr/local # the base's own Node.js: nothing to download
else
	jw_node_install "$1" "$2" "$prefix"
fi
export PATH="$prefix/bin:$PATH"
[ -x "$prefix/bin/corepack" ] || npm install -g --prefix "$prefix" "corepack@$COREPACK_VERSION" # not bundled from Node.js 25
corepack enable --install-directory "$prefix/bin"
npm install -g --prefix "$prefix" "tsx@$TSX_VERSION" "typescript@$TYPESCRIPT_VERSION"
