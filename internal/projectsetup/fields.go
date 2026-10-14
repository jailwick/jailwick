package projectsetup

import (
	"fmt"
	"github.com/jailwick/jailwick/internal/setup"
	"maps"
	"slices"
	"strings"

	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/presets"
	"github.com/jailwick/jailwick/internal/stacks"
)

// The setup's types, from the shared engine.
type (
	Field    = setup.Field[*Draft]
	Option   = setup.Option
	StepView = setup.StepView
)

const (
	Text   = setup.Text
	Secret = setup.Secret
	Multi  = setup.Multi
	List   = setup.List
)

// what names the setup in problems.
const what = "new project"

// Form is the guided flow. A review of the resulting file always follows. Adding a setting means
// adding a Field here: the CLI prompt and flag, the API schema and the UI form all follow from it.
var Form = setup.Form[*Draft]{What: what, Steps: steps(), Clone: func(d *Draft) *Draft { c := *d; return &c }}

func steps() []setup.Step[*Draft] {
	return []setup.Step[*Draft]{
		{ID: "project", Title: "Project", Fields: []Field{
			text("name", "name", "Project name",
				"Lowercase letters, digits and dashes. Used in commands, e.g. jw claude -p <name>.",
				func(p *config.Project) *string { return &p.Name }).
				CheckedBy(config.ValidName),
			workspaceField(),
			workspaceFolderField(),
		}},
		{ID: "services", Title: "Services and database logins", Fields: []Field{
			envFile(),
			multi("services", "services", "Services", "Started next to the sandbox. Detected ones are selected.",
				setup.Fixed[*Draft](config.ServiceNames...), func(p *config.Project) *[]string { return &p.Services }),
			text("postgres.db", "postgres-db", "Postgres database", "Default: app.",
				func(p *config.Project) *string { return &p.Postgres.DB }).When("services", "postgres"),
			text("postgres.user", "postgres-user", "Postgres user", "Default: app.",
				func(p *config.Project) *string { return &p.Postgres.User }).When("services", "postgres"),
			text("postgres.password", "postgres-password", "Postgres password", "Default: app.",
				func(p *config.Project) *string { return &p.Postgres.Password }).AsSecret().When("services", "postgres"),
			redisPassword().When("services", "redis"),
			text("mongo.user", "mongo-user", "MongoDB user", "Default: app.",
				func(p *config.Project) *string { return &p.Mongo.User }).When("services", "mongo"),
			text("mongo.password", "mongo-password", "MongoDB password", "Default: app.",
				func(p *config.Project) *string { return &p.Mongo.Password }).AsSecret().When("services", "mongo"),
			list("local_forwards", "forward", "Local forwards",
				"localhost:<port> inside the sandbox reaches a service, so env files work unchanged.",
				func(p *config.Project) *[]string { return &p.LocalForwards }).
				CheckedBy(checkForward).WithPlaceholder("5433:postgres:5432").When("services", ""),
		}},
		{ID: "image", Title: "Image", Fields: imageFields()},
		{ID: "toolset", Title: "Toolset", Fields: toolsetFields()},
		{ID: "network", Title: "Network access", Fields: []Field{
			multi("allow_presets", "presets", "Allowlist presets",
				"Groups of sites the sandbox may reach. The ones the repo needs are ticked. "+
					"jw allow adds single sites later.",
				presetOptions, func(p *config.Project) *[]string { return &p.AllowPresets }).WithPlaceholder("npm, pypi").
				NotedBy(networkNote),
		}},
		{ID: "worktrees", Title: "Worktrees", Fields: []Field{
			list("worktree.copy_files", "copy", "Files copied into new worktrees",
				"Git-ignored files a worktree needs, like .env. Database names in them are rewritten.",
				func(p *config.Project) *[]string { return &p.Worktree.CopyFiles }).WithPlaceholder(".env"),
			list("worktree.setup", "setup", "Worktree setup commands", "Run in a new worktree after it is created.",
				func(p *config.Project) *[]string { return &p.Worktree.Setup }).WithPlaceholder("pnpm install"),
			text("run", "run", "Run command",
				"Starts the app from the Run button: in a worktree with that worktree's ports ($PORT) and databases.",
				func(p *config.Project) *string { return &p.Run }).WithPlaceholder("pnpm dev"),
		}},
	}
}

