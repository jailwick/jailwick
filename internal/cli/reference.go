package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/jailwick/jailwick/internal/core"
)

// Reference renders the complete command reference as Markdown (docs/CLI.md), from the same
// command definitions the CLI runs, so the two can't drift apart.
func Reference() string {
	a := &app{env: core.Open()}
	a.store = a.env.Store
	root := a.rootCmd()
	root.Use = "jw"
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()

	var b strings.Builder
	b.WriteString("# Command reference\n\n")
	b.WriteString("Generated from the code by `make docs`; don't edit by hand. `jw` and `jailwick` are the same\n")
	b.WriteString("command. `jw <command> --help` prints the same text in the terminal.\n\n")
	b.WriteString("## Global flags\n\nEvery command accepts these:\n\n")
	writeFlags(&b, root.PersistentFlags())

	b.WriteString("\n## Commands\n\n")
	for _, g := range root.Groups() {
		fmt.Fprintf(&b, "**%s**", strings.TrimSuffix(g.Title, ":"))
		var names []string
		for _, c := range root.Commands() {
			if c.GroupID == g.ID && c.IsAvailableCommand() {
				names = append(names, fmt.Sprintf("[`%s`](#%s)", c.Name(), anchor(c)))
			}
		}
		b.WriteString(" " + strings.Join(names, " · ") + "\n\n")
	}
	for _, g := range root.Groups() {
		fmt.Fprintf(&b, "---\n\n## %s\n\n", strings.TrimSuffix(g.Title, ":"))
		for _, c := range root.Commands() {
			if c.GroupID == g.ID && c.IsAvailableCommand() {
				writeCommand(&b, c, 3)
			}
		}
	}
	b.WriteString("---\n\n## Other\n\n")
	for _, c := range root.Commands() {
		if c.GroupID == "" && c.IsAvailableCommand() {
			writeCommand(&b, c, 3)
		}
	}
	return b.String()
}

func anchor(c *cobra.Command) string {
	return strings.ReplaceAll(strings.TrimPrefix(c.CommandPath(), "jw "), " ", "-")
}

func writeCommand(b *strings.Builder, c *cobra.Command, level int) {
	path := strings.TrimPrefix(c.CommandPath(), "jw ")
	fmt.Fprintf(b, "<a id=\"%s\"></a>\n%s `jw %s`\n\n", anchor(c), strings.Repeat("#", level), path)
	text := c.Long
	if text == "" {
		text = c.Short
	}
	b.WriteString(strings.TrimSpace(text) + "\n\n")
	if c.Runnable() {
		fmt.Fprintf(b, "```\n%s\n```\n\n", c.UseLine())
	}
	if len(c.Aliases) > 0 {
		fmt.Fprintf(b, "Also: `%s`.\n\n", strings.Join(c.Aliases, "`, `"))
	}
	if c.HasAvailableLocalFlags() {
		writeFlags(b, c.LocalNonPersistentFlags())
		b.WriteString("\n")
	}
	for _, sub := range c.Commands() {
		if sub.IsAvailableCommand() {
			writeCommand(b, sub, min(level+1, 5))
		}
	}
}

func writeFlags(b *strings.Builder, fs *pflag.FlagSet) {
	b.WriteString("| Flag | Default | Meaning |\n|---|---|---|\n")
	fs.VisitAll(func(f *pflag.Flag) {
		if f.Hidden || f.Name == "help" {
			return
		}
		name := "`--" + f.Name + "`"
		if f.Shorthand != "" {
			name = "`-" + f.Shorthand + "`, " + name
		}
		def := f.DefValue
		switch {
		case def == "" || def == "[]" || def == "false" || def == "0":
			def = ""
		default:
			def = "`" + def + "`"
		}
		fmt.Fprintf(b, "| %s | %s | %s |\n", name, def, strings.ReplaceAll(f.Usage, "|", "\\|"))
	})
}
