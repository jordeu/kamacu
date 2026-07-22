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

// call is the shared response-handling half every Phase 07 tool handler
// delegates to (the per-resource files extract request args, build the path
// and body, then call this). It encapsulates the Phase 06 listProjects shape:
// b.do → defer Close → io.ReadAll(LimitReader) → transport-error wrap
// ("kamacu bridge: %w") → non-2xx wrap (D-05 — body passed as a raw string,
// shape-agnostic per 07-RESEARCH Gap 2: the real Kamacu error shape is
// {"error":"..."}, not {"status","message"}) → 2xx TextContent passthrough.
//
// On transport error: returns a wrapped error (the SDK surfaces this as a
// JSON-RPC error.code = -32603 response).
// On non-2xx: returns a descriptive error with the raw body inlined.
// On 2xx: returns the body verbatim in a single TextContent block. The full
// 2xx range is accepted so the bridge works across Kamacu's heterogeneous
// success codes: GET handlers return 200, POST create returns 201 (Created),
// DELETE returns 204 (No Content). Plan 01 shipped this helper accepting only
// 200 because list_projects (GET) was the only caller; Plan 02's create_task
// and delete_task exposed the gap, fixed here (Rule 1 — auto-fix blocking
// bug; otherwise every successful create/delete would surface as an error to
// the agent).
func (b *bridge) call(ctx context.Context, method, path string, body io.Reader) (*mcp.CallToolResult, error) {
	resp, err := b.do(ctx, method, path, body)
	if err != nil {
		return nil, fmt.Errorf("kamacu bridge: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("kamacu %s %s: read body: %w", method, path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("kamacu %s %s: HTTP %d: %s", method, path, resp.StatusCode, string(respBody))
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(respBody)},
		},
	}, nil
}
