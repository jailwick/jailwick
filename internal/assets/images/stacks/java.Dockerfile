# Java {{.Version}}: Eclipse Temurin's JDK. Build with the project's ./mvnw or ./gradlew (or add
# maven or gradle to the apt packages). Java ignores HTTPS_PROXY, so the proxy goes in its options.
RUN {{mounts "stacks/java.sh"}}VERSION={{.Version}} sh /tmp/jw/java.sh
ENV JAVA_HOME=/usr/local/java \
    PATH=/usr/local/java/bin:$PATH \
    JAVA_TOOL_OPTIONS="-Dhttp.proxyHost=egress-proxy -Dhttp.proxyPort=3128 -Dhttps.proxyHost=egress-proxy -Dhttps.proxyPort=3128 -Dhttp.nonProxyHosts=localhost|127.0.0.1|postgres|redis|mongodb|postgres-test|redis-test|mongodb-test"
