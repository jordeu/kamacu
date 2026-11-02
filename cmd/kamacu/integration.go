package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/google/subcommands"
	"kamacu/internal/integration"
)

type integrationCmd struct{}

func (integrationCmd) Name() string { return "integration" }
func (integrationCmd) Synopsis() string {
	return "Install, inspect, repair, or remove agent MCP integration"
}
func (integrationCmd) Usage() string {
	return `integration status
integration enable <claude|codex|opencode>
integration repair <claude|codex|opencode>
integration remove <claude|codex|opencode>

Registers a user-level MCP server. Repair preserves user edits. Remove deletes
only unchanged managed registrations. Restart agent sessions after changes.
`
}
func (integrationCmd) SetFlags(*flag.FlagSet) {}
func (c integrationCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...any) subcommands.ExitStatus {
	if f.NArg() < 1 {
		fmt.Fprint(os.Stderr, c.Usage())
		return subcommands.ExitUsageError
	}
	action := f.Arg(0)
	if (action == "status" && f.NArg() != 1) || (action != "status" && (f.NArg() != 2 || (action != "enable" && action != "repair" && action != "remove"))) {
		fmt.Fprint(os.Stderr, c.Usage())
		return subcommands.ExitUsageError
	}
	manager, err := integration.Default()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return subcommands.ExitFailure
	}
	var result any
	switch action {
	case "status":
		result, err = manager.Status(ctx)
	case "enable", "repair":
		result, err = manager.Enable(ctx, f.Arg(1))
	case "remove":
		result, err = manager.Disable(ctx, f.Arg(1))
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return subcommands.ExitFailure
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if encoder.Encode(result) != nil {
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}
