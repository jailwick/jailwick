package cli

import (
	"errors"
	"fmt"
	"github.com/jailwick/jailwick/internal/buildinfo"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/hostutil"
	"github.com/jailwick/jailwick/internal/image"
	"github.com/jailwick/jailwick/internal/paths"
	"github.com/jailwick/jailwick/internal/secrets"
)

func (a *app) versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			a.ui.Println("jailwick " + buildinfo.Get().String())
			a.updateNotice(cmd.Context())
		},
	}
}

func (a *app) configCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "config", Short: "Show or edit settings (project, or --global)"}
	var global bool
	show := &cobra.Command{
		Use:   "show",
		Short: "Print the project's settings with defaults filled in (or --global)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if global {
				g, err := a.globalConfig()
				if err != nil {
					return err
				}
				return a.printYAML(g)
			}
			p, _, err := a.target()
			if err != nil {
				return err
			}
			r, err := a.resolvedProject(p)
			if err != nil {
				return err
			}
			return a.printYAML(r)
		},
	}
	edit := &cobra.Command{
		Use:   "edit",
		Short: "Edit the project's file (or --global: config.yaml) in $EDITOR",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if global {
				file := a.store.GlobalFile()
				if !hostutil.Exists(file) {
					if err := a.store.SaveGlobal(config.Global{}); err != nil {
						return err
					}
				}
				return a.editValidated(file, func() error { _, err := a.store.Global(); return err })
			}
			p, _, err := a.target()
			if err != nil {
				return err
			}
			return a.editValidated(a.store.ProjectFile(p), func() error {
				pr, err := a.store.Project(p)
				if err != nil {
					return err
				}
				return pr.Validate()
			})
		},
	}
	path := &cobra.Command{
		Use:   "path",
		Short: "Print jailwick's configuration and state folders",
		Args:  cobra.NoArgs,
		Run: func(*cobra.Command, []string) {
			a.ui.Printf("config  %s\nstate   %s\n", a.store.Dir, paths.StateDir())
		},
	}
	cmd.PersistentFlags().BoolVar(&global, "global", false, "the global config.yaml")
	cmd.AddCommand(show, edit, path)
	return cmd
}

func (a *app) printYAML(v any) error {
	data, err := config.Marshal(v)
	if err != nil {
		return err
	}
	a.ui.Print(string(data))
	return nil
}

