package mcp

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestSC2_HandlerReturningError_ProducesCleanJSONRPCErrResponse is the SC2
// regression test (D-11). It proves a tool handler returning `(nil, error)`
// surfaces as a clean JSON-RPC error response on stdout — never a process
// crash, never a non-JSON line, never a stream desync. The SDK wraps the
// handler error as JSON-RPC error.code = -32603 (internal error).
//
// The test drives the server over an in-memory transport via a real
// mcp.Client, so the JSON-RPC roundtrip is end-to-end (handler → SDK → wire →
// client SDK → caller). If the SDK ever emitted non-JSON bytes on stdout or
// crashed the process, this test would fail with a transport error rather
// than a clean result.
func TestSC2_HandlerReturningError_ProducesCleanJSONRPCErrResponse(t *testing.T) {
	ctx := context.Background()
	t1, t2 := mcp.NewInMemoryTransports()

	srv := mcp.NewServer(&mcp.Implementation{Name: "kamacu-test", Version: "test"}, nil)
	srv.AddTool(
		&mcp.Tool{
			Name:        "fail_err",
			Description: "deliberately errors to exercise the JSON-RPC error-response path",
			InputSchema: map[string]any{"type": "object"},
		},
		func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return nil, errors.New("boom")
		},
	)
	serverSession, err := srv.Connect(ctx, t1, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer serverSession.Close() //nolint:errcheck

	client := mcp.NewClient(&mcp.Implementation{Name: "kamacu-test-client", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer clientSession.Close() //nolint:errcheck

	// Initialize is implicit in Connect; call the tool and assert a clean
	// error (no panic, no stream desync — CallTool returns a wrapped error).
	_, callErr := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "fail_err",
		Arguments: map[string]any{},
	})
	if callErr == nil {
		t.Fatal("CallTool: expected error from fail_err handler, got nil")
	}
	// JSON-RPC error.code = -32603 (internal error). The SDK surfaces it via
	// the error message; assert the handler's "boom" message survives.
	if !strings.Contains(callErr.Error(), "boom") {
		t.Errorf("CallTool error message: want substring \"boom\", got %q", callErr.Error())
	}
}

// TestSC2_HandlerReturningIsErrorResult_ProducesCleanJSONRPCSuccessWithIsErrorTrue
// is the second SC2 case (D-11). It proves a tool handler returning
// `(*CallToolResult{IsError: true, Content: [TextContent]}, nil)` surfaces as
// a JSON-RPC *success* response (no `error` field on the wire) with
// `result.isError == true` — the MCP-level error path. The LLM sees and can
// self-correct; the SDK does not raise a transport-level error.
func TestSC2_HandlerReturningIsErrorResult_ProducesCleanJSONRPCSuccessWithIsErrorTrue(t *testing.T) {
	ctx := context.Background()
	t1, t2 := mcp.NewInMemoryTransports()

	srv := mcp.NewServer(&mcp.Implementation{Name: "kamacu-test", Version: "test"}, nil)
	srv.AddTool(
		&mcp.Tool{
			Name:        "fail_iserror",
			Description: "returns IsError=true to exercise the MCP-level error path",
			InputSchema: map[string]any{"type": "object"},
		},
		func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: "boom"},
				},
			}, nil
		},
	)
	serverSession, err := srv.Connect(ctx, t1, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer serverSession.Close() //nolint:errcheck

	client := mcp.NewClient(&mcp.Implementation{Name: "kamacu-test-client", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer clientSession.Close() //nolint:errcheck

	// The IsError path is a JSON-RPC SUCCESS — CallTool returns no Go error.
	res, callErr := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "fail_iserror",
		Arguments: map[string]any{},
	})
	if callErr != nil {
		t.Fatalf("CallTool: expected nil error (IsError path is JSON-RPC success), got %v", callErr)
	}
	if !res.IsError {
		t.Errorf("CallTool result.IsError: want true, got false")
	}
	if len(res.Content) == 0 {
		t.Fatal("CallTool result.Content: empty")
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("CallTool result.Content[0]: want *TextContent, got %T", res.Content[0])
	}
	if tc.Text != "boom" {
		t.Errorf("CallTool result.Content[0].Text: want %q, got %q", "boom", tc.Text)
	}
}
