# Android toolchain: JDK 17 and the Android SDK (platform-tools, a platform and build-tools). Google
# ships build-tools for linux x86_64 only, so jailwick builds this image for linux/amd64.
# shellcheck source=lib.sh
. /tmp/jw/lib.sh
ANDROID_PLATFORM=android-35
ANDROID_BUILD_TOOLS=35.0.0
url=https://dl.google.com/android/repository/commandlinetools-linux-11076708_latest.zip
sum=2d2d50857e4eb553af5a6dc3ad507a17adf43d115264b1afc116f95c92e5e258 # Google publishes its SHA-1
jw_resolve_only && echo "$url $sum" && exit 0

jw_apt_install openjdk-17-jdk-headless unzip
export ANDROID_HOME=/opt/android-sdk
rm -rf "$ANDROID_HOME/cmdline-tools"
mkdir -p "$ANDROID_HOME/cmdline-tools"
unzip -q "$(jw_fetch "$url" "$sum")" -d "$ANDROID_HOME/cmdline-tools"
mv "$ANDROID_HOME/cmdline-tools/cmdline-tools" "$ANDROID_HOME/cmdline-tools/latest"
yes | "$ANDROID_HOME/cmdline-tools/latest/bin/sdkmanager" --licenses >/dev/null
"$ANDROID_HOME/cmdline-tools/latest/bin/sdkmanager" platform-tools "platforms;$ANDROID_PLATFORM" "build-tools;$ANDROID_BUILD_TOOLS"
chown -R "$SANDBOX_USER" "$ANDROID_HOME"