func (a *app) secretCmd() *cobra.Command {
	var global bool
	cmd := &cobra.Command{
		Use:   "secret",
		Short: "Set, remove or list secrets (tokens passed to the sandbox, stored outside the YAML)",
		Long: `Secrets are stored in the macOS Keychain (or a 0600 file, or read from environment variables)
and passed to the sandbox as environment variables. Known ones: ` + strings.Join(secrets.Known, ", ") + `.
Others are passed when listed in the project's secret_env. --global sets a value for all projects.`,
	}
	scope := func() (string, error) {
		if global {
			return secrets.Global, nil
		}
		p, _, err := a.target()
		return p, err
	}
	set := &cobra.Command{
		Use:   "set NAME [VALUE]",
		Short: "Set a secret (asks for the value when not given)",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			sc, err := scope()
			if err != nil {
				return err
			}
			s, err := a.secretStore()
			if err != nil {
				return err
			}
			key := strings.ToUpper(args[0])
			val := ""
			if len(args) == 2 {
				val = args[1]
			} else if val, err = a.ui.Secret(key); err != nil {
				return err
			}
			if val == "" {
				return errors.New("empty value")
			}
			if err := s.Set(sc, key, val); err != nil {
				return err
			}
			a.ui.OK("%s set for %s (%s); applies at the next jw restart", key, scopeName(sc), s.Name())
			return nil
		},
	}
	unset := &cobra.Command{
		Use:   "unset NAME",
		Short: "Remove a secret",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sc, err := scope()
			if err != nil {
				return err
			}
			s, err := a.secretStore()
			if err != nil {
				return err
			}
			key := strings.ToUpper(args[0])
			err = s.Delete(sc, key)
			if errors.Is(err, secrets.ErrNotFound) && sc != secrets.Global {
				// Set for all projects, not this one: say so instead of a bare "not set".
				if _, gerr := s.Get(secrets.Global, key); gerr == nil {
					return fmt.Errorf("%s isn't set for %s; it is set for all projects: jw secret unset --global %s", key, sc, key)
				}
			}
			if err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
			a.ui.OK("removed %s from %s", key, scopeName(sc))
			return nil
		},
	}
	ls := &cobra.Command{
		Use:   "ls",
		Short: "List secret names (never values) for the project and global",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := a.secretStore()
			if err != nil {
				return err
			}
			scopes := []string{secrets.Global}
			if p, _, err := a.target(); err == nil {
				scopes = append([]string{p}, scopes...)
			}
			var rows [][]string
			for _, sc := range scopes {
				keys, err := s.Keys(sc)
				if err != nil {
					return err
				}
				for _, k := range keys {
					rows = append(rows, []string{k, scopeName(sc), s.Name()})
				}
			}
			if len(rows) == 0 {
				a.ui.Println("No secrets set. Set one with: jw secret set CLAUDE_CODE_OAUTH_TOKEN")
				return nil
			}
			a.ui.Table([]string{"NAME", "SCOPE", "STORE"}, rows)
			return nil
		},
	}
	cmd.PersistentFlags().BoolVar(&global, "global", false, "for all projects")
	cmd.AddCommand(set, unset, ls)
	return cmd
}

func scopeName(s string) string {
	if s == secrets.Global {
		return "all projects"
	}
	return s
}

