#!/bin/sh
# Builds the desktop app for a release on a Mac: a universal jailwick.app (Apple silicon and
# Intel), signed with a Developer ID, notarized and stapled, zipped into dist-app/, with the
# Homebrew cask filled in. Run it before `goreleaser release`, which attaches the zip.
#
#   DEVELOPER_ID="Developer ID Application: Your Name (TEAMID)" NOTARY_PROFILE=jailwick \
#     scripts/release-mac.sh
#
# NOTARY_PROFILE is a notarytool keychain profile (once: xcrun notarytool store-credentials).
# See docs/RELEASING.md.
set -eu

: "${DEVELOPER_ID:?set DEVELOPER_ID to your Developer ID Application identity}"
: "${NOTARY_PROFILE:?set NOTARY_PROFILE to a notarytool keychain profile}"
[ "$(uname)" = Darwin ] || { echo "run this on a Mac" >&2; exit 1; }

version=$(cat VERSION)
out=dist-app
app=$out/jailwick.app
zip=$out/jailwick-app_${version}_darwin_universal.zip
export MACOSX_DEPLOYMENT_TARGET=13.0

step() { printf '• %s\n' "$*"; }

step "jailwick $version: the web app and the agents"
make web agent >/dev/null

step "the app for arm64 and amd64, then one universal binary"
rm -rf "$out" && mkdir -p "$out"
for arch in arm64 amd64; do
	CGO_ENABLED=1 GOOS=darwin GOARCH=$arch go build -trimpath -ldflags "-s -w" \
		-o "$out/jailwick-app-$arch" ./cmd/jailwick-app
done
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
lipo -create -output "$app/Contents/MacOS/jailwick-app" "$out/jailwick-app-arm64" "$out/jailwick-app-amd64"
rm "$out"/jailwick-app-arm64 "$out"/jailwick-app-amd64
cp cmd/jailwick-app/packaging/darwin/jailwick.icns "$app/Contents/Resources/"
sed "s/>VERSION</>$version</" cmd/jailwick-app/packaging/darwin/Info.plist >"$app/Contents/Info.plist"

step "signing (hardened runtime, as notarization requires)"
codesign --force --options runtime --timestamp --sign "$DEVELOPER_ID" "$app"
codesign --verify --strict --verbose=2 "$app"

step "notarizing (a few minutes)"
ditto -c -k --keepParent "$app" "$zip"
xcrun notarytool submit "$zip" --keychain-profile "$NOTARY_PROFILE" --wait
xcrun stapler staple "$app"
rm "$zip" && ditto -c -k --keepParent "$app" "$zip" # the zip now carries the stapled ticket
spctl --assess --type execute --verbose "$app"

step "the Homebrew cask"
sum=$(shasum -a 256 "$zip" | cut -d' ' -f1)
sed -e "s/@VERSION@/$version/" -e "s/@SHA256@/$sum/" packaging/homebrew/jailwick-app.rb.tmpl >"$out/jailwick-app.rb"

printf '\nDone: %s\n  cask: %s (commit it to jailwick/homebrew-tap as Casks/jailwick-app.rb)\n' "$zip" "$out/jailwick-app.rb"
