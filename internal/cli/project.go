package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/hostutil"
	"github.com/jailwick/jailwick/internal/paths"
	"github.com/jailwick/jailwick/internal/projectsetup"
	"github.com/jailwick/jailwick/internal/sandbox"
	"github.com/jailwick/jailwick/internal/workspace"
)

func (a *app) projectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "project",
		Aliases: []string{"projects-config"},
		Short:   "Add, list, show, edit or remove projects",
	}
	cmd.AddCommand(a.projectAddCmd(), a.projectLsCmd(), a.projectShowCmd(), a.projectEditCmd(), a.projectRmCmd())
	return cmd
}

func (a *app) projectAddCmd() *cobra.Command {
	var yes bool
	var given func() map[string]any
	cmd := &cobra.Command{
		Use:   "add [dir]",
		Short: "Add a repo as a project, step by step (detects services and env files)",
		Long: `Add a repo as a project, step by step:
  1. Project: its name.
  2. Services and database logins: Postgres, Redis, MongoDB, their logins, and forwards so
     localhost:<port> inside the sandbox reaches them.
  3. Toolset: the stacks in the sandbox image (Node.js, Python, Go, Rust, Bun) at the versions the
     repo names, Playwright browsers, apt packages.
  4. Network access: the allowlist presets the repo and its stacks call for (package registries,
     services), ticked; broad ones, which accept data from anyone, are suggested but not ticked.
  5. Worktrees: files copied into new worktrees, setup commands.
Answers are prefilled from the repo's .env files (and .env.example when there's no .env).
Every setting also has a flag; --yes takes the prefilled answers without asking.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			return a.projectAdd(dir, given(), yes)
		},
	}
	given = setupFlags(cmd, projectsetup.Form)
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "take the prefilled settings without asking")
	return cmd
}

// projectAdd runs the guided setup: flags first, then a prompt for each remaining setting.
func (a *app) projectAdd(dir string, given map[string]any, yes bool) error {
	d, err := projectsetup.New(a.store, dir)
	if err != nil {
		return err
	}
	if err := d.SetAll(given); err != nil {
		return err
	}
	for _, h := range d.Hints {
		a.ui.Step("%s", h)
	}
	if a.ui.Interactive && !yes && !a.plain {
		if err := askSetup(a, projectsetup.Form, d, given); err != nil {
			return err
		}
	}
	p, err := d.Finish(a.store)
	if err != nil {
		return err
	}
	data, _ := config.Marshal(p)
	a.ui.Println(a.ui.Dim("--- " + paths.Tilde(a.store.ProjectFile(p.Name))))
	a.ui.Print(string(data))
	if a.ui.Interactive && !yes && !a.plain {
		ok, err := a.ui.Confirm("Save this project?", true)
		if err != nil || !ok {
			return errors.Join(err, errors.New("not saved"))
		}
	}
	if err := a.store.SaveProject(p); err != nil {
		return err
	}
	a.ui.OK("Added %s. Start a session with: cd %s && jw claude", p.Name, paths.Tilde(p.Dir))
	// Its folder becomes a member of its workspace, unless it keeps to itself: the other projects
	// see it after a restart.
	if !d.AddsMember() {
		return nil
	}
	if added, err := workspace.AddMember(a.store, p); err != nil {
		a.ui.Warn("couldn't add %s to the members of %s: %v (jw workspace edit %s)", paths.Tilde(p.Dir), p.Workspace, err, p.Workspace)
	} else if others := workspace.Others(a.store, p.Workspace, p.Name); added && len(others) > 0 {
		a.ui.Println(fmt.Sprintf("Its folder is now a member of %s: %s see it after a restart (jw restart in each).", p.Workspace, strings.Join(others, ", ")))
	}
	return nil
}

func (a *app) projectLsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List projects",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return a.listProjects(cmd.Context(), false) },
	}
}

func (a *app) projectShowCmd() *cobra.Command {
	var resolved bool
	cmd := &cobra.Command{
		Use:   "show [name]",
		Short: "Print a project's settings (--resolved: with defaults filled in)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := a.nameArg(args)
			if err != nil {
				return err
			}
			p, err := a.store.Project(name)
			if err != nil {
				return err
			}
			if resolved {
				g, err := a.globalConfig()
				if err != nil {
					return err
				}
				p = p.Resolved(g)
			}
			data, err := config.Marshal(p)
			if err != nil {
				return err
			}
			a.ui.Print(string(data))
			return nil
		},
	}
	cmd.Flags().BoolVar(&resolved, "resolved", false, "fill in defaults")
	return cmd
}

func (a *app) projectEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit [name]",
		Short: "Edit a project's file in $EDITOR, validated on save",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := a.nameArg(args)
			if err != nil {
				return err
			}
			return a.editValidated(a.store.ProjectFile(name), func() error {
				p, err := a.store.Project(name)
				if err != nil {
					return err
				}
				return p.Validate()
			})
		},
	}
}

// editValidated opens a file in the editor until it validates (or the user gives up).
func (a *app) editValidated(file string, validate func() error) error {
	if err := a.editUntilValid(file, validate); err != nil {
		return err
	}
	a.ui.OK("saved %s; changes apply at the next jw up / restart", paths.Tilde(file))
	return nil
}

// editUntilValid opens a file in the editor until it validates (or the user gives up).
func (a *app) editUntilValid(file string, validate func() error) error {
	for {
		if err := hostutil.Edit(file); err != nil {
			return err
		}
		err := validate()
		if err == nil {
			return nil
		}
		a.ui.Fail(err)
		if !a.ui.Interactive {
			return errors.New("the file has errors")
		}
		again, aerr := a.ui.Confirm("Edit again?", true)
		if aerr != nil || !again {
			return errors.New("the file has errors")
		}
	}
}

func (a *app) projectRmCmd() *cobra.Command {
	var purge, yes, sessions bool
	cmd := &cobra.Command{
		Use:   "rm <name>",
		Short: "Remove a project from jailwick (--purge: also its images and data)",
		Long: `Remove a project from jailwick. Its sandbox is stopped first, so none is left running
without a project; that refuses while sessions run unless --include-sessions. Its data
(databases, history, its own Claude login) is kept unless --purge, which also removes its images (as jw destroy
--volumes), so adding the project again with the same name finds it. The repo and its notes are
untouched.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if _, err := a.store.Project(name); err != nil {
				return err
			}
			if purge && !yes {
				ok, err := a.ui.Confirm(fmt.Sprintf("Delete %s's sandbox, databases, history and own Claude login too?", name), false)
				if err != nil || !ok {
					return errors.Join(err, errors.New("not removed"))
				}
			}
			// Stop the sandbox first: one left running without its project would be no one's.
			m, err := a.manager(cmd.Context(), name)
			if err != nil {
				return fmt.Errorf("%w (its sandbox must be stopped first)", err)
			}
			if _, err := m.Remove(cmd.Context(), name,
				sandbox.RemoveOptions{Sessions: sessions, Images: purge, Volumes: purge}); err != nil {
				return sessionsHint(err)
			}
			if err := a.store.DeleteProject(name); err != nil {
				return err
			}
			if purge {
				a.ui.OK("removed %s, its sandbox, images and data (the repo and its notes are untouched)", name)
			} else {
				a.ui.OK("removed %s and stopped its sandbox; its data is kept (the repo and its notes are untouched)", name)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&purge, "purge", false, "also delete its images and data")
	cmd.Flags().BoolVar(&sessions, "include-sessions", false, "also end running sessions")
	cmd.Flags().BoolVar(&yes, "yes", false, "don't ask")
	return cmd
}

// nameArg is the project from args, flags or the current folder.
func (a *app) nameArg(args []string) (string, error) {
	if len(args) == 1 {
		return args[0], nil
	}
	p, _, err := a.target()
	return p, err
}
