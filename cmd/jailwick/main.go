// jailwick runs AI coding agents in a sandbox per repo. See README.md.
package main

import (
	"os"

	"github.com/jailwick/jailwick/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
