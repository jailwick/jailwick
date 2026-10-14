// jailwick-agent runs inside every jailwick sandbox (mounted read-only at
// /opt/jailwick/bin/jailwick-agent). The host calls it through the runtime's exec.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/jailwick/jailwick/internal/agent"
	"github.com/jailwick/jailwick/internal/agentapi"
	"github.com/jailwick/jailwick/internal/history"
)

const usage = `jailwick-agent: the helper inside a jailwick sandbox.

  init [-session]              container main process: local forwards, then wait (-session:
                               until the container's session ends; without: first empties the
                               project's shared list of sessions)
  session [-worktree N] [-base REF] -- ARGS   start the agent CLI (in a worktree)
  status [-json]               sessions and worktrees
  watch                        the status as JSON lines, again whenever it changes
  exec [-worktree N] -- CMD    run a command (in a worktree, with its environment)
  wt ls [-json] | wt rm N      worktrees
  changes [-worktree N] [-file F]   what a checkout changed (JSON), with a file's diff
  run start|stop|log [-worktree N]   the project's run command, in the background
  worktree-hook create|remove  Claude Code's WorktreeCreate/WorktreeRemove hooks (input on stdin)
  worktree-env                 exports for the worktree of the current folder (BASH_ENV runs it)
  stop                         end every agent session
  verify [-other H:P,...]      isolation checks (JSON); -other: other sandboxes' services
  ports                        listening ports (JSON)
  top [-watch]                 CPU and memory per session
  history [-n N]               saved conversations (JSON)
  version [-json]              the agent's content hash (as in its file name on the host);
                               -json: with the host contract it speaks
`

func main() {
	log.SetFlags(0)
	log.SetPrefix("jailwick-agent: ")
	// Mounted as xclip and wl-paste in a session container: the pasted image (see agent.Clipboard).
	if agent.IsClipboardTool(os.Args[0]) {
		os.Exit(agent.Clipboard(os.Args[0], os.Args[1:], agentapi.ClipboardDir, os.Stdout, os.Stderr))
	}
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	var err error
	switch cmd {
	case "init":
		err = runInit(args)
	case "session":
		err = runSession(args)
	case "status":
		err = runStatus(args)
	case "watch":
		err = runWatch()
	case "version":
		err = printVersion(slices.Contains(args, "-json"))
	default:
		err = runMore(cmd, args)
	}
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func config() *agentapi.Config {
	c, err := agent.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}
	return c
}

// runInit starts the local forwards and waits for the container to stop. In a session
// container (-session) it also returns when the container's session ends, which ends the
// container.
func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	session := fs.Bool("session", false, "end with this container's session")
	_ = fs.Parse(args)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	c, err := agent.LoadConfig()
	if err != nil {
		log.Print(err) // keep the container up so the user can inspect it
	} else {
		if !*session {
			agent.ClearPeers(c) // before any session of this PID namespace registers
		}
		for _, f := range c.Forwards {
			go func() {
				if err := agent.Forward(ctx, f); err != nil {
					log.Print(err)
				}
			}()
		}
	}
	if *session {
		agent.WaitForSession(ctx, agentapi.SessionPIDFile, 2*time.Minute, agent.Alive)
		return nil
	}
	<-ctx.Done()
	return nil
}

func runSession(args []string) error {
	fs := flag.NewFlagSet("session", flag.ExitOnError)
	wt := fs.String("worktree", "", "worktree name")
	base := fs.String("base", "HEAD", "base for a new worktree branch")
	_ = fs.Parse(args)
	c := config()
	if os.Getenv(agentapi.SessionEnv) != "" {
		// This process becomes the agent: the container lives as long as it does.
		if err := agent.WriteSessionPID(agentapi.SessionPIDFile); err != nil {
			return err
		}
		if err := agent.WaitProxy(context.Background(), os.Getenv("HTTPS_PROXY"), 15*time.Second); err != nil {
			return err
		}
	}
	args = agent.WithNotifyChannel(fs.Args(), os.Getenv(agentapi.NotifyEnv), os.TempDir())
	if *wt == "" {
		return agent.Exec(c, c.Dir, os.Environ(), args)
	}
	return agent.StartWorktree(c, *wt, *base, args)
}

func runStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "print JSON")
	_ = fs.Parse(args)
	c := config()
	st := agent.Status(c)
	if *asJSON {
		return agent.PrintJSON(st)
	}
	for _, s := range st.Sessions {
		where := "main checkout"
		if s.Worktree != "" {
			where = "wt " + s.Worktree
		}
		fmt.Printf("%d\t%s\t%s\n", s.PID, where, s.Name)
	}
	return nil
}

// runWatch prints the status as JSON lines when it changes, until the host goes away (see
// agent.WatchStatus): the app follows a container with it instead of asking status repeatedly.
func runWatch() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	return agent.Watch(ctx, config(), os.Stdout)
}

