# Go stack: VERSION ("1.25" for its latest release, or "1.25.3") in /usr/local/go, checked against
# the checksum go.dev publishes.
# shellcheck source=../project/lib.sh
. /tmp/jw/lib.sh

index=$(jw_get "https://go.dev/dl/?mode=json&include=all")
release=$(echo "$index" | jq -r --arg v "go$VERSION" \
	'[.[] | select(.stable) | .version | select(. == $v or startswith($v + "."))][0] // empty')
[ -n "$release" ] || jw_fail "Go $VERSION: no such release"
file=$(echo "$index" | jq -r --arg v "$release" --arg a "$(jw_arch)" \
	'.[] | select(.version == $v) | .files[] | select(.os == "linux" and .arch == $a and .kind == "archive") | "\(.filename) \(.sha256)"')
[ -n "$file" ] || jw_fail "Go ${release#go}: no build for linux-$(jw_arch)"
url="https://go.dev/dl/${file% *}"
jw_resolve_only && echo "$url ${file#* }" && exit 0

rm -rf /usr/local/go
tar -xzf "$(jw_fetch "$url" "${file#* }")" -C /usr/local --no-same-owner
