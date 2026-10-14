package image

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/jailwick/jailwick/internal/runtime"
)

// Checking an image tries what jailwick needs from it before a start would fail on it: a tiny
// image FROM it whose one step runs checkScript (so it works the same on every runtime, and the
// image is pulled for the platform asked for). Nothing of it is kept.

// CheckOptions say what to check an image for.
type CheckOptions struct {
	Ref      string // the image
	User     string // the sandbox user (default node)
	Home     string // its home (default /home/node)
	AsIs     bool   // used as is: Claude Code, git and the home must be there too
	Platform string // linux/amd64 or linux/arm64; "" the machine's
}

// Finding is one thing checked: OK, or why not.
type Finding struct {
	Name   string `json:"name"` // distro, libc, arch, user, home, claude, git
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

// checkScript prints one "JWCHECK ok|bad <name> <detail>" line per finding. $1 user, $2 home,
// $3 "asis" or "".
const checkScript = `#!/bin/sh
say() { echo "JWCHECK $1 $2 $3"; }
name=unknown
[ -r /etc/os-release ] && name=$(. /etc/os-release && echo "${PRETTY_NAME:-$ID}")
if command -v apt-get >/dev/null 2>&1 && command -v dpkg >/dev/null 2>&1; then
	say ok distro "$name"
else
	say bad distro "$name: jailwick needs Debian or Ubuntu (apt), e.g. a -bookworm or -jammy tag"
fi
if glibc=$(getconf GNU_LIBC_VERSION 2>/dev/null) || glibc=$(ldd --version 2>&1 | grep -iE 'glibc|gnu libc' | head -n1); then
	say ok libc "$glibc"
else
	say bad libc "no glibc (musl?): Node and the stacks need glibc"
fi
case $(uname -m) in
x86_64|aarch64) say ok arch "$(uname -m)" ;;
*) say bad arch "$(uname -m): jailwick runs on amd64 or arm64" ;;
esac
if id -u "$1" >/dev/null 2>&1; then
	say ok user "$1 (uid $(id -u "$1"))"
else
	say bad user "no user $1: set the sandbox user to one the image has, or add it (useradd)"
fi
if [ -d "$2" ]; then
	say ok home "$2"
elif [ "$3" = asis ]; then
	say bad home "$2 isn't there: an image used as is must have the user's home"
else
	say ok home "$2 (made by jailwick's layer)"
fi
if [ "$3" = asis ]; then
	for c in claude git; do
		if command -v "$c" >/dev/null 2>&1; then say ok "$c" "$(command -v "$c")"; else say bad "$c" "not in the image (an image used as is gets nothing added)"; fi
	done
fi
`

var findingLine = regexp.MustCompile(`JWCHECK (ok|bad) (\w+) (.*)$`)

// Check tries an image for jailwick (see CheckOptions). An error is about the check itself: the
// image can't be pulled (it doesn't exist, or needs a login), or has no shell to run it.
func (b *Builder) Check(ctx context.Context, o CheckOptions) ([]Finding, error) {
	if o.User == "" {
		o.User = "node"
	}
	if o.Home == "" {
		o.Home = "/home/" + o.User
	}
	asIs := ""
	if o.AsIs {
		asIs = "asis"
	}
	files := map[string][]byte{
		"Dockerfile": []byte(fmt.Sprintf("FROM %s\nUSER root\nCOPY check.sh /tmp/jwcheck.sh\nRUN sh /tmp/jwcheck.sh %q %q %q\n",
			o.Ref, o.User, o.Home, asIs)),
		"check.sh": []byte(checkScript),
	}
	dir, err := b.writeContext(files)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(o.Ref + o.User + o.Home + asIs + o.Platform))
	tag := "jailwick-check:" + hex.EncodeToString(sum[:6])
	var out bytes.Buffer
	err = b.Driver.Build(ctx, runtime.BuildOptions{Context: dir, Tag: tag, Platform: o.Platform, NoCache: true, Out: &out})
	_ = b.Driver.RemoveImage(context.WithoutCancel(ctx), tag)
	_ = os.RemoveAll(dir)
	var found []Finding
	for _, line := range strings.Split(out.String(), "\n") {
		if m := findingLine.FindStringSubmatch(line); m != nil {
			found = append(found, Finding{Name: m[2], OK: m[1] == "ok", Detail: strings.TrimSpace(m[3])})
		}
	}
	if len(found) == 0 {
		return nil, fmt.Errorf("couldn't check %s: %s", o.Ref, checkFailure(out.String(), err))
	}
	return found, nil
}

// checkFailure says why an image couldn't be checked, from the build's output.
func checkFailure(out string, err error) string {
	low := strings.ToLower(out)
	switch {
	case strings.Contains(low, "not found") || strings.Contains(low, "manifest unknown") || strings.Contains(low, "does not exist"):
		return "the image or tag doesn't exist (or is private: log in to its registry first)"
	case strings.Contains(low, "unauthorized") || strings.Contains(low, "denied"):
		return "the registry refused it: log in to it first (docker login)"
	case strings.Contains(low, "no match for platform") || strings.Contains(low, "no matching manifest"):
		return "it has no build for this machine's architecture"
	case strings.Contains(low, "/bin/sh") && strings.Contains(low, "no such file"):
		return "it has no shell (distroless?): jailwick needs Debian or Ubuntu"
	}
	if err != nil {
		lines := strings.Split(strings.TrimSpace(out), "\n")
		return fmt.Sprintf("%v: %s", err, lines[len(lines)-1])
	}
	return "it printed nothing"
}