// ---- field constructors: the engine's, on the draft's project ----

func text(key, flag, label, help string, ptr func(*config.Project) *string) Field {
	return setup.TextField(key, flag, label, help, func(d *Draft) *string { return ptr(&d.Project) })
}

func list(key, flag, label, help string, ptr func(*config.Project) *[]string) Field {
	return setup.ListField(key, flag, label, help, func(d *Draft) *[]string { return ptr(&d.Project) })
}

func multi(key, flag, label, help string, options func(*Draft) []Option, ptr func(*config.Project) *[]string) Field {
	return setup.MultiField(key, flag, label, help, options, func(d *Draft) *[]string { return ptr(&d.Project) })
}

// envFile reads the logins of an env file of the app into the service fields, so the sandbox's
// databases take the logins the app already uses.
func envFile() Field {
	return Field{
		Key: "env_file", Flag: "env-file", Label: "Read logins from", Kind: Text, Placeholder: ".env",
		Help: "An env file of the app. Its database logins become the sandbox's, so the app connects unchanged.",
		Resets: []string{"services", "postgres.db", "postgres.user", "postgres.password", "redis.password",
			"mongo.user", "mongo.password", "local_forwards"},
		OptionsOf: func(d *Draft) []Option {
			out := make([]Option, len(d.Detected.EnvCandidates))
			for i, f := range d.Detected.EnvCandidates {
				out[i] = Option{Value: f}
			}
			return out
		},
		Get: func(d *Draft) any { return d.EnvFile },
		Set: func(d *Draft, v any) error { return d.UseEnvFile(v.(string)) },
	}
}

// workspaceField joins a workspace whose root holds the repo: ticked when there is one.
func workspaceField() Field {
	return Field{
		Key: "workspace", Flag: "workspace", Label: "Workspace", Kind: Multi,
		Help: "The repo is in this workspace's folder: in it, the project sees the related repos read-only.",
		OptionsOf: func(d *Draft) []Option {
			if d == nil {
				return nil
			}
			out := make([]Option, len(d.Workspaces))
			for i, n := range d.Workspaces {
				out[i] = Option{Value: n}
			}
			return out
		},
		Get: func(d *Draft) any {
			if d.Project.Workspace == "" {
				return []string{}
			}
			return []string{d.Project.Workspace}
		},
		Set: func(d *Draft, v any) error {
			switch l := v.([]string); len(l) {
			case 0:
				d.Project.Workspace = ""
			case 1:
				d.Project.Workspace = l[0]
			default:
				return fmt.Errorf("one workspace at most")
			}
			return nil
		},
	}
}

// Answers of workspaceFolderField.
const (
	FolderShow = "show"
	FolderHide = "hide"
)

// workspaceFolderField: whether the repo's folder becomes a member of the workspace it joins, so
// the workspace's other projects see it (read-only), as it sees theirs. Some repos shouldn't be
// seen by the others (a private one, one with data): it can keep to itself.
func workspaceFolderField() Field {
	return Field{
		Key: "workspace.folder", Flag: "workspace-folder", Label: "Its folder in the workspace", Kind: setup.Choice,
		Help: "Whether the workspace's other projects see this repo, as it sees theirs.",
		OptionsOf: func(*Draft) []Option {
			return []Option{
				{Value: FolderShow, Label: "Show it to the other projects", Help: "its folder becomes a member: they see it read-only, after a restart"},
				{Value: FolderHide, Label: "Keep it to itself", Help: "it sees the other projects; they don't see it"},
			}
		},
		NoteOf: func(d *Draft) string {
			if d != nil && d.Shown[d.Project.Workspace] {
				return "Its folder is already a member of " + d.Project.Workspace + ": the other projects see it. Change that in the workspace's Settings."
			}
			return ""
		},
		Get: func(d *Draft) any { return d.Folder },
		Set: func(d *Draft, v any) error { d.Folder = v.(string); return nil },
	}.When("workspace", "")
}

