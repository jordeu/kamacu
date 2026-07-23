package mcp

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"

	"github.com/google/subcommands"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ServeCommand is the body of `kamacu mcp serve`. It owns the MCP stdio
// server lifecycle: env parse → bridge construction → SDK init → tool
// registration → stdio loop. The subcommand is spawned by an agent CLI
// (claude / opencode) inside a Kamacu task PTY; the agent CLI speaks JSON-RPC
// over the subcommand's stdio.
type ServeCommand struct{}

func (ServeCommand) Name() string { return "serve" }

func (ServeCommand) Synopsis() string { return "run the Kamacu MCP stdio server" }

func (ServeCommand) Usage() string {
	return `kamacu mcp serve

Reads from stdin and writes JSON-RPC to stdout. Spawned by an agent CLI
(claude / opencode) inside a Kamacu task PTY. Env-driven (Phase 06 has no
flags by design — D-05):

  KAMACU_HOOK_BASE    Kamacu base URL (default http://127.0.0.1:7333)
  KAMACU_HOOK_TOKEN   Per-instance hook token (sent as X-Kamacu-Token header)

Exits 0 on stdin EOF or context cancel; non-zero on bridge/server error.
`
}

// SetFlags is empty — Phase 06 is env-driven, no flags (D-05 /
// 06-RESEARCH.md Anti-Patterns).
func (ServeCommand) SetFlags(*flag.FlagSet) {}

// Execute runs the MCP stdio server until stdin EOF or context cancel.
// Pitfall 5 (load-bearing): on stdin EOF the SDK's Run returns nil; that nil
// MUST map to ExitSuccess so `echo | kamacu mcp serve; echo $?` prints 0.
func (ServeCommand) Execute(ctx context.Context, _ *flag.FlagSet, _ ...any) subcommands.ExitStatus {
	// D-12(a): pin slog to stderr explicitly so future import-graph changes
	// can't redirect slog to stdout and pollute the JSON-RPC stream.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	b, err := newBridgeFromEnv()
	if err != nil {
		slog.Error("mcp bridge env", "error", err)
		return subcommands.ExitFailure
	}

	// nil options means the SDK's own logger discards log output entirely
	// (slog.DiscardHandler per 06-RESEARCH.md Pitfall 5).
	srv := mcp.NewServer(&mcp.Implementation{Name: "kamacu", Version: "dev"}, nil)
	registerTools(srv, b)

	if err := srv.Run(ctx, &mcp.StdioTransport{}); err != nil {
		if errors.Is(err, context.Canceled) {
			return subcommands.ExitSuccess
		}
		slog.Error("mcp server ended", "error", err)
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}

// registerTools delegates to the per-resource registrars (D-07 split). Each
// registrar (registerTaskTools / registerProjectTools / registerWorkspaceTools
// / registerSessionTools / registerReviewTools) owns its s.AddTool calls for
// the tools in its resource; the bridge shared helper (bridge.call) handles
// the response-shape half of every handler (the list-review tools diverge for
// the state-field model — see reviews.go).
//
// The low-level Server.AddTool method is used with an explicit
// map[string]any InputSchema — NOT the typed generic AddTool helper from the
// SDK (06-RESEARCH.md Anti-Patterns). SDK v1.6.1's Tool.InputSchema field is
// `any`; a plain map satisfies the type:"object" requirement without
// jsonschema-go.
func registerTools(s *mcp.Server, b *bridge) {
	registerTaskTools(s, b)
	registerProjectTools(s, b)
	registerWorkspaceTools(s, b)
	registerSessionTools(s, b)
	registerReviewTools(s, b) // NEW — Phase 09 (list_pending_reviews / list_recently_reviewed / open_review)
}

