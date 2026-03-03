package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/miyamiyaz/mcp-supervisor/internal/mcp"
	"github.com/miyamiyaz/mcp-supervisor/internal/proxy"
)

func main() {
	log.SetOutput(os.Stderr)
	log.SetFlags(log.Ltime | log.Lshortfile)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	transport := mcp.NewTransport(os.Stdin, os.Stdout)
	p := proxy.New(nil) // onChanged set below

	info := mcp.ServerInfo{Name: "mcp-supervisor", Version: "0.1.0"}

	server := mcp.NewServer(transport, info, toolsProvider(p), toolHandler(ctx, p))

	// Wire up notifications: when child tools change, notify client
	p.SetOnChanged(func() {
		if err := server.NotifyToolsChanged(); err != nil {
			log.Printf("notify tools changed: %v", err)
		}
	})

	// Graceful shutdown
	go func() {
		<-ctx.Done()
		log.Println("shutting down, stopping all child MCPs...")
		p.StopAll()
	}()

	if err := server.Serve(); err != nil {
		log.Fatalf("server: %v", err)
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
					"name":    map[string]any{"type": "string", "description": "Unique name for this MCP instance (used as tool prefix)"},
					"command": map[string]any{"type": "string", "description": "Command to run"},
					"args":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Command arguments"},
					"env":     map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "description": "Extra environment variables (merged with parent env)"},
				},
				"required": []string{"name", "command"},
			}),
		},
		{
			Name:        "stop_mcp",
			Description: "Stop a running child MCP server and remove its tools",
			InputSchema: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string", "description": "Name of the MCP to stop"},
				},
				"required": []string{"name"},
			}),
		},
		{
			Name:        "list_mcps",
			Description: "List all running child MCP servers and their tools",
			InputSchema: mustJSON(map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			}),
		},
	}
}

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
			// Check if it's a proxied tool (contains ".")
			if strings.Contains(params.Name, ".") {
				return p.CallTool(ctx, params)
			}
			return mcp.ToolResult{}, fmt.Errorf("unknown tool: %s", params.Name)
		}
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
