# Helpers for jailwick's install scripts, sourced by them. POSIX sh, run as root during the build.
# With JW_RESOLVE_ONLY=1 a script prints what it would download and its checksum, and stops
# (a check that needs only the network).
set -eu

jw_fail() {
	echo "jailwick: $*" >&2
	exit 1
}

jw_resolve_only() { [ "${JW_RESOLVE_ONLY:-}" = 1 ]; }

# jw_arch prints the Debian architecture: amd64 or arm64.
jw_arch() {
	a=$(dpkg --print-architecture)
	case "$a" in amd64 | arm64) echo "$a" ;; *) jw_fail "unsupported architecture $a" ;; esac
}

# jw_get prints a small text file (an index, a checksum list).
jw_get() { curl -fsSL --retry 3 "$1"; }

# jw_fetch URL SHA256 downloads a file into the build's download cache (a cache mount), checks its
# checksum and prints its path. A cached file with the right checksum isn't downloaded again.
jw_fetch() {
	dir=${JW_CACHE:-/var/cache/jailwick}/downloads
	mkdir -p "$dir"
	file="$dir/$(printf '%s' "$1" | sha256sum | cut -c1-12)-$(basename "$1")"
	if ! echo "$2  $file" | sha256sum -c - >/dev/null 2>&1; then
		curl -fsSL --retry 3 -o "$file.part" "$1"
		echo "$2  $file.part" | sha256sum -c - >/dev/null 2>&1 || {
			rm -f "$file.part"
			jw_fail "$1: checksum mismatch"
		}
		mv "$file.part" "$file"
	fi
	echo "$file"
}

# jw_apt_install installs the Debian packages that aren't installed yet.
jw_apt_install() {
	missing=""
	for p in "$@"; do
		dpkg -s "$p" >/dev/null 2>&1 || missing="$missing $p"
	done
	[ -z "$missing" ] && return 0
	apt-get update -q
	# shellcheck disable=SC2086 # one word per package
	apt-get install -y --no-install-recommends $missing
}

# jw_node_resolve VERSION prints "URL SHA256" of the Node.js build for a major ("22") or an exact
# version ("22.11.0"), from nodejs.org's checksum list.
jw_node_resolve() {
	case "$(jw_arch)" in amd64) a=x64 ;; arm64) a=arm64 ;; esac
	case "$1" in
	*.*) dir="https://nodejs.org/dist/v$1" ;;
	*) dir="https://nodejs.org/dist/latest-v$1.x" ;;
	esac
	line=$(jw_get "$dir/SHASUMS256.txt" | grep " node-v[0-9.]*-linux-$a\.tar\.xz$") ||
		jw_fail "Node.js $1: no build for linux-$a"
	echo "$dir/${line##* } ${line%% *}"
}

# jw_node_install URL SHA256 PREFIX unpacks a Node.js build into PREFIX.
jw_node_install() {
	tarball=$(jw_fetch "$1" "$2")
	mkdir -p "$3"
	tar -xJf "$tarball" -C "$3" --strip-components=1 --no-same-owner \
		--exclude='*/CHANGELOG.md' --exclude='*/README.md' --exclude='*/LICENSE'
}
