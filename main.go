package main

import (
	"os"

	"github.com/search17/search17/cmd"
)

// version is injected at build time via -ldflags (see .goreleaser.yaml).
var version = "dev"

func main() {
	cmd.SetVersion(version)
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
