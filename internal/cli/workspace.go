package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jailwick/jailwick/internal/paths"
	"github.com/jailwick/jailwick/internal/workspace"
	"github.com/jailwick/jailwick/internal/workspacesetup"
)

func (a *app) workspaceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workspace",
		Short: "Group related repos: each project sees the others read-only",
		Long: `A workspace is a folder of related repos (an API, a web app, shared docs). Each project in it
also sees the other members read-only, with secrets hidden, and its sessions can pass requests
to each other through a shared handoff folder.`,
	}
	cmd.AddCommand(a.workspaceAddCmd(), a.workspaceLsCmd(), a.workspaceShowCmd(), a.workspaceEditCmd(), a.workspaceRenameCmd(), a.workspaceRmCmd())
	return cmd
}

func (a *app) workspaceAddCmd() *cobra.Command {
	var yes bool
	var given func() map[string]any
	cmd := &cobra.Command{
		Use:   "add <name> [root]",
		Short: "Create a workspace from a folder of repos and add its projects to it",
		Long: `Create a workspace from a folder of repos, step by step:
  1. Members: the folders each project sees read-only (default: the git repos under the root).
  2. Projects: the projects under the root that join (default: those in no workspace).
  3. Sharing: the handoff folder (default .handoff), workspace instructions (a CLAUDE.md at the
     root), more hiding rules.
The root defaults to the current project's parent folder. Every setting also has a flag; --yes
takes the prefilled answers without asking.`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := "."
			if len(args) == 2 {
				root = args[1]
			} else if p, _, err := a.target(); err == nil {
				if proj, err := a.store.Project(p); err == nil {
					root = filepath.Dir(proj.Dir)
				}
			}
			return a.workspaceAdd(args[0], root, given(), yes)
		},
	}
	given = setupFlags(cmd, workspacesetup.Form)
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "take the prefilled settings without asking")
	return cmd
}

func (a *app) workspaceAdd(name, root string, given map[string]any, yes bool) error {
	d, err := workspacesetup.New(a.store, root)
	if err != nil {
		return err
	}
	given["name"] = name
	if err := d.SetAll(given); err != nil {
		return err
	}
	if a.ui.Interactive && !yes && !a.plain {
		if err := askSetup(a, workspacesetup.Form, d, given); err != nil {
			return err
		}
	}
	changed, err := d.Save(a.store)
	if err != nil {
		return err
	}
	a.ui.OK("created workspace %s (%s)", name, paths.Tilde(a.store.WorkspaceFile(name)))
	for _, p := range changed {
		a.ui.OK("project %s joined %s; apply with: jw restart -p %s", p, name, p)
	}
	return nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func (a *app) workspaceLsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List workspaces",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ps, _ := a.store.Projects()
			var rows [][]string
			for _, n := range a.store.Workspaces() {
				w, err := a.store.Workspace(n)
				if err != nil {
					a.ui.Warn("%v", err)
					continue
				}
				var members, projects []string
				for _, m := range w.Members {
					members = append(members, m.Name)
				}
				for _, p := range ps {
					if p.Workspace == n {
						projects = append(projects, p.Name)
					}
				}
				rows = append(rows, []string{n, paths.Tilde(w.Root), strings.Join(members, " "), strings.Join(projects, " ")})
			}
			if len(rows) == 0 {
				a.ui.Println("No workspaces. Create one with: jw workspace add <name> <root>")
				return nil
			}
			a.ui.Table([]string{"WORKSPACE", "ROOT", "MEMBERS", "PROJECTS"}, rows)
			return nil
		},
	}
}

func (a *app) workspaceName(args []string) (string, error) {
	if len(args) == 1 {
		return args[0], nil
	}
	p, _, err := a.target()
	if err != nil {
		return "", err
	}
	proj, err := a.store.Project(p)
	if err != nil {
		return "", err
	}
	if proj.Workspace == "" {
		return "", fmt.Errorf("%s isn't in a workspace", p)
	}
	return proj.Workspace, nil
}

func (a *app) workspaceShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show [name]",
		Short: "Print a workspace's settings",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			n, err := a.workspaceName(args)
			if err != nil {
				return err
			}
			w, err := a.store.Workspace(n)
			if err != nil {
				return err
			}
			return a.printYAML(w)
		},
	}
}