// runMore handles the commands added by later modules.
func runMore(cmd string, args []string) error {
	switch cmd {
	case "exec":
		fs := flag.NewFlagSet("exec", flag.ExitOnError)
		wt := fs.String("worktree", "", "worktree name")
		_ = fs.Parse(args)
		return agent.ExecIn(config(), *wt, fs.Args())
	case "wt":
		return runWorktree(args)
	case "worktree-hook":
		if len(args) != 1 {
			return fmt.Errorf("usage: worktree-hook create|remove (Claude Code's input on stdin)")
		}
		return agent.WorktreeHook(config(), args[0], os.Stdin, os.Stdout)
	case "run":
		fs := flag.NewFlagSet("run", flag.ExitOnError)
		wt := fs.String("worktree", "", "worktree name; empty: the main checkout")
		if len(args) == 0 {
			return fmt.Errorf("usage: run start|stop|log [-worktree N]")
		}
		_ = fs.Parse(args[1:])
		switch args[0] {
		case "start", "stop": // then the runs, as status has them
			c := config()
			err := agent.StopRun(*wt)
			if args[0] == "start" {
				err = agent.StartRun(c, *wt)
			}
			if err != nil {
				return err
			}
			return agent.PrintJSON(agent.Runs(c))
		case "log":
			return agent.PrintJSON(agent.RunLog(*wt))
		}
		return fmt.Errorf("usage: run start|stop|log [-worktree N]")
	case "changes":
		fs := flag.NewFlagSet("changes", flag.ExitOnError)
		wt := fs.String("worktree", "", "worktree name; empty: the main checkout")
		file := fs.String("file", "", "also this file's diff")
		_ = fs.Parse(args)
		ch, err := agent.Changes(config(), *wt, *file)
		if err != nil {
			return err
		}
		return agent.PrintJSON(ch)
	case "worktree-env":
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		agent.PrintExports(os.Stdout, agent.WorktreeEnvFor(config(), dir))
		return nil
	case "stop":
		return agent.StopSessions(config())
	case "verify":
		fs := flag.NewFlagSet("verify", flag.ExitOnError)
		others := fs.String("other", "", "host:port of other sandboxes' services, comma-separated")
		_ = fs.Parse(args)
		var list []string
		if *others != "" {
			list = strings.Split(*others, ",")
		}
		return agent.PrintJSON(agent.Verify(config(), list))
	case "ports":
		return agent.PrintJSON(agent.Ports(config()))
	case "history":
		fs := flag.NewFlagSet("history", flag.ExitOnError)
		n := fs.Int("n", 200, "how many")
		_ = fs.Parse(args)
		c := config()
		return agent.PrintJSON(history.List(history.Root(""), history.Options{
			Limit: *n, Missing: true,
			Where: func(dir string) string {
				switch {
				case dir == c.Dir:
					return "main checkout"
				case agent.WorktreeOf(c.Dir, dir) != "":
					return "wt " + agent.WorktreeOf(c.Dir, dir)
				}
				return dir
			},
		}))
	case "top":
		fs := flag.NewFlagSet("top", flag.ExitOnError)
		watch := fs.Bool("watch", false, "refresh every two seconds")
		_ = fs.Parse(args)
		agent.Top(config(), os.Stdout, *watch)
		return nil
	}
	fmt.Fprint(os.Stderr, usage)
	return fmt.Errorf("unknown command %q", cmd)
}

func runWorktree(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: wt ls [-json] | wt add NAME [-base B] | wt check NAME | wt branches | wt rm NAME [-force] [-branch merged|any]")
	}
	c := config()
	switch args[0] {
	case "ls":
		fs := flag.NewFlagSet("wt ls", flag.ExitOnError)
		asJSON := fs.Bool("json", false, "print JSON")
		_ = fs.Parse(args[1:])
		wts := agent.ListWorktrees(c)
		if *asJSON {
			return agent.PrintJSON(wts)
		}
		for _, w := range wts {
			fmt.Printf("%s\t%d\t%v\t%d\t%s\n", w.Name, w.Port, w.DBs, w.RedisDB, w.Branch)
		}
		return nil
	case "rm":
		fs := flag.NewFlagSet("wt rm", flag.ExitOnError)
		force := fs.Bool("force", false, "remove even with uncommitted changes")
		branch := fs.String("branch", "", "also delete the branch: merged (only if merged) or any")
		_ = fs.Parse(args[1:])
		if fs.NArg() != 1 || (*branch != "" && *branch != "merged" && *branch != "any") {
			return fmt.Errorf("usage: wt rm NAME [-force] [-branch merged|any]")
		}
		return agent.RemoveWorktree(c, fs.Arg(0), agent.RemoveOptions{Force: *force, Branch: *branch})
	case "add": // a worktree without a session in it (the app's New worktree); JSON on stdout
		fs := flag.NewFlagSet("wt add", flag.ExitOnError)
		base := fs.String("base", "HEAD", "branch or commit a new worktree starts from")
		_ = fs.Parse(args[1:])
		if fs.NArg() != 1 {
			return fmt.Errorf("usage: wt add NAME [-base B]")
		}
		w, err := agent.AddWorktree(c, fs.Arg(0), *base)
		if err != nil {
			return err
		}
		return agent.PrintJSON(w)
	case "check":
		if len(args) != 2 {
			return fmt.Errorf("usage: wt check NAME")
		}
		ch, err := agent.CheckWorktree(c, args[1])
		if err != nil {
			return err
		}
		return agent.PrintJSON(ch)
	case "branches":
		return agent.PrintJSON(agent.Branches(c))
	}
	return fmt.Errorf("unknown wt command %q", args[0])
}

// printVersion prints the hash of the agent's own bytes, or with asJSON the contract it speaks
// too. The agent has no version stamp of its own (see the Makefile): jailwick names the agent file
// it mounts by this same hash.
func printVersion(asJSON bool) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	build := hex.EncodeToString(sum[:6])
	if asJSON {
		return agent.PrintJSON(agentapi.Version{Contract: agentapi.Contract, Build: build})
	}
	fmt.Println(build)
	return nil
}
