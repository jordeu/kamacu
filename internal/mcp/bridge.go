package mcp

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// defaultBase is the Kamacu loopback URL every spawned agent CLI sees when
// KAMACU_HOOK_BASE is unset. MCPPROC-03 / SC4 lock this default.
const defaultBase = "http://127.0.0.1:7333"

// maxBodyBytes caps a single bridge response at 1 MiB. T-06-08 mitigation:
// bounds a runaway Kamacu server's response so a tool handler can't OOM the
// subcommand. Bumpable in one line if a Phase 07+ tool returns more.
const maxBodyBytes = 1 << 20

// bridge is the per-process HTTP client the MCP tool handlers use to call the
// running Kamacu HTTP API. The bridge is plain HTTP (not WS) in Phase 06; all
// auth rides the X-Kamacu-Token header on every request (D-06/D-07 wire
// contract). Kamacu ignores the header on /api/* routes today (loopback
// binding is the actual v1.11 auth boundary), but the header is sent on every
// call so future server-side checks see it without further bridge changes.
type bridge struct {
	base   string
	token  string
	client *http.Client
}

// newBridgeFromEnv reads KAMACU_HOOK_BASE (default http://127.0.0.1:7333 when
// empty/unset) and KAMACU_HOOK_TOKEN (may be "" — Kamacu ignores on /api/*
// per D-06) and returns the constructed *bridge. An invalid KAMACU_HOOK_BASE
// surfaces as a startup error so the subcommand fails fast instead of silently
// pointing at a malformed URL.
func newBridgeFromEnv() (*bridge, error) {
	base := strings.TrimRight(os.Getenv("KAMACU_HOOK_BASE"), "/")
	if base == "" {
		base = defaultBase
	}
	if _, err := url.Parse(base); err != nil {
		return nil, fmt.Errorf("invalid KAMACU_HOOK_BASE %q: %w", base, err)
	}
	return &bridge{
		base:   base,
		token:  os.Getenv("KAMACU_HOOK_TOKEN"),
		client: &http.Client{Timeout: 10 * time.Second}, // fail fast; no retry (the agent's Discretion)
	}, nil
}

// do constructs and executes a single HTTP request against the Kamacu API,
// setting X-Kamacu-Token on every call. The body reader is passed through
// verbatim; nil means no body (GET / DELETE).
func (b *bridge) do(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, b.base+path, body)
	if err != nil {
		return nil, fmt.Errorf("kamacu bridge: build %s %s: %w", method, path, err)
	}
	req.Header.Set("X-Kamacu-Token", b.token)
	return b.client.Do(req)
}

// listProjects is the body of the list_projects tool handler. It calls
// GET /api/projects unchanged (D-08 — the optional project_id arg is a
// documented no-op today; Phase 07 will honor it) and returns the raw JSON
// body as a single TextContent block (the agent's Discretion in CONTEXT.md —
// raw passthrough keeps the bridge trivial and avoids forcing every Phase 07
// tool to declare an output type).
//
// On transport error: returns a wrapped error (the SDK surfaces this as a
// JSON-RPC error.code = -32603 response).
// On non-200: returns a descriptive error (same JSON-RPC wrapping).
// On 200: returns the body verbatim in a TextContent block.
func (b *bridge) listProjects(ctx context.Context) (*mcp.CallToolResult, error) {
	resp, err := b.do(ctx, http.MethodGet, "/api/projects", nil)
	if err != nil {
		return nil, fmt.Errorf("kamacu bridge: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("kamacu GET /api/projects: read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kamacu GET /api/projects: HTTP %d: %s", resp.StatusCode, string(body))
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(body)},
		},
	}, nil
}