func (a *app) workspaceEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit [name]",
		Short: "Edit a workspace's file in $EDITOR",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			n, err := a.workspaceName(args)
			if err != nil {
				return err
			}
			return a.editValidated(a.store.WorkspaceFile(n), func() error {
				w, err := a.store.Workspace(n)
				if err != nil {
					return err
				}
				return w.Validate()
			})
		},
	}
}

func (a *app) workspaceRenameCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rename <name> <new-name>",
		Short: "Rename a workspace (its projects and notes move along)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := workspacesetup.Load(a.store, args[0])
			if err != nil {
				return err
			}
			if err := d.SetAll(map[string]any{"name": args[1]}); err != nil {
				return err
			}
			changed, err := d.Save(a.store)
			if err != nil {
				return err
			}
			a.ui.OK("renamed workspace %s to %s", args[0], args[1])
			if len(changed) > 0 {
				a.ui.Step("restart %s to apply it (jw restart -p <name>)", strings.Join(changed, ", "))
			}
			return nil
		},
	}
}

func (a *app) workspaceRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <name>",
		Short: "Remove a workspace (its projects leave it; repos and the handoff folder are kept)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			n := args[0]
			left, err := workspacesetup.Remove(a.store, n)
			for _, p := range left {
				a.ui.Step("%s left %s (applies at jw restart)", p, n)
			}
			if err != nil {
				return err
			}
			a.ui.OK("removed workspace %s", n)
			return nil
		},
	}
}

// workspaceResult generates the current project's workspace view (nothing is started).
func (a *app) workspaceResult() (*workspace.Result, string, error) {
	p, _, err := a.target()
	if err != nil {
		return nil, "", err
	}
	proj, err := a.store.Project(p)
	if err != nil {
		return nil, "", err
	}
	if proj.Workspace == "" {
		return nil, p, fmt.Errorf("%s isn't in a workspace", p)
	}
	proj.Dir = paths.Real(proj.Dir)
	r, err := workspace.Generate(a.store, proj, filepath.Join(paths.StateDir(), "mask"))
	return r, p, err
}

func (a *app) hiddenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "hidden",
		Short: "List the paths hidden in the workspace members (as of now; applied at restart)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			r, p, err := a.workspaceResult()
			if err != nil {
				return err
			}
			if len(r.Info.Hidden) == 0 {
				a.ui.Printf("Nothing hidden in %s's members.\n", p)
				return nil
			}
			a.ui.Printf("Hidden in %s's members (under %s):\n", p, paths.Tilde(r.Info.Root))
			for _, h := range r.Info.Hidden {
				a.ui.Println("  " + strings.TrimPrefix(h, r.Info.Root+"/"))
			}
			a.ui.Println(a.ui.Dim(fmt.Sprintf("%d paths. Rules: %s", len(r.Info.Hidden), strings.Join(r.Rules, " "))))
			return nil
		},
	}
}

func (a *app) handoffsCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "handoffs",
		Short: "List requests between the workspace's sessions (open first; --all includes done)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			r, _, err := a.workspaceResult()
			if err != nil {
				return err
			}
			if r.Info.Handoff == "" {
				return errors.New("this workspace has no handoff folder (set handoff: .handoff with jw workspace edit)")
			}
			hs, err := workspace.ListHandoffs(r.Info.Handoff)
			if err != nil {
				return err
			}
			var rows [][]string
			for _, h := range hs {
				if h.Done && !all {
					continue
				}
				status := h.Status
				switch status {
				case "open":
					status = a.ui.Yellow(status)
				case "answered":
					status = a.ui.Green(status)
				}
				rows = append(rows, []string{status, h.From, h.Inbox, h.Subject, h.Created})
			}
			if len(rows) == 0 {
				a.ui.Printf("No open handoffs in %s.\n", paths.Tilde(r.Info.Handoff))
				return nil
			}
			a.ui.Table([]string{"STATUS", "FROM", "TO", "SUBJECT", "CREATED"}, rows)
			a.ui.Println(a.ui.Dim("Files: " + paths.Tilde(r.Info.Handoff)))
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "include done requests")
	return cmd
}
