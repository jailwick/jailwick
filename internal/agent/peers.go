package agent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"

	"github.com/jailwick/jailwick/internal/agentapi"
)

// A project's sessions share its container's PID namespace (see spec.BuildSession), so /proc lists
// the processes of every container in it. Each container's agent counts only its own: those in
// its mount namespace.

var selfMountNS = sync.OnceValue(func() string {
	ns, _ := os.Readlink("/proc/self/ns/mnt")
	return ns
})

// ownProcess reports whether pid runs in this container. Without our own namespace to compare
// with, every process counts.
func ownProcess(pid int) bool {
	self := selfMountNS()
	if self == "" {
		return true
	}
	ns, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "ns", "mnt"))
	return err == nil && ns == self
}

// peerDirs are Claude Code's list of running sessions and its sockets folder: the PeersVolume
// in the project's container and its sessions', the container's own in a private session.
func peerDirs(c *agentapi.Config) []string {
	return []string{filepath.Join(claudeDir(c.Home), "sessions"), agentapi.SocketsDir}
}

// claudeDir is Claude Code's config folder.
func claudeDir(home string) string {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(home, ".claude")
}

// ClearPeers empties the shared list of sessions and their sockets. The project's container calls
// it as it starts: its PID namespace is new, so every entry there describes a process that's gone
// (and whose PID a new process may reuse).
func ClearPeers(c *agentapi.Config) { clearDirs(peerDirs(c)...) }

// clearDirs removes what the folders hold, keeping the folders.
func clearDirs(dirs ...string) {
	for _, dir := range dirs {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			_ = os.RemoveAll(filepath.Join(dir, e.Name()))
		}
	}
}

// Private reports whether this container's session is kept out of messages between sessions.
func Private() bool { return os.Getenv(agentapi.PrivateEnv) != "" }

// peers checks that the project's sessions can find and message each other: both folders are
// the shared volume (not a folder of this container's own), and the user's alone, as Claude Code
// requires of its sockets folder.
func (v *verifier) peers() {
	v.section = "Sessions"
	if Private() {
		v.info("this session is private: no messages to or from other sessions")
		return
	}
	types := mountTypes()
	for _, dir := range peerDirs(v.c) {
		v.expect("shared with the project's sessions: "+dir, sharedDir(dir, types[dir]))
	}
}

// sharedDir checks a folder of the PeersVolume: a mount that isn't in memory, owned by this user,
// with access for no one else.
func sharedDir(dir, fsType string) error {
	switch fsType {
	case "":
		return errors.New("not mounted: this container keeps its own (restart the sandbox)")
	case "tmpfs":
		return errors.New("in this container's memory, not shared (restart the sandbox)")
	}
	info, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok && int(st.Uid) != os.Getuid() {
		return fmt.Errorf("owned by uid %d, not this user", st.Uid)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		return fmt.Errorf("others have access (mode %o)", perm)
	}
	return nil
}
