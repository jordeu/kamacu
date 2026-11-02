package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"kamacu/internal/notifications"
)

// NotificationClient uses the real MCP tool handlers and transport. Its server
// exposes only the notification allowlist, with no agent/session identity.
type NotificationClient struct {
	client *sdk.ClientSession
	server *sdk.ServerSession
	tools  []notifications.Tool
}

func NewNotificationClient(ctx context.Context, base, token string) (*NotificationClient, error) {
	b := &bridge{base: base, token: token, client: &http.Client{Timeout: 90 * time.Second}}
	server := sdk.NewServer(&sdk.Implementation{Name: "kamacu-notifications", Version: "1"}, nil)
	registerTools(server, b)
	st, ct := sdk.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		return nil, err
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "notification-worker", Version: "1"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		ss.Close()
		return nil, err
	}
	c := &NotificationClient{client: cs, server: ss}
	result, err := cs.ListTools(ctx, nil)
	if err != nil {
		c.Close()
		return nil, err
	}
	for _, tool := range result.Tools {
		if !notifications.AllowedTool(tool.Name) {
			server.RemoveTools(tool.Name)
			continue
		}
		c.tools = append(c.tools, notifications.Tool{Name: tool.Name, Description: tool.Description, InputSchema: tool.InputSchema})
	}
	return c, nil
}
func (c *NotificationClient) Close()                                             { _ = c.client.Close(); _ = c.server.Close() }
func (c *NotificationClient) List(context.Context) ([]notifications.Tool, error) { return c.tools, nil }
func (c *NotificationClient) Call(ctx context.Context, name string, args json.RawMessage) (string, error) {
	if !notifications.AllowedTool(name) {
		return "", errors.New("tool unavailable to notifications")
	}
	var arguments map[string]any
	if err := json.Unmarshal(args, &arguments); err != nil {
		return "", err
	}
	result, err := c.client.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		return "", err
	}
	var text strings.Builder
	for _, content := range result.Content {
		if item, ok := content.(*sdk.TextContent); ok {
			text.WriteString(item.Text)
			text.WriteByte('\n')
		}
	}
	if result.IsError {
		return "", errors.New(text.String())
	}
	return text.String(), nil
}
