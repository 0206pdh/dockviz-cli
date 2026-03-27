package main

import "github.com/0206pdh/dockviz-cli/cmd"

// version is set at build time via -ldflags="-X main.version=<tag>".
// It defaults to "dev" for local builds.
var version = "dev"

func main() {
	cmd.Execute(version)
}
