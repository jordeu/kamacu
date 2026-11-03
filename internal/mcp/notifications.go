package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"kamacu/internal/notifications"
)

// NotificationClient uses the real MCP tool handlers and transport. Its server
// exposes the complete Kamacu tool catalog, with no agent/session identity.
// Agent-scoped compatibility tools retain their normal API identity requirements.
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
	for tool, err := range cs.Tools(ctx, nil) {
		if err != nil {
			c.Close()
			return nil, err
		}
		description := tool.Description
		if tool.Name == "get_session_output" || tool.Name == "subscribe_session_output" {
			description += " For the notification worker, output is decoded to readable UTF-8 and limited to the most recent 12000 bytes. Prefer get_session_output for a quick current snapshot."
		}
		c.tools = append(c.tools, notifications.Tool{Name: tool.Name, Description: description, InputSchema: tool.InputSchema})
	}
	return c, nil
}
func (c *NotificationClient) Close()                                             { _ = c.client.Close(); _ = c.server.Close() }
func (c *NotificationClient) List(context.Context) ([]notifications.Tool, error) { return c.tools, nil }
func (c *NotificationClient) Call(ctx context.Context, name string, args json.RawMessage) (string, error) {
	available := false
	for _, tool := range c.tools {
		if tool.Name == name {
			available = true
			break
		}
	}
	if !available {
		return "", errors.New("unknown Kamacu MCP tool")
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
	output := text.String()
	if name == "get_session_output" || name == "subscribe_session_output" {
		return readableNotificationTerminal(output)
	}
	return output, nil
}

// The session API transports bytes as base64. Give the notification model
// readable recent terminal evidence rather than asking it to decode that data.
func readableNotificationTerminal(raw string) (string, error) {
	var envelope map[string]any
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return "", err
	}
	if envelope["encoding"] != "base64" {
		return raw, nil
	}
	encoded, _ := envelope["output"].(string)
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	if len(decoded) > 12000 {
		decoded = decoded[len(decoded)-12000:]
		envelope["notification_excerpt_truncated"] = true
	}
	envelope["encoding"] = "utf-8"
	text := strings.ToValidUTF8(string(decoded), "�")
	// JSON escaping expands ANSI/control characters. Budget the final envelope,
	// preserving its newest text so downstream limits never cut off the tail.
	for {
		envelope["output"] = text
		result, err := json.Marshal(envelope)
		if err != nil {
			return "", err
		}
		if len(result) <= notifications.ToolResultLimit {
			return string(result), nil
		}
		if len(text) == 0 {
			return "", errors.New("terminal metadata exceeds notification tool-result limit")
		}
		envelope["notification_excerpt_truncated"] = true
		drop := min(max(1, len(text)/2), max(1, len(result)-notifications.ToolResultLimit))
		text = strings.ToValidUTF8(text[drop:], "")
	}
}
