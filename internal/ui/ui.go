// Package ui prints jailwick's output: rich on a terminal (colours, symbols, prompts), plain
// when piped, in CI, with NO_COLOR or --plain.
package ui

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// UI writes messages and asks questions.
type UI struct {
	Out, Err    io.Writer
	Rich        bool // colours and symbols
	Interactive bool // may prompt

	ok, warn, bad, dim, bold, accent lipgloss.Style
}

// ErrNotInteractive is returned when a prompt is needed but there is no terminal.
var ErrNotInteractive = errors.New("needs an answer, but this isn't an interactive terminal (pass the value as a flag)")

// New returns a UI for stdout/stderr.
func New(plain bool) *UI {
	tty := term.IsTerminal(int(os.Stdout.Fd()))
	rich := tty && !plain && os.Getenv("NO_COLOR") == "" && os.Getenv("CI") == "" && os.Getenv("TERM") != "dumb"
	u := &UI{
		Out: os.Stdout, Err: os.Stderr, Rich: rich,
		Interactive: tty && term.IsTerminal(int(os.Stdin.Fd())) && os.Getenv("CI") == "",
	}
	u.ok = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	u.warn = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	u.bad = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	u.dim = lipgloss.NewStyle().Faint(true)
	u.bold = lipgloss.NewStyle().Bold(true)
	u.accent = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	return u
}

func (u *UI) style(s lipgloss.Style, text string) string {
	if !u.Rich {
		return text
	}
	return s.Render(text)
}

// Text styles for composing lines.
func (u *UI) Bold(s string) string   { return u.style(u.bold, s) }
func (u *UI) Dim(s string) string    { return u.style(u.dim, s) }
func (u *UI) Green(s string) string  { return u.style(u.ok, s) }
func (u *UI) Yellow(s string) string { return u.style(u.warn, s) }
func (u *UI) Red(s string) string    { return u.style(u.bad, s) }
func (u *UI) Cyan(s string) string   { return u.style(u.accent, s) }

// Println writes a plain line.
func (u *UI) Println(a ...any) { fmt.Fprintln(u.Out, a...) }

// Printf writes formatted text.
func (u *UI) Printf(format string, a ...any) { fmt.Fprintf(u.Out, format, a...) }

// Step reports progress ("• Building …").
func (u *UI) Step(format string, a ...any) {
	fmt.Fprintln(u.Err, u.Dim(u.sym("•", "-")+" "+fmt.Sprintf(format, a...)))
}

// OK reports success.
func (u *UI) OK(format string, a ...any) {
	fmt.Fprintln(u.Out, u.Green(u.sym("✔", "ok:"))+" "+fmt.Sprintf(format, a...))
}

// Warn reports something to look at.
func (u *UI) Warn(format string, a ...any) {
	fmt.Fprintln(u.Err, u.Yellow(u.sym("!", "warning:"))+" "+fmt.Sprintf(format, a...))
}

// Fail reports an error.
func (u *UI) Fail(err error) {
	msg := err.Error()
	fmt.Fprintln(u.Err, u.Red(u.sym("✘", "error:"))+" "+msg)
}

// Hint prints an indented follow-up line.
func (u *UI) Hint(format string, a ...any) {
	fmt.Fprintln(u.Err, "  "+u.Dim(fmt.Sprintf(format, a...)))
}

func (u *UI) sym(rich, plain string) string {
	if u.Rich {
		return rich
	}
	return plain
}

// Dot is a state marker: ● running, ○ stopped.
func (u *UI) Dot(on bool) string {
	if on {
		return u.Green(u.sym("●", "*"))
	}
	return u.Dim(u.sym("○", "-"))
}

// Table prints aligned columns. Cells may contain styled text.
func (u *UI) Table(header []string, rows [][]string) {
	widths := make([]int, len(header))
	all := append([][]string{header}, rows...)
	for _, r := range all {
		for i, c := range r {
			if i < len(widths) {
				widths[i] = max(widths[i], lipgloss.Width(c))
			}
		}
	}
	for n, r := range all {
		var b strings.Builder
		for i, c := range r {
			if i == len(r)-1 {
				b.WriteString(c)
				break
			}
			b.WriteString(c + strings.Repeat(" ", widths[i]-lipgloss.Width(c)+2))
		}
		line := strings.TrimRight(b.String(), " ")
		if n == 0 {
			line = u.Dim(line)
		}
		fmt.Fprintln(u.Out, line)
	}
}

// Confirm asks a yes/no question.
func (u *UI) Confirm(question string, def bool) (bool, error) {
	if !u.Interactive {
		return false, ErrNotInteractive
	}
	v := def
	err := huh.NewConfirm().Title(question).Value(&v).Run()
	return v, err
}

// Input asks for a line of text.
func (u *UI) Input(title, placeholder, def string, validate func(string) error) (string, error) {
	if !u.Interactive {
		return "", ErrNotInteractive
	}
	v := def
	in := huh.NewInput().Title(title).Placeholder(placeholder).Value(&v)
	if validate != nil {
		in = in.Validate(validate)
	}
	err := in.Run()
	return strings.TrimSpace(v), err
}

// Secret asks for a hidden value.
func (u *UI) Secret(title string) (string, error) {
	if !u.Interactive {
		return "", ErrNotInteractive
	}
	var v string
	err := huh.NewInput().Title(title).EchoMode(huh.EchoModePassword).Value(&v).Run()
	return strings.TrimSpace(v), err
}

// Option is a choice in Select and MultiSelect.
type Option struct{ Label, Value string }

// Select asks for one choice.
func (u *UI) Select(title string, opts []Option, def string) (string, error) {
	if !u.Interactive {
		return "", ErrNotInteractive
	}
	v := def
	err := huh.NewSelect[string]().Title(title).Options(toHuh(opts)...).Value(&v).Filtering(len(opts) > 8).Run()
	return v, err
}

// MultiSelect asks for several choices.
func (u *UI) MultiSelect(title string, opts []Option, selected []string) ([]string, error) {
	if !u.Interactive {
		return nil, ErrNotInteractive
	}
	v := selected
	err := huh.NewMultiSelect[string]().Title(title).Options(toHuh(opts)...).Value(&v).Filterable(len(opts) > 8).Run()
	return v, err
}

func toHuh(opts []Option) []huh.Option[string] {
	out := make([]huh.Option[string], len(opts))
	for i, o := range opts {
		out[i] = huh.NewOption(o.Label, o.Value)
	}
	return out
}

// Print writes text as is.
func (u *UI) Print(s string) { fmt.Fprint(u.Out, s) }
