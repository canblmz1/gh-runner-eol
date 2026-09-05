// gh-runner-eol audits GitHub self-hosted runner fleets against GitHub's
// official runner end-of-life schedule and flags versions that will stop
// receiving jobs — before they do.
package main

import (
	"os"

	"github.com/canblmz1/gh-runner-eol/internal/cli"
)

// version is overwritten at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	os.Exit(cli.Execute(version, os.Args[1:], os.Stdout, os.Stderr))
}