func (a *app) notesCmd() *cobra.Command {
	var workspace bool
	cmd := &cobra.Command{
		Use:   "notes",
		Short: "Edit the project's notes for Claude (or --workspace: the workspace's)",
		Long: `Notes tell Claude how to run and test this project inside the sandbox. They are read at the
start of each session, so edits apply to the next session without a restart.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, _, err := a.target()
			if err != nil {
				return err
			}
			file := filepath.Join(a.store.ProjectNotesDir(p), "notes.md")
			tmpl := fmt.Sprintf(projectNotesTemplate, p)
			if workspace {
				proj, err := a.store.Project(p)
				if err != nil {
					return err
				}
				if proj.Workspace == "" {
					return fmt.Errorf("%s isn't in a workspace", p)
				}
				file = filepath.Join(a.store.WorkspaceNotesDir(proj.Workspace), "notes.md")
				tmpl = fmt.Sprintf(workspaceNotesTemplate, proj.Workspace)
			}
			if !hostutil.Exists(file) {
				if err := config.WriteFileAtomic(file, []byte(tmpl), 0o644); err != nil {
					return err
				}
			}
			return hostutil.Edit(file)
		},
	}
	cmd.Flags().BoolVar(&workspace, "workspace", false, "the workspace's notes")
	return cmd
}

const projectNotesTemplate = `# %s

## Services
<!-- What runs here and how the app connects (env files, ports). -->

## Run
<!-- Commands to install and start the app. Worktrees: PORT=$PORT … -->

## Checks and tests
<!-- Typecheck, lint, test commands. -->
`

const workspaceNotesTemplate = `# Workspace %s

## Repos
<!-- What each member repo is. -->

## How they connect
<!-- Which repo calls which, shared contracts. -->
`

func (a *app) imageCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "image", Short: "Build, update or clean up sandbox images"}
	var fresh bool
	build := &cobra.Command{
		Use:   "build",
		Short: "Rebuild the project's images (base and project layer)",
		Long: `Rebuild the project's images. Steps that didn't change come from the build cache;
--fresh runs every step again and pulls the images they start from, so base packages pick up
security updates.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.buildImages(cmd, image.Options{Rebuild: true, Fresh: fresh})
		},
	}
	build.Flags().BoolVar(&fresh, "fresh", false, "no build cache, and pull the images built from again")
	update := &cobra.Command{
		Use:   "update",
		Short: "Rebuild the project layer when a newer Claude Code is out",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.buildImages(cmd, image.Options{Update: true})
		},
	}
	ls := &cobra.Command{
		Use:   "ls",
		Short: "List images built by jailwick",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			m, err := a.manager(cmd.Context(), "")
			if err != nil {
				return err
			}
			uses, err := m.Images(cmd.Context())
			if err != nil {
				return err
			}
			var rows [][]string
			for _, u := range uses {
				rows = append(rows, []string{u.Ref, u.Size, u.Created, imageUsers(u)})
			}
			a.ui.Table([]string{"IMAGE", "SIZE", "CREATED", "USED BY"}, rows)
			return nil
		},
	}
	prune := &cobra.Command{
		Use:   "prune",
		Short: "Free disk: remove the images jailwick built that nothing uses",
		Long: `Remove the images jailwick built that no project uses any more (older settings, earlier
Claude Code versions) and no container is made from. Images in use, other images and volumes are
kept, and so is the runtime's build cache, which makes rebuilds fast: docker builder prune frees it.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			m, err := a.manager(cmd.Context(), "")
			if err != nil {
				return err
			}
			removed, err := m.PruneImages(cmd.Context())
			for _, u := range removed {
				a.ui.OK("removed %s (%s)", u.Ref, u.Size)
			}
			if err != nil {
				return err
			}
			if len(removed) == 0 {
				a.ui.OK("nothing to remove: every image is in use")
			}
			return nil
		},
	}
	cmd.AddCommand(build, update, ls, prune, a.imageCheckCmd(), a.imageDefaultCmd())
	return cmd
}

// imageCheckCmd tries an image for jailwick: the project's, or one named.
func (a *app) imageCheckCmd() *cobra.Command {
	var o image.CheckOptions
	cmd := &cobra.Command{
		Use:   "check [image]",
		Short: "Check that an image can be a sandbox's base (default: this project's)",
		Long: `Check an image before a build fails on it: Debian or Ubuntu (apt), glibc, amd64 or arm64,
the sandbox user and its home, and for an image used as is (--as-is) Claude Code and git. The
image is pulled first. Without an image, the project's own is checked (its base, or its image
used as is; a Dockerfile is checked when it builds).`,
		Example: "  jw image check\n  jw image check eclipse-temurin:21-jdk --user app --home /home/app",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if len(args) == 1 {
				o.Ref = args[0]
			} else {
				p, _, err := a.target()
				if err != nil {
					return err
				}
				proj, err := a.store.Project(p)
				if err != nil {
					return err
				}
				g, _ := a.store.Global()
				img := proj.Resolved(g).Image
				switch {
				case img.Name != "":
					o.Ref, o.AsIs = img.Name, true
				case img.Base != "":
					o.Ref = img.Base
				case img.Dockerfile != "":
					return fmt.Errorf("%s builds its Dockerfile (%s): jw image build tries it", p, img.Dockerfile)
				default:
					a.ui.OK("%s builds on jailwick's base: nothing to check", p)
					return nil
				}
				if !cmd.Flags().Changed("user") {
					o.User = img.User
				}
				if !cmd.Flags().Changed("home") {
					o.Home = img.Home
				}
				if !cmd.Flags().Changed("platform") {
					o.Platform = img.Platform
				}
			}
			for _, err := range []error{config.CheckImageRef(o.Ref), config.CheckUserName(o.User), config.CheckHome(o.Home)} {
				if err != nil {
					return err
				}
			}
			m, err := a.manager(ctx, "")
			if err != nil {
				return err
			}
			a.ui.Println(a.ui.Dim("checking " + o.Ref + " (pulled first: a large image takes a while)"))
			found, err := m.CheckImage(ctx, o)
			if err != nil {
				return err
			}
			ok := true
			for _, f := range found {
				if f.OK {
					a.ui.OK("%-6s %s", f.Name, f.Detail)
				} else {
					ok = false
					a.ui.Println(a.ui.Red("✗") + " " + fmt.Sprintf("%-6s %s", f.Name, f.Detail))
				}
			}
			if !ok {
				return fmt.Errorf("%s can't be used as it is: see above", o.Ref)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&o.User, "user", "", "the sandbox user (default node)")
	cmd.Flags().StringVar(&o.Home, "home", "", "its home (default /home/<user>)")
	cmd.Flags().BoolVar(&o.AsIs, "as-is", false, "the image is used as is: Claude Code and git must be in it")
	cmd.Flags().StringVar(&o.Platform, "platform", "", "linux/amd64 or linux/arm64 (default: this machine's)")
	return cmd
}

// imageDefaultCmd shows or sets the image every project builds on unless it names its own.
func (a *app) imageDefaultCmd() *cobra.Command {
	var user, home string
	var reset bool
	cmd := &cobra.Command{
		Use:   "default [image]",
		Short: "Show or set the image projects build on unless they name their own",
		Long: `Show or set the default base image (config.yaml's image): a Debian or Ubuntu image with the
sandbox user, which jailwick builds its layer on for every project that names no image of its own.
--reset goes back to jailwick's base. Projects pick it up at their next start or restart.`,
		Example: "  jw image default\n  jw image default eclipse-temurin:21-jdk --user app --home /home/app\n  jw image default --reset",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			g, err := a.store.Global()
			if err != nil {
				return err
			}
			if len(args) == 0 && !reset {
				if g.Image.Base == "" && g.Image.Dockerfile == "" && g.Image.Name == "" {
					a.ui.Println("jailwick's base (no default image set)")
					return nil
				}
				for _, kv := range [][2]string{{"base", g.Image.Base}, {"dockerfile", g.Image.Dockerfile}, {"name", g.Image.Name},
					{"user", g.Image.User}, {"home", g.Image.Home}} {
					if kv[1] != "" {
						a.ui.Println(fmt.Sprintf("%-10s %s", kv[0], kv[1]))
					}
				}
				return nil
			}
			if reset {
				g.Image.Base, g.Image.Dockerfile, g.Image.Context, g.Image.Name, g.Image.User, g.Image.Home = "", "", "", "", "", ""
			} else {
				g.Image.Base, g.Image.Dockerfile, g.Image.Context, g.Image.Name = args[0], "", "", ""
				g.Image.User, g.Image.Home = user, home
			}
			if err := g.Validate(); err != nil {
				return err
			}
			if err := a.store.SaveGlobal(g); err != nil {
				return err
			}
			if reset {
				a.ui.OK("projects build on jailwick's base")
			} else {
				a.ui.OK("projects build on %s unless they name their own; jw image check %s tries it", args[0], args[0])
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&user, "user", "", "the sandbox user in the image (default node)")
	cmd.Flags().StringVar(&home, "home", "", "its home (default /home/node)")
	cmd.Flags().BoolVar(&reset, "reset", false, "go back to jailwick's base")
	return cmd
}

