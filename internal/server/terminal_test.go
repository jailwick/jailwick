package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jailwick/jailwick/internal/config"
)

func TestSessionCommand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", "") // no jw on the PATH: the command names jailwick
	repo := filepath.Join(home, "code", "api")
	os.MkdirAll(filepath.Join(repo, ".jailwick", "worktrees", "auth"), 0o755)
	os.MkdirAll(filepath.Join(repo, ".claude", "worktrees", "old"), 0o755)
	p := config.Project{Name: "api", Dir: repo}

	for _, c := range []struct {
		name string
		s    Session
		want string
	}{
		{"main checkout", Session{}, "cd ~/code/api && jailwick claude"},
		{"shell", Session{Command: "shell"}, "cd ~/code/api && jailwick shell"},
		{"whole internet", Session{Open: true}, "cd ~/code/api && jailwick claude --open"},
		{"private", Session{Private: true}, "cd ~/code/api && jailwick claude --private"},
		{"private shell: no flag", Session{Command: "shell", Private: true}, "cd ~/code/api && jailwick shell"},
		{"a worktree", Session{Worktree: "auth"}, "cd ~/code/api/.jailwick/worktrees/auth && jailwick claude"},
		{"a worktree from before 0.2.5", Session{Worktree: "old"}, "cd ~/code/api/.claude/worktrees/old && jailwick claude"},
		{"a new worktree", Session{Worktree: "feat/x"}, "cd ~/code/api && jailwick wt feat/x"},
		{"resume where it ran", Session{Resume: "3f2a-91", Dir: filepath.Join(repo, ".jailwick", "worktrees", "auth")},
			"cd ~/code/api/.jailwick/worktrees/auth && jailwick claude 3f2a-91"},
		{"a terminal in a folder", Session{Command: "folder", Dir: filepath.Join(repo, ".jailwick", "worktrees", "auth")},
			"cd ~/code/api/.jailwick/worktrees/auth"},
		{"a terminal in the repo", Session{Command: "folder"}, "cd ~/code/api"},
	} {
		got, err := SessionCommand(p, c.s)
		if err != nil || got != c.want {
			t.Errorf("%s: %q %v, want %q", c.name, got, err, c.want)
		}
	}
	for _, s := range []Session{{Resume: "x; rm -rf ~"}, {Resume: "abc", Dir: "/etc"}, {Command: "folder", Dir: "/etc"}} {
		if _, err := SessionCommand(p, s); err == nil {
			t.Errorf("%+v must be refused", s)
		}
	}
}

func TestCheckTerminal(t *testing.T) {
	for _, ok := range []string{"", "kitty", "iterm2", "alacritty -e sh -c {cmd}"} {
		if CheckTerminal(ok) != nil {
			t.Errorf("%q refused", ok)
		}
	}
	if CheckTerminal("xterm") == nil {
		t.Error("xterm without {cmd} accepted")
	}
}
