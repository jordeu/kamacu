package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/google/subcommands"
)

// version is injected at build time by goreleaser
// (-ldflags "-X main.version={{.Version}}"); plain `go build` yields "dev".
var version = "dev"

// printVersion writes the version string for the --version flag.
func printVersion(w io.Writer) {
	fmt.Fprintln(w, version)
}

func main() {
	subcommands.Register(subcommands.HelpCommand(), "")
	subcommands.Register(&serveCmd{}, "")
	subcommands.Register(mcpCmd{}, "")

	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		printVersion(os.Stdout)
		return
	}
	os.Exit(int(subcommands.Execute(context.Background())))
}
