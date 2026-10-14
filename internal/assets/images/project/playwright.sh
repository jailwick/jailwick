# Playwright: the MCP server Claude uses, the Playwright CLI at the version that server uses, and
# BROWSERS for them and for each of the project's own Playwright VERSIONS (its @playwright/test).
#
# The browsers are downloaded into the build's download cache (a cache mount), which every
# project's build shares and keeps: each browser version is downloaded once, and a build copies
# what its project needs from there into the image.
# shellcheck source=lib.sh
. /tmp/jw/lib.sh
MCP_VERSION=0.0.83
jw_resolve_only && echo "npm @playwright/mcp@$MCP_VERSION" && exit 0

cache=${JW_CACHE:-/var/cache/jailwick}/ms-playwright
export PATH="/usr/local/bin:$PATH" PLAYWRIGHT_BROWSERS_PATH="$cache"
npm install -g --prefix /usr/local "@playwright/mcp@$MCP_VERSION"
pw=$(node -p "require(require.resolve('playwright-core/package.json', {paths: ['/usr/local/lib/node_modules/@playwright/mcp']})).version")
npm install -g --prefix /usr/local "playwright@$pw"

# install_browsers CLI…: the browsers with their system libraries (--with-deps updates apt's
# lists itself), into the cache (a browser already there isn't downloaded again), then their
# folders into the image. The folders are the ones the dry run names; a Playwright too old to
# name them gets the whole cache.
install_browsers() {
	# shellcheck disable=SC2086 # one word per browser
	"$@" install --with-deps $BROWSERS
	# shellcheck disable=SC2086
	dirs=$("$@" install --dry-run $BROWSERS 2>/dev/null | sed -n 's/^ *Install location: *//p')
	if [ -z "$dirs" ]; then
		echo "jailwick: $* names no browser folders; copying the whole browser cache"
		dirs=$(find "$cache" -mindepth 1 -maxdepth 1 -type d ! -name '.*')
	fi
	mkdir -p /ms-playwright
	for d in $dirs; do
		[ -d "/ms-playwright/$(basename "$d")" ] || cp -a "$d" /ms-playwright/
	done
}

install_browsers playwright
for v in $VERSIONS; do
	install_browsers npx -y "playwright@$v"
done
chown -R "$SANDBOX_USER" /ms-playwright
