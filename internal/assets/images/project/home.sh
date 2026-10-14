# The sandbox user's home: the folders that become volumes exist and belong to the user (a new
# volume takes them over), git trusts the mounted repos, and Gradle, which ignores HTTPS_PROXY, is
# told about the proxy and the sandbox's own hosts.
# shellcheck source=lib.sh
. /tmp/jw/lib.sh

# jailwick never creates the user: a base image of the user's must have it.
if ! id -u "$SANDBOX_USER" >/dev/null 2>&1; then
	echo "jailwick: the image has no user $SANDBOX_USER: set image.user (the project's Settings, Image) to one it has, or add it in your Dockerfile (useradd)" >&2
	exit 1
fi

for d in .claude .npm .cache .local/bin .gradle/caches; do
	mkdir -p "$SANDBOX_HOME/$d"
done
chown "$SANDBOX_USER" "$SANDBOX_HOME" "$SANDBOX_HOME/.local" "$SANDBOX_HOME/.gradle"
for d in .claude .npm .cache .local/bin .gradle/caches; do
	chown -R "$SANDBOX_USER" "$SANDBOX_HOME/$d"
done
git config --system init.defaultBranch main
git config --system --replace-all safe.directory '*'
cat >"$SANDBOX_HOME/.gradle/gradle.properties" <<PROPS
systemProp.http.proxyHost=egress-proxy
systemProp.http.proxyPort=3128
systemProp.https.proxyHost=egress-proxy
systemProp.https.proxyPort=3128
systemProp.http.nonProxyHosts=localhost|127.0.0.1|postgres|redis|mongodb|postgres-test|redis-test|mongodb-test
PROPS
chown "$SANDBOX_USER" "$SANDBOX_HOME/.gradle/gradle.properties"
