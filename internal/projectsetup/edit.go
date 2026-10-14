package projectsetup

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/detect"
	"github.com/jailwick/jailwick/internal/setup"
)

// EditForm changes an existing project with the setup's own fields: every step but the first
// (the name, which names its files and containers, and the workspace, which the project page
// joins and leaves), and its resources.
var EditForm = setup.Form[*Draft]{
	What:  "project settings",
	Steps: append(steps()[1:], setup.Step[*Draft]{ID: "resources", Title: "Resources", Fields: resourceFields()}),
	Clone: Form.Clone,
}

// Edit starts a draft of an existing project and sets the values on it. The error is from
// setting the values or checking the result: problems for the form to show.
func Edit(store Store, name string, values map[string]any) (*Draft, error) {
	p, err := store.Project(name)
	if err != nil {
		return nil, err
	}
	d := &Draft{Project: p}
	d.Detected.Languages = detect.Languages(p.Dir) // for the image's note
	if err := EditForm.SetAll(d, values); err != nil {
		return d, err
	}
	tidy(&d.Project)
	if err := d.checkImage(); err != nil {
		return d, err
	}
	if err := d.Project.Validate(); err != nil {
		return d, err
	}
	return d, checkWorkspace(store, d.Project)
}

var (
	memoryRE = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?[kmgKMG]?$`)
	cpusRE   = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)
)

// resourceFields are what the sandbox may use. Empty takes jailwick's default (config/defaults.go).
func resourceFields() []Field {
	return []Field{
		text("resources.memory", "memory", "Memory", "Empty: 12g. Like 8g or 16g; services take their own.",
			func(p *config.Project) *string { return &p.Resources.Memory }).WithPlaceholder("12g").
			CheckedBy(func(v string) error {
				if v != "" && !memoryRE.MatchString(v) { // empty: the default
					return fmt.Errorf("%q isn't an amount of memory, like 8g or 512m", v)
				}
				return nil
			}),
		text("resources.cpus", "cpus", "CPUs", "Empty: 6. Whole or part CPUs, like 4 or 2.5.",
			func(p *config.Project) *string { return &p.Resources.CPUs }).WithPlaceholder("6").
			CheckedBy(func(v string) error {
				if v == "" { // the default
					return nil
				}
				if n, err := strconv.ParseFloat(v, 64); !cpusRE.MatchString(v) || err != nil || n <= 0 {
					return fmt.Errorf("%q isn't a number of CPUs, like 4 or 2.5", v)
				}
				return nil
			}),
	}
}
