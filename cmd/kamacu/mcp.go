package main

import (
	"context"
	"flag"

	"github.com/google/subcommands"

	kamacumcp "kamacu/internal/mcp"
)

// mcpCmd is the `mcp` command group. `google/subcommands` is FLAT, not
// nested — its Execute() looks only at the FIRST positional arg. "Groups"
// are help-organization only (06-RESEARCH.md § "Pattern 2"). To get
// `kamacu mcp serve`, this Command owns its own inner subcommands.Commander
// and dispatches to it. Future `mcp <other>` subcommands slot in via
// additional cdr.Register calls inside Execute.
type mcpCmd struct{}

func (mcpCmd) Name() string { return "mcp" }

func (mcpCmd) Synopsis() string { return "MCP-related subcommands" }

func (mcpCmd) Usage() string {
	return `mcp <subcommand>:
  mcp serve   Run the Kamacu MCP stdio server.
`
}

// SetFlags is empty — the `mcp` group itself takes no flags; each
// subcommand owns its own FlagSet.
func (mcpCmd) SetFlags(*flag.FlagSet) {}

// Execute constructs an inner subcommands.Commander bound to this Command's
// FlagSet, registers the library's HelpCommand and the `serve` body from
// internal/mcp, and dispatches. Per 06-RESEARCH.md Pattern 2 / Pitfall 3:
// do NOT register a Command literally named "mcp serve" with a space — the
// library matches on the first positional arg only.
func (mcpCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...any) subcommands.ExitStatus {
	cdr := subcommands.NewCommander(f, "kamacu mcp")
	cdr.Register(cdr.HelpCommand(), "")
	cdr.Register(kamacumcp.ServeCommand{}, "")
	return cdr.Execute(ctx)
}