// imageUsers says what keeps an image: projects, a container, or nothing.
func imageUsers(u image.Use) string {
	switch {
	case len(u.Projects) > 0:
		return strings.Join(u.Projects, ", ")
	case u.Containers:
		return "a container"
	}
	return "unused"
}

func (a *app) buildImages(cmd *cobra.Command, o image.Options) error {
	m, p, _, err := a.needProject(cmd.Context())
	if err != nil {
		return err
	}
	if m.Out == nil {
		m.Out = os.Stderr // builds are long: show their output
	}
	prep, err := m.Prepare(cmd.Context(), p, o)
	if err != nil {
		return err
	}
	a.ui.OK("images ready: %s", prep.Images.Sandbox)
	st, err := m.Status(cmd.Context(), p)
	if err == nil {
		a.showPending(st)
	}
	return nil
}

func (a *app) aliasCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "alias", Short: "Add or remove the short command jw"}
	add := &cobra.Command{
		Use:   "add [name]",
		Short: "Add a short command (default jw) next to the jailwick binary",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := "jw"
			if len(args) == 1 {
				name = args[0]
			}
			link, err := addAlias(name)
			if err != nil {
				return err
			}
			a.ui.OK("%s now runs jailwick (%s)", name, paths.Tilde(link))
			return nil
		},
	}
	rm := &cobra.Command{
		Use:   "remove [name]",
		Short: "Remove the short command",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := "jw"
			if len(args) == 1 {
				name = args[0]
			}
			if err := removeAlias(name); err != nil {
				return err
			}
			a.ui.OK("removed %s", name)
			return nil
		},
	}
	cmd.AddCommand(add, rm)
	return cmd
}

