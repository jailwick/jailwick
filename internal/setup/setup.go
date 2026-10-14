// Package setup is the engine of jailwick's guided setups (a new project, a workspace): fields
// declared once become the CLI's prompts and flags, the API's schema and the app's form. A setup
// is a Form over its own draft type D, which the fields read and write.
package setup

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jailwick/jailwick/internal/config"
)

// Kind is how a field is asked and edited. Text and Secret hold a string, Multi and List a []string.
type Kind string

const (
	Text   Kind = "text"   // one line
	Secret Kind = "secret" // one line, masked in the UI
	Multi  Kind = "multi"  // any of the field's options
	List   Kind = "list"   // free list of values
	Choice Kind = "choice" // exactly one of the field's options (a string)
)

// Cond shows a field only while another field's value contains Has (or is not empty, when Has is
// empty); with Not, only while it doesn't. It is plain data so the UI can evaluate it as the user
// edits.
type Cond struct {
	Key string `json:"key"`
	Has string `json:"has,omitempty"`
	Not bool   `json:"not,omitempty"`
}

// Option is one choice of a Multi field (or a suggestion for a Text field).
type Option struct {
	Value    string `json:"value"`
	Label    string `json:"label,omitempty"` // how a Choice shows it ("" : the value)
	Help     string `json:"help,omitempty"`
	Group    string `json:"group,omitempty"`    // options are shown under their group
	Tag      string `json:"tag,omitempty"`      // a short label next to it ("broad")
	Warn     string `json:"warn,omitempty"`     // a reason to think twice before choosing it
	Fixed    bool   `json:"fixed,omitempty"`    // always chosen; it can't be removed
	Disabled bool   `json:"disabled,omitempty"` // can't be chosen (Warn says why)
}

// Field is one answer of a setup. Get and Set read and write it on the draft; the others are
// optional. Options and Note get a nil draft when only the field itself is described (flags).
type Field[D any] struct {
	Key         string // stable id, also the key in API values ("postgres.user")
	Flag        string // CLI flag; empty: none
	Label       string
	Help        string
	Placeholder string
	Kind        Kind
	ShowIf      *Cond
	// Resets are the keys a change to this field recomputes: their earlier answers no longer apply.
	Resets []string

	Get       func(D) any
	Set       func(D, any) error
	OptionsOf func(D) []Option
	NoteOf    func(D) string     // something about this draft worth seeing next to the field
	Check     func(string) error // each value, before it's set
	// Action names something the app can do with the field's value, next to it ("check-image").
	Action string
}

// Options are the choices of the field.
func (f Field[D]) Options(d D) []Option {
	if f.OptionsOf == nil {
		return nil
	}
	return f.OptionsOf(d)
}

// Note is the field's note for this draft ("" for none).
func (f Field[D]) Note(d D) string {
	if f.NoteOf == nil {
		return ""
	}
	return f.NoteOf(d)
}

// IsList reports whether the field holds a []string.
func (f Field[D]) IsList() bool { return f.Kind == Multi || f.Kind == List }

// Chained modifiers, for declaring fields.

// When shows the field only while the field key holds has (see Cond).
func (f Field[D]) When(key, has string) Field[D] { f.ShowIf = &Cond{Key: key, Has: has}; return f }
func (f Field[D]) Unless(key, has string) Field[D] {
	f.ShowIf = &Cond{Key: key, Has: has, Not: true}
	return f
}
func (f Field[D]) WithAction(a string) Field[D]            { f.Action = a; return f }
func (f Field[D]) AsSecret() Field[D]                      { f.Kind = Secret; return f }
func (f Field[D]) WithPlaceholder(s string) Field[D]       { f.Placeholder = s; return f }
func (f Field[D]) CheckedBy(c func(string) error) Field[D] { f.Check = c; return f }
func (f Field[D]) NotedBy(n func(D) string) Field[D]       { f.NoteOf = n; return f }

