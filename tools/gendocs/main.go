// gendocs writes docs/CLI.md from the command definitions. Run with `make docs`.
package main

import (
	"log"
	"os"

	"github.com/jailwick/jailwick/internal/cli"
)

func main() {
	out := "docs/CLI.md"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	if err := os.WriteFile(out, []byte(cli.Reference()), 0o644); err != nil {
		log.Fatal(err)
	}
}
