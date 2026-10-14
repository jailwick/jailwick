package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/jailwick/jailwick/internal/agentapi"
)

// Channels are the values of Claude Code's preferredNotifChannel a session may get.
var Channels = []string{"iterm2", "iterm2_with_bell", "kitty", "ghostty", "terminal_bell", "notifications_disabled"}

// WithNotifyChannel returns the agent's arguments with its settings (--settings <file>) set to
// notify through channel: a copy of the file with preferredNotifChannel, in dir. Settings
// passed with --settings outrank Claude's own, so the session's terminal decides, not a
// /config choice made in another terminal. Without a channel, or when the file can't be read,
// the arguments stay as they are: Claude Code then picks the channel itself.
func WithNotifyChannel(args []string, channel, dir string) []string {
	if channel == "" || !slices.Contains(Channels, channel) {
		return args
	}
	for i := 0; i+1 < len(args); i++ {
		if args[i] != "--settings" {
			continue
		}
		data, err := os.ReadFile(args[i+1])
		if err != nil {
			return args
		}
		settings := map[string]any{}
		if json.Unmarshal(data, &settings) != nil {
			return args
		}
		settings["preferredNotifChannel"] = channel
		out, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			return args
		}
		file := filepath.Join(dir, fmt.Sprintf("jailwick-settings-%d.json", os.Getpid()))
		if os.WriteFile(file, out, 0o600) != nil {
			return args
		}
		next := append([]string{}, args...)
		next[i+1] = file
		return next
	}
	return args
}

// notifyOf reads a process's notification channel from its environment (NotifyEnv).
func notifyOf(pid int) string {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "environ"))
	if err != nil {
		return ""
	}
	prefix := []byte(agentapi.NotifyEnv + "=")
	for _, kv := range bytes.Split(data, []byte{0}) {
		if v, ok := bytes.CutPrefix(kv, prefix); ok {
			return string(v)
		}
	}
	return ""
}