// Step groups fields under one heading.
type Step[D any] struct {
	ID     string
	Title  string
	Fields []Field[D]
}

// Form is a whole setup: its steps in order. What names it in problems ("new project").
type Form[D any] struct {
	What  string
	Steps []Step[D]
	// Clone copies a draft, so a value can be checked without changing the draft (see Check).
	Clone func(D) D
}

// Verify checks the form's own declaration: unique keys and flags, conditions on known fields,
// options for Multi fields, Get and Set everywhere. Tests call it.
func (fm Form[D]) Verify() error {
	var errs []error
	keys, flags := map[string]bool{}, map[string]bool{}
	for _, f := range fm.Fields() {
		if keys[f.Key] || (f.Flag != "" && flags[f.Flag]) {
			errs = append(errs, fmt.Errorf("duplicate key or flag: %s / %s", f.Key, f.Flag))
		}
		keys[f.Key], flags[f.Flag] = true, true
		if f.ShowIf != nil {
			if _, ok := fm.Field(f.ShowIf.Key); !ok {
				errs = append(errs, fmt.Errorf("%s: show_if refers to unknown %q", f.Key, f.ShowIf.Key))
			}
		}
		if (f.Kind == Multi || f.Kind == Choice) && f.OptionsOf == nil {
			errs = append(errs, fmt.Errorf("%s: multi field without options", f.Key))
		}
		if f.Get == nil || f.Set == nil {
			errs = append(errs, fmt.Errorf("%s: no Get or Set", f.Key))
		}
	}
	if fm.Clone == nil {
		errs = append(errs, errors.New("no Clone"))
	}
	return errors.Join(errs...)
}

// Fields returns every field in order.
func (fm Form[D]) Fields() []Field[D] {
	var out []Field[D]
	for _, s := range fm.Steps {
		out = append(out, s.Fields...)
	}
	return out
}

// Field finds a field by key.
func (fm Form[D]) Field(key string) (Field[D], bool) {
	for _, s := range fm.Steps {
		for _, f := range s.Fields {
			if f.Key == key {
				return f, true
			}
		}
	}
	return Field[D]{}, false
}

// Visible reports whether a field applies, given the other values. A Multi field without options
// has nothing to ask.
func (fm Form[D]) Visible(d D, f Field[D]) bool {
	if f.Kind == Multi && len(f.Options(d)) == 0 {
		return false
	}
	if f.ShowIf == nil {
		return true
	}
	other, ok := fm.Field(f.ShowIf.Key)
	if !ok {
		return true
	}
	return f.ShowIf.holds(other.Get(d)) != f.ShowIf.Not
}

// holds reports whether a value contains Has (or is not empty, when Has is empty).
func (c Cond) holds(value any) bool {
	switch v := value.(type) {
	case []string:
		if c.Has == "" {
			return len(v) > 0
		}
		return slices.Contains(v, c.Has)
	case string:
		if c.Has == "" {
			return v != ""
		}
		return v == c.Has
	}
	return true
}

// Set checks and stores a field's value. It accepts a string or []string (or the []any that JSON
// decodes to); list values are trimmed and empty entries dropped.
func (fm Form[D]) Set(d D, f Field[D], v any) error {
	val, err := normalize(f, v)
	if err != nil {
		return fm.Problem("%s: %v", f.Label, err)
	}
	if f.Check != nil {
		vals, _ := val.([]string)
		if s, ok := val.(string); ok {
			vals = []string{s}
		}
		for _, s := range vals {
			if err := f.Check(s); err != nil {
				return fm.Problem("%s: %v", f.Label, err)
			}
		}
	}
	if f.Kind == Choice {
		if s := val.(string); s != "" && !slices.ContainsFunc(f.Options(d), func(o Option) bool { return o.Value == s }) {
			return fm.Problem("%s: %q is not one of %s", f.Label, s, strings.Join(values(f.Options(d)), ", "))
		}
	}
	if f.Kind == Multi {
		opts := f.Options(d)
		for _, s := range val.([]string) {
			i := slices.IndexFunc(opts, func(o Option) bool { return o.Value == s })
			switch {
			case i < 0:
				return fm.Problem("%s: %q is not one of %s", f.Label, s, strings.Join(values(opts), ", "))
			case opts[i].Disabled && !slices.Contains(asList(f.Get(d)), s):
				return fm.Problem("%s: %s can't be chosen: %s", f.Label, s, opts[i].Warn)
			}
		}
	}
	if err := f.Set(d, val); err != nil {
		return fm.Problem("%s: %v", f.Label, err)
	}
	return nil
}

