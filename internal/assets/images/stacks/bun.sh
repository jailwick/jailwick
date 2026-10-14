# Bun stack: bun VERSION ("1" for its latest 1.x, or "1.2.3") from npm, which checks the
# package's integrity.
# shellcheck source=../project/lib.sh
. /tmp/jw/lib.sh

jw_resolve_only && echo "npm bun@$VERSION" && exit 0
PATH="/usr/local/bin:$PATH" npm install -g --prefix /usr/local "bun@$VERSION"
