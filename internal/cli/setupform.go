package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/jailwick/jailwick/internal/setup"
	"github.com/jailwick/jailwick/internal/ui"
)

// setupFlags adds a flag for each field that has one. The returned function gives the values of
// the flags that were set, by field key.
func setupFlags[D any](cmd *cobra.Command, form setup.Form[D]) func() map[string]any {
	value := map[string]func() any{}
	for _, f := range form.Fields() {
		if f.Flag == "" {
			continue
		}
		if f.IsList() {
			v := cmd.Flags().StringSlice(f.Flag, nil, flagUsage(f))
			value[f.Key] = func() any { return *v }
		} else {
			v := cmd.Flags().String(f.Flag, "", flagUsage(f))
			value[f.Key] = func() any { return *v }
		}
	}
	return func() map[string]any {
		given := map[string]any{}
		for _, f := range form.Fields() {
			if get, ok := value[f.Key]; ok && cmd.Flags().Changed(f.Flag) {
				given[f.Key] = get()
			}
		}
		return given
	}
}

// flagUsage describes a setup field as a flag.
func flagUsage[D any](f setup.Field[D]) string {
	u := strings.ToLower(f.Label)
	var none D
	switch {
	case f.Kind == setup.Multi && len(f.Options(none)) > 8:
		u += ", e.g. " + f.Placeholder
	case (f.Kind == setup.Multi || f.Kind == setup.Choice) && len(f.Options(none)) > 0:
		var vals []string
		for _, o := range f.Options(none) {
			vals = append(vals, o.Value)
		}
		u += ": " + strings.Join(vals, ", ")
	case f.IsList() && f.Placeholder != "":
		u += ", e.g. " + f.Placeholder + " (repeatable)"
	case f.IsList():
		u += " (repeatable)"
	}
	return u
}

// optionLabel is a choice as the prompt shows it: its name, what it is, and a warning.
func optionLabel(o setup.Option) string {
	l := o.Value
	if o.Help != "" {
		l += ": " + o.Help
	}
	switch {
	case o.Fixed:
		l += " (always)"
	case o.Tag != "" && o.Warn != "":
		l += " (" + o.Tag + ": " + o.Warn + ")"
	case o.Warn != "":
		l += " (" + o.Warn + ")"
	}
	return l
}

// askSetup prompts, step by step, for each visible field not given as a flag.
func askSetup[D any](a *app, form setup.Form[D], d D, given map[string]any) error {
	for _, st := range form.Steps {
		a.ui.Println(a.ui.Bold(st.Title))
		for _, f := range st.Fields {
			if _, ok := given[f.Key]; ok || !form.Visible(d, f) {
				continue
			}
			if err := askField(a, form, d, f); err != nil {
				return err
			}
		}
	}
	return nil
}

// askField prompts for one setting, prefilled with the draft's value and checked as it's typed.
func askField[D any](a *app, form setup.Form[D], d D, f setup.Field[D]) error {
	if f.Help != "" {
		a.ui.Hint("%s", f.Help)
	}
	if n := f.Note(d); n != "" {
		a.ui.Warn("%s", n)
	}
	check := func(s string) error { return form.Check(d, f, s) }
	var v any
	var err error
	switch cur := f.Get(d).(type) {
	case []string:
		if f.Kind == setup.Multi {
			var opts []ui.Option
			for _, o := range f.Options(d) {
				if !o.Disabled {
					opts = append(opts, ui.Option{Label: optionLabel(o), Value: o.Value})
				}
			}
			v, err = a.ui.MultiSelect(f.Label, opts, cur)
			break
		}
		v, err = a.ui.Input(f.Label+" (comma-separated)", f.Placeholder, strings.Join(cur, ", "), check)
	case string:
		if f.Kind == setup.Choice {
			var opts []ui.Option
			for _, o := range f.Options(d) {
				if !o.Disabled {
					opts = append(opts, ui.Option{Label: optionLabel(o), Value: o.Value})
				}
			}
			v, err = a.ui.Select(f.Label, opts, cur)
			break
		}
		v, err = a.ui.Input(f.Label, f.Placeholder, cur, check)
	}
	if err != nil {
		return err
	}
	return form.Set(d, f, v)
}
