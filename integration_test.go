package main_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/miyamiyaz/mcp-supervisor/internal/mcp"
	"github.com/miyamiyaz/mcp-supervisor/internal/proxy"
)

var echomcpBin string

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "mcp-supervisor-test")
	if err != nil {
		fmt.Fprintf(os.Stderr, "mkdirtemp: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmp)

	echomcpBin = filepath.Join(tmp, "echomcp")
	out, err := exec.Command("go", "build", "-o", echomcpBin, "./internal/testutil/echomcp").CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "build echomcp: %v\n%s\n", err, out)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func TestIntegration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Pipes: test writes to supervisorIn, reads from supervisorOut.
	clientReader, supervisorOut := io.Pipe()
	supervisorIn, clientWriter := io.Pipe()

	transport := mcp.NewTransport(supervisorIn, supervisorOut)
	p := proxy.New(nil)
	info := mcp.ServerInfo{Name: "mcp-supervisor", Version: "test"}
	server := mcp.NewServer(transport, info, toolsProvider(p), toolHandler(ctx, p))
	p.SetOnChanged(func() {
		_ = server.NotifyToolsChanged()
	})

	// Client-side transport: reads server responses, sends requests.
	clientTransport := mcp.NewTransport(clientReader, clientWriter)

	serverDone := make(chan error, 1)
	go func() { serverDone <- server.Serve() }()

	// Helper: send request, read response (skipping notifications).
	var nextID int
	send := func(t *testing.T, method string, params any) mcp.Response {
		t.Helper()
		nextID++
		id, _ := json.Marshal(nextID)
		var rawParams json.RawMessage
		if params != nil {
			rawParams, _ = json.Marshal(params)
		}
		req := mcp.Request{JSONRPC: "2.0", ID: id, Method: method, Params: rawParams}
		if err := clientTransport.WriteMessage(req); err != nil {
			t.Fatalf("send %s: %v", method, err)
		}

		for {
			raw, err := clientTransport.ReadMessage()
			if err != nil {
				t.Fatalf("read response for %s: %v", method, err)
			}
			var resp mcp.Response
			if err := json.Unmarshal(raw, &resp); err != nil {
				t.Fatalf("unmarshal response: %v", err)
			}
			// Skip notifications (no ID).
			if resp.ID == nil {
				continue
			}
			return resp
		}
	}

	// 1. Initialize
	resp := send(t, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]string{"name": "test", "version": "0.0.1"},
	})
	if resp.Error != nil {
		t.Fatalf("initialize error: %s", resp.Error.Message)
	}
	var initResult mcp.InitializeResult
	if err := json.Unmarshal(resp.Result, &initResult); err != nil {
		t.Fatalf("parse initialize result: %v", err)
	}
	if initResult.ServerInfo.Name != "mcp-supervisor" {
		t.Fatalf("expected server name mcp-supervisor, got %s", initResult.ServerInfo.Name)
	}

	// Send initialized notification.
	notif := mcp.Request{JSONRPC: "2.0", Method: "initialized"}
	if err := clientTransport.WriteMessage(notif); err != nil {
		t.Fatalf("send initialized: %v", err)
	}

	// 2. tools/list — only supervisor tools
	resp = send(t, "tools/list", nil)
	var toolsList mcp.ToolsListResult
	if err := json.Unmarshal(resp.Result, &toolsList); err != nil {
		t.Fatalf("parse tools/list: %v", err)
	}
	if len(toolsList.Tools) != 3 {
		t.Fatalf("expected 3 supervisor tools, got %d", len(toolsList.Tools))
	}

	// 3. start_mcp with echomcp
	resp = send(t, "tools/call", mcp.ToolCallParams{
		Name: "start_mcp",
		Arguments: mustJSON(map[string]any{
			"name":    "test",
			"command": echomcpBin,
		}),
	})
	if resp.Error != nil {
		t.Fatalf("start_mcp error: %s", resp.Error.Message)
	}
	var startResult mcp.ToolResult
	if err := json.Unmarshal(resp.Result, &startResult); err != nil {
		t.Fatalf("parse start_mcp result: %v", err)
	}
	if startResult.IsError {
		t.Fatalf("start_mcp returned error: %s", startResult.Content[0].Text)
	}
	// Verify the response mentions the echo tool.
	var startInfo map[string]any
	if err := json.Unmarshal([]byte(startResult.Content[0].Text), &startInfo); err != nil {
		t.Fatalf("parse start_mcp text: %v", err)
	}
	tools, ok := startInfo["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("expected 1 tool from child, got %v", startInfo["tools"])
	}

	// 4. tools/list — should now include test.echo (4 tools total)
	resp = send(t, "tools/list", nil)
	if err := json.Unmarshal(resp.Result, &toolsList); err != nil {
		t.Fatalf("parse tools/list: %v", err)
	}
	if len(toolsList.Tools) != 4 {
		names := make([]string, len(toolsList.Tools))
		for i, tool := range toolsList.Tools {
			names[i] = tool.Name
		}
		t.Fatalf("expected 4 tools, got %d: %v", len(toolsList.Tools), names)
	}
	found := false
	for _, tool := range toolsList.Tools {
		if tool.Name == "test.echo" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("test.echo not found in tools list")
	}

	// 5. Call test.echo
	resp = send(t, "tools/call", mcp.ToolCallParams{
		Name:      "test.echo",
		Arguments: mustJSON(map[string]any{"message": "hello"}),
	})
	if resp.Error != nil {
		t.Fatalf("test.echo error: %s", resp.Error.Message)
	}
	var echoResult mcp.ToolResult
	if err := json.Unmarshal(resp.Result, &echoResult); err != nil {
		t.Fatalf("parse test.echo result: %v", err)
	}
	if echoResult.IsError {
		t.Fatalf("test.echo returned error: %s", echoResult.Content[0].Text)
	}
	if echoResult.Content[0].Text != `{"message":"hello"}` {
		t.Fatalf("expected echo to return {\"message\":\"hello\"}, got %s", echoResult.Content[0].Text)
	}

	// 6. stop_mcp
	resp = send(t, "tools/call", mcp.ToolCallParams{
		Name:      "stop_mcp",
		Arguments: mustJSON(map[string]any{"name": "test"}),
	})
	if resp.Error != nil {
		t.Fatalf("stop_mcp error: %s", resp.Error.Message)
	}

	// 7. tools/list — test.echo should be gone
	resp = send(t, "tools/list", nil)
	if err := json.Unmarshal(resp.Result, &toolsList); err != nil {
		t.Fatalf("parse tools/list: %v", err)
	}
	if len(toolsList.Tools) != 3 {
		names := make([]string, len(toolsList.Tools))
		for i, tool := range toolsList.Tools {
			names[i] = tool.Name
		}
		t.Fatalf("expected 3 tools after stop, got %d: %v", len(toolsList.Tools), names)
	}

	// 8. Close client writer → server sees EOF → exits cleanly.
	clientWriter.Close()
	if err := <-serverDone; err != nil {
		t.Fatalf("server exited with error: %v", err)
	}
}