// addAlias creates name as a symlink to the jailwick binary, in the binary's folder or, if that
// isn't writable, ~/.local/bin.
func addAlias(name string) (string, error) {
	if strings.ContainsAny(name, "/ ") || name == "" {
		return "", fmt.Errorf("invalid name %q", name)
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	exe, _ = filepath.EvalSymlinks(exe)
	for _, dir := range []string{filepath.Dir(exe), filepath.Join(paths.Home(), ".local", "bin")} {
		link := filepath.Join(dir, name)
		if target, err := os.Readlink(link); err == nil {
			if target == exe {
				return link, nil
			}
			return "", fmt.Errorf("%s already exists and points elsewhere (%s)", link, target)
		}
		if hostutil.Exists(link) {
			return "", fmt.Errorf("%s already exists", link)
		}
		_ = os.MkdirAll(dir, 0o755)
		if err := os.Symlink(exe, link); err == nil {
			return link, nil
		}
	}
	return "", errors.New("couldn't create the alias next to jailwick or in ~/.local/bin")
}

func removeAlias(name string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, _ = filepath.EvalSymlinks(exe)
	for _, dir := range []string{filepath.Dir(exe), filepath.Join(paths.Home(), ".local", "bin")} {
		link := filepath.Join(dir, name)
		if target, err := os.Readlink(link); err == nil && target == exe {
			return os.Remove(link)
		}
	}
	return fmt.Errorf("no %s alias for jailwick found", name)
}

func (a *app) whereCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "where",
		Short: "Show the project, worktree and workspace for the current folder",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			a.ui.Printf("config     %s\n", paths.Tilde(a.store.Dir))
			p, wt, err := a.target()
			if err != nil {
				a.ui.Println("project    (none for this folder)")
				return nil
			}
			proj, err := a.store.Project(p)
			if err != nil {
				return err
			}
			a.ui.Printf("project    %s  %s\n", p, a.ui.Dim(paths.Tilde(proj.Dir)))
			if wt != "" {
				a.ui.Printf("worktree   %s\n", wt)
			} else {
				a.ui.Println("worktree   (main checkout)")
			}
			if proj.Workspace != "" {
				a.ui.Printf("workspace  %s\n", proj.Workspace)
			}
			return nil
		},
	}
}

func (a *app) pathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path [worktree]",
		Short: "Print the project's folder (or a worktree's)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, _, err := a.target()
			if err != nil {
				return err
			}
			proj, err := a.store.Project(p)
			if err != nil {
				return err
			}
			dir := paths.Real(proj.Dir)
			if len(args) == 1 {
				dir = filepath.Join(dir, ".claude", "worktrees", strings.ReplaceAll(args[0], "/", "-"))
			}
			a.ui.Println(dir)
			return nil
		},
	}
}