// Check reports whether Set would accept the value, without changing the draft.
func (fm Form[D]) Check(d D, f Field[D], v any) error {
	return fm.Set(fm.Clone(d), f, v)
}

// SetAll sets values by field key, in the order of the steps.
func (fm Form[D]) SetAll(d D, values map[string]any) error {
	for k := range values {
		if _, ok := fm.Field(k); !ok {
			return fm.Problem("unknown setting %q", k)
		}
	}
	for _, f := range fm.Fields() {
		if v, ok := values[f.Key]; ok {
			if err := fm.Set(d, f, v); err != nil {
				return err
			}
		}
	}
	return nil
}

// Problem is a problem with the answers, for the form to show.
func (fm Form[D]) Problem(format string, a ...any) error {
	return &config.Problems{What: fm.What, List: []string{fmt.Sprintf(format, a...)}}
}

// StepView and FieldView are the steps as the app shows them, with a draft's values.
type StepView struct {
	ID     string      `json:"id"`
	Title  string      `json:"title"`
	Fields []FieldView `json:"fields"`
}

type FieldView struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Help        string   `json:"help,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
	Note        string   `json:"note,omitempty"` // about this draft, e.g. presets detected but not chosen
	Kind        Kind     `json:"kind"`
	Options     []Option `json:"options,omitempty"`
	ShowIf      *Cond    `json:"show_if,omitempty"`
	Resets      []string `json:"resets,omitempty"` // keys a change to this field recomputes
	Action      string   `json:"action,omitempty"` // what the app can do with the value (Field.Action)
	Value       any      `json:"value"`
}

// View describes every step with the draft's current values.
func (fm Form[D]) View(d D) []StepView {
	out := make([]StepView, 0, len(fm.Steps))
	for _, s := range fm.Steps {
		sv := StepView{ID: s.ID, Title: s.Title, Fields: make([]FieldView, 0, len(s.Fields))}
		for _, f := range s.Fields {
			v := f.Get(d)
			if l, ok := v.([]string); ok && l == nil {
				v = []string{}
			}
			sv.Fields = append(sv.Fields, FieldView{
				Key: f.Key, Label: f.Label, Help: f.Help, Placeholder: f.Placeholder, Note: f.Note(d), Kind: f.Kind,
				Options: f.Options(d), ShowIf: f.ShowIf, Resets: f.Resets, Action: f.Action, Value: v,
			})
		}
		out = append(out, sv)
	}
	return out
}

func normalize[D any](f Field[D], v any) (any, error) {
	if !f.IsList() {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("expected text")
		}
		return strings.TrimSpace(s), nil
	}
	var in []string
	switch x := v.(type) {
	case []string:
		in = x
	case []any:
		for _, e := range x {
			s, ok := e.(string)
			if !ok {
				return nil, fmt.Errorf("expected a list of text")
			}
			in = append(in, s)
		}
	case string: // "a, b" from a flag or a single input
		in = strings.Split(x, ",")
	case nil:
	default:
		return nil, fmt.Errorf("expected a list")
	}
	out := []string{}
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" && !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out, nil
}

func values(opts []Option) []string {
	out := make([]string, len(opts))
	for i, o := range opts {
		out[i] = o.Value
	}
	return out
}

func asList(v any) []string {
	l, _ := v.([]string)
	return l
}
