# Releasing jailwick

A release has two parts:

- **The CLI** (`jailwick`, with `jw`): built for macOS and Linux by goreleaser, published as a
  GitHub release with checksums, and installed with `brew install jailwick/tap/jailwick`.
- **The desktop app** (`jailwick.app`): needs cgo and Apple's signing tools, so it is built,
  signed and notarized on a Mac by `scripts/release-mac.sh`, attached to the same release, and
  installed with `brew install --cask jailwick/tap/jailwick-app`.

The version is the `VERSION` file; the tag is `v` plus that version.

## Once

1. **The Homebrew tap.** Create the GitHub repo `jailwick/homebrew-tap` with a `Formula/` and a
   `Casks/` folder. Make a fine-grained token that can write its contents; keep it as
   `HOMEBREW_TAP_TOKEN` (goreleaser commits the formula with it).
2. **A Developer ID.** In an Apple Developer account, create a *Developer ID Application*
   certificate and install it in the login keychain. `security find-identity -v -p codesigning`
   shows its name, e.g. `Developer ID Application: Your Name (TEAMID)`.
3. **Notarization credentials**, kept in the keychain under a profile name:
   ```
   xcrun notarytool store-credentials jailwick --apple-id you@example.com --team-id TEAMID
   ```
   It asks for an app-specific password (appleid.apple.com).
4. **goreleaser**: `brew install goreleaser`, then `goreleaser check` to validate
   `.goreleaser.yaml` (it couldn't be validated where it was written).
5. **Private vulnerability reporting.** Turn it on in the repo's Settings → Security, since
   `SECURITY.md` points to it.

## Each release

1. **Version.** Set `VERSION` (e.g. `0.2.6`) and commit. `go test ./...` checks its format.
2. **Checks.** `make lint test`, then on the Mac `make app` and start it once.
3. **Tag.** `git tag -a v0.2.6 -m "jailwick 0.2.6 …"` (the notes become the release's).
4. **The app.**
   ```
   DEVELOPER_ID="Developer ID Application: Your Name (TEAMID)" NOTARY_PROFILE=jailwick \
     scripts/release-mac.sh
   ```
   It leaves `dist-app/jailwick-app_<version>_darwin_universal.zip` and the filled-in cask
   `dist-app/jailwick-app.rb`.
5. **The CLI and the release.**
   ```
   git push origin main v0.2.6
   GITHUB_TOKEN=… HOMEBREW_TAP_TOKEN=… goreleaser release --clean
   ```
   This builds and archives the CLI, writes `checksums.txt`, creates a **draft** release with the
   app's zip attached, and commits `Formula/jailwick.rb` to the tap.
6. **The cask.** Commit `dist-app/jailwick-app.rb` to the tap as `Casks/jailwick-app.rb`.
7. **Check and publish.** On a clean Mac (or another user account):
   ```
   brew install jailwick/tap/jailwick && jw version
   brew install --cask jailwick/tap/jailwick-app
   ```
   The app must open without a Gatekeeper warning. Then publish the draft release.

A dry run, without publishing or signing: `goreleaser release --snapshot --clean`.

## What to know

- **Notarization** needs the hardened runtime, which `release-mac.sh` signs with. If the app
  misbehaves only when notarized, it's missing an entitlement: add an `entitlements.plist` and
  pass `--entitlements` to `codesign`.
- **The universal binary.** The app is built for arm64 and amd64 separately (cgo) and joined
  with `lipo`. Building amd64 on Apple silicon needs only Xcode's command-line tools.
- **The CLI from Homebrew** isn't quarantined, so it doesn't need notarizing. A CLI downloaded
  from the release page in a browser is, and macOS asks once. Homebrew is the supported way.
- **Linux** gets the CLI only. The app's Linux build (`make app` on Linux) isn't packaged yet.