// redisPassword edits the first accepted Redis password, the one used in REDIS_URL.
func redisPassword() Field {
	return Field{
		Key: "redis.password", Flag: "redis-password", Label: "Redis password", Help: "Empty: no password.",
		Kind: Secret,
		Get: func(d *Draft) any {
			if len(d.Project.Redis.Passwords) == 0 {
				return ""
			}
			return d.Project.Redis.Passwords[0]
		},
		Set: func(d *Draft, v any) error {
			// A new project has at most this one password.
			if pw := v.(string); pw == "" {
				d.Project.Redis.Passwords = nil
			} else {
				d.Project.Redis.Passwords = []string{pw}
			}
			return nil
		},
	}
}

// networkNote names the broad presets the repo calls for, which aren't chosen for it, and the
// presets the chosen stacks download from that aren't ticked.
func networkNote(d *Draft) string {
	var notes []string
	if _, broad := presets.Split(d.Detected.Presets); len(broad) > 0 {
		notes = append(notes, presets.BroadHint(broad))
	}
	var missing []string
	for _, c := range stacks.Chosen(d.Project.Image.Stacks) {
		for _, name := range c.Presets {
			if !slices.Contains(d.Project.AllowPresets, name) && !slices.Contains(missing, name) {
				missing = append(missing, name)
			}
		}
	}
	if len(missing) > 0 {
		notes = append(notes, "The chosen stacks download packages through "+strings.Join(missing, ", ")+
			", which aren't ticked: without them installs fail.")
	}
	return strings.Join(notes, " ")
}

// toolsetFields are what the sandbox image has: the stacks, each with its version, Playwright and
// apt packages.
func toolsetFields() []Field {
	fields := []Field{{
		Key: "image.stacks", Flag: "stacks", Label: "Stacks", Kind: Multi,
		Help: "Toolchains in the sandbox image, each a layer of its own. The ones the repo uses are " +
			"ticked, at the version it names.",
		OptionsOf: func(*Draft) []Option {
			var out []Option
			for _, s := range stacks.All() {
				out = append(out, Option{Value: s.Name, Help: s.Description})
			}
			return out
		},
		Get: func(d *Draft) any {
			var names []string
			for _, c := range stacks.Chosen(d.Project.Image.Stacks) {
				names = append(names, c.Name)
			}
			return names
		},
		Set: func(d *Draft, v any) error {
			var chosen map[string]string // a new map: drafts share the old one (Form.Check)
			for _, name := range v.([]string) {
				if chosen == nil {
					chosen = map[string]string{}
				}
				chosen[name] = d.Project.Image.Stacks[name]
			}
			d.Project.Image.Stacks = chosen
			return nil
		},
	}}
	for _, s := range stacks.All() {
		fields = append(fields, Field{
			Key: "image.stacks." + s.Name, Flag: s.Name + "-version", Label: s.Title + " version", Kind: Text,
			Help:        fmt.Sprintf("Empty: %s. For example %s.", s.Default, strings.Join(s.Versions, ", ")),
			Placeholder: s.Default,
			Get:         func(d *Draft) any { return d.Project.Image.Stacks[s.Name] },
			Set: func(d *Draft, v any) error {
				if _, ok := d.Project.Image.Stacks[s.Name]; ok {
					chosen := maps.Clone(d.Project.Image.Stacks)
					chosen[s.Name] = v.(string)
					d.Project.Image.Stacks = chosen
				}
				return nil
			},
			Check: s.Check,
		}.When("image.stacks", s.Name))
	}
	return append(fields,
		multi("image.playwright.browsers", "browsers", "Playwright browsers",
			"For screenshots and browser tests: Playwright's CLI, its MCP server for Claude and these "+
				"browsers. None: no Playwright.",
			setup.Fixed[*Draft](config.Browsers...), func(p *config.Project) *[]string { return &p.Image.Playwright.Browsers }),
		list("image.apt_packages", "apt", "Extra apt packages", "Debian packages installed into the sandbox image.",
			func(p *config.Project) *[]string { return &p.Image.AptPackages }).WithPlaceholder("libvips-dev"),
	)
}

// presetOptions is the preset catalogue, by category; the always-on presets are fixed.
func presetOptions(*Draft) []Option {
	var out []Option
	for _, p := range presets.Catalogue() {
		o := Option{Value: p.Name, Help: p.Description, Group: p.Category, Fixed: p.Always}
		if p.Broad != "" {
			o.Tag, o.Warn = "broad", p.Broad
		}
		out = append(out, o)
	}
	return out
}

func checkForward(f string) error {
	_, _, _, err := config.ParseForward(f)
	return err
}
