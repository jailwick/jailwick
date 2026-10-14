# Java stack: VERSION (a feature release: "21") of Eclipse Temurin's JDK in /usr/local/java, checked
# against the checksum Adoptium publishes. Maven gets a settings.xml with the sandbox's proxy (Java
# ignores HTTPS_PROXY) and its repository in ~/.cache, which outlives the container.
# shellcheck source=../project/lib.sh
. /tmp/jw/lib.sh

case $(jw_arch) in amd64) arch=x64 ;; arm64) arch=aarch64 ;; esac
index=$(jw_get "https://api.adoptium.net/v3/assets/latest/$VERSION/hotspot?architecture=$arch&image_type=jdk&os=linux&vendor=eclipse")
file=$(echo "$index" | jq -r '.[0].binary.package // empty | "\(.link) \(.checksum)"')
[ -n "$file" ] || jw_fail "Java $VERSION: no Temurin JDK for linux-$arch"
jw_resolve_only && echo "$file" && exit 0

rm -rf /usr/local/java
mkdir -p /usr/local/java
tar -xzf "$(jw_fetch "${file% *}" "${file#* }")" -C /usr/local/java --strip-components=1 --no-same-owner

mkdir -p "$SANDBOX_HOME/.m2"
cat >"$SANDBOX_HOME/.m2/settings.xml" <<XML
<settings>
  <localRepository>$SANDBOX_HOME/.cache/maven/repository</localRepository>
  <proxies>
    <proxy><id>http</id><active>true</active><protocol>http</protocol><host>egress-proxy</host><port>3128</port>
      <nonProxyHosts>localhost|127.0.0.1|postgres|redis|mongodb|postgres-test|redis-test|mongodb-test</nonProxyHosts></proxy>
    <proxy><id>https</id><active>true</active><protocol>https</protocol><host>egress-proxy</host><port>3128</port>
      <nonProxyHosts>localhost|127.0.0.1|postgres|redis|mongodb|postgres-test|redis-test|mongodb-test</nonProxyHosts></proxy>
  </proxies>
</settings>
XML
# The user owns it; a missing user is reported by home.sh, with what to do.
chown -R "$SANDBOX_USER" "$SANDBOX_HOME/.m2" 2>/dev/null || true
