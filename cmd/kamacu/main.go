package main

import (
	"context"
	"flag"
	"os"

	"github.com/google/subcommands"
)

func main() {
	subcommands.Register(subcommands.HelpCommand(), "")
	subcommands.Register(&serveCmd{}, "")
	subcommands.Register(mcpCmd{}, "")

	flag.Parse()
	os.Exit(int(subcommands.Execute(context.Background())))
}