// toolsProvider and toolHandler mirror cmd/supervisor-mcp/main.go but are
// defined here so the test can construct the server without importing main.

func toolsProvider(p *proxy.Proxy) mcp.ToolsProvider {
	return func() []mcp.Tool {
		tools := supervisorTools()
		tools = append(tools, p.Tools()...)
		return tools
	}
}

func toolHandler(ctx context.Context, p *proxy.Proxy) mcp.ToolHandler {
	return func(params mcp.ToolCallParams) (mcp.ToolResult, error) {
		switch params.Name {
		case "start_mcp":
			return handleStartMCP(ctx, p, params.Arguments)
		case "stop_mcp":
			return handleStopMCP(p, params.Arguments)
		case "list_mcps":
			return handleListMCPs(p)
		default:
			if len(params.Name) > 0 {
				for i := 0; i < len(params.Name); i++ {
					if params.Name[i] == '.' {
						return p.CallTool(ctx, params)
					}
				}
			}
			return mcp.ToolResult{}, fmt.Errorf("unknown tool: %s", params.Name)
		}
	}
}

func supervisorTools() []mcp.Tool {
	return []mcp.Tool{
		{
			Name:        "start_mcp",
			Description: "Start a child MCP server and proxy its tools",
			InputSchema: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":    map[string]any{"type": "string"},
					"command": map[string]any{"type": "string"},
					"args":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"env":     map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}},
				},
				"required": []string{"name", "command"},
			}),
		},
		{
			Name:        "stop_mcp",
			Description: "Stop a running child MCP server",
			InputSchema: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string"},
				},
				"required": []string{"name"},
			}),
		},
		{
			Name:        "list_mcps",
			Description: "List running child MCP servers",
			InputSchema: mustJSON(map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			}),
		},
	}
}

func handleStartMCP(ctx context.Context, p *proxy.Proxy, args json.RawMessage) (mcp.ToolResult, error) {
	var params proxy.StartParams
	if err := json.Unmarshal(args, &params); err != nil {
		return mcp.ToolResult{}, fmt.Errorf("invalid arguments: %w", err)
	}
	result, err := p.StartMCP(ctx, params)
	if err != nil {
		return mcp.ToolResult{}, err
	}
	return textResult(result)
}

func handleStopMCP(p *proxy.Proxy, args json.RawMessage) (mcp.ToolResult, error) {
	var params struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return mcp.ToolResult{}, fmt.Errorf("invalid arguments: %w", err)
	}
	if err := p.StopMCP(params.Name); err != nil {
		return mcp.ToolResult{}, err
	}
	return textResult(map[string]any{"stopped": true, "name": params.Name})
}

func handleListMCPs(p *proxy.Proxy) (mcp.ToolResult, error) {
	return textResult(map[string]any{"mcps": p.ListMCPs()})
}

func textResult(v any) (mcp.ToolResult, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return mcp.ToolResult{}, err
	}
	return mcp.ToolResult{
		Content: []mcp.Content{{Type: "text", Text: string(data)}},
	}, nil
}

func mustJSON(v any) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}
