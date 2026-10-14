# Prerequisites of jailwick's layer, on any base: apt keeps its downloads (for the build's cache
# mounts), the tools the install scripts use, and Node.js for installing Claude Code when the
# base has none. On jailwick's base everything is there already.
# shellcheck source=lib.sh
. /tmp/jw/lib.sh

command -v apt-get >/dev/null || jw_fail "jailwick's layer needs a Debian or Ubuntu base image (apt-get)"
rm -f /etc/apt/apt.conf.d/docker-clean
echo 'Binary::apt::APT::Keep-Downloaded-Packages "true";' >/etc/apt/apt.conf.d/keep-downloads
jw_apt_install ca-certificates curl xz-utils jq git

# Claude Code installs with npm and needs Node.js 22 or later.
major=$(/usr/local/bin/node -p 'process.versions.node.split(".")[0]' 2>/dev/null || echo 0)
if [ "$major" -lt 22 ]; then
	resolved=$(jw_node_resolve "$NODE_DEFAULT")
	# shellcheck disable=SC2086 # URL and checksum
	set -- $resolved
	jw_node_install "$1" "$2" /usr/local
fi
