package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/qbj1122/campusclaw-zww/internal/model"
)

var ErrMCPCall = errors.New("mcp call failed")

type MCPClient struct {
	HTTP *http.Client
}

func NewMCPClient() *MCPClient {
	return &MCPClient{HTTP: &http.Client{Timeout: 20 * time.Second}}
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

func mcpEndpoint(base string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if strings.HasSuffix(base, "/mcp") {
		return base
	}
	return base + "/mcp"
}

func (c *MCPClient) rpc(ctx context.Context, base, token string, id int, method string, params any) (json.RawMessage, error) {
	body := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if params != nil {
		body["params"] = params
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, mcpEndpoint(base), bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMCPCall, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("%w: 令牌被拒绝 (HTTP %d)", ErrMCPCall, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: HTTP %d", ErrMCPCall, resp.StatusCode)
	}
	var out rpcResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("%w: 响应不是合法 JSON-RPC", ErrMCPCall)
	}
	if out.Error != nil {
		return nil, fmt.Errorf("%w: %s", ErrMCPCall, out.Error.Message)
	}
	return out.Result, nil
}

// Connect performs handshake + discovery. Both steps must succeed before the
// caller replaces its tool cache and marks the server connected.
func (c *MCPClient) Connect(ctx context.Context, base, token string) ([]model.MCPTool, error) {
	initParams := map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "campusclaw", "version": "1.0.0"},
	}
	if _, err := c.rpc(ctx, base, token, 1, "initialize", initParams); err != nil {
		return nil, err
	}
	raw, err := c.rpc(ctx, base, token, 2, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var res struct {
		Tools []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("%w: tools/list 结构不正确", ErrMCPCall)
	}
	tools := make([]model.MCPTool, 0, len(res.Tools))
	for _, t := range res.Tools {
		if t.Name == "" {
			continue
		}
		tools = append(tools, model.MCPTool{Name: t.Name, Description: t.Description, SchemaJSON: string(t.InputSchema)})
	}
	return tools, nil
}

type ToolResult struct {
	Text    string
	IsError bool
}

func (c *MCPClient) Call(ctx context.Context, base, token, tool string, args map[string]any) (*ToolResult, error) {
	raw, err := c.rpc(ctx, base, token, 3, "tools/call", map[string]any{"name": tool, "arguments": args})
	if err != nil {
		return nil, err
	}
	var res struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("%w: tools/call 结构不正确", ErrMCPCall)
	}
	var parts []string
	for _, p := range res.Content {
		if p.Text != "" {
			parts = append(parts, p.Text)
		}
	}
	return &ToolResult{Text: strings.Join(parts, "\n"), IsError: res.IsError}, nil
}
