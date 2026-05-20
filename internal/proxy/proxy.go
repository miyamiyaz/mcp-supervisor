package proxy

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/miyamiyaz/mcp-supervisor/internal/childmcp"
	"github.com/miyamiyaz/mcp-supervisor/internal/mcp"
	"github.com/miyamiyaz/mcp-supervisor/internal/remotemcp"
)

// Client is the common interface for stdio and remote MCP connections.
type Client interface {
	Tools() []mcp.Tool
	CallTool(ctx context.Context, params mcp.ToolCallParams) (mcp.ToolResult, error)
	Stop()
	Info() map[string]any
}

// Proxy manages child MCP servers and proxies their tools.
type Proxy struct {
	mu        sync.RWMutex
	children  map[string]Client
	onChanged func()
}

func New(onChanged func()) *Proxy {
	return &Proxy{
		children:  make(map[string]Client),
		onChanged: onChanged,
	}
}

// SetOnChanged sets the callback invoked when the tool list changes.
func (p *Proxy) SetOnChanged(fn func()) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onChanged = fn
}

// StartParams for start_mcp. Exactly one of Command or URL must be set.
type StartParams struct {
	Name    string            `json:"name"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// StartMCP launches or connects to a child MCP, registers its tools, and notifies the client.
func (p *Proxy) StartMCP(ctx context.Context, params StartParams) (map[string]any, error) {
	if params.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if params.Command == "" && params.URL == "" {
		return nil, fmt.Errorf("either command or url is required")
	}
	if params.Command != "" && params.URL != "" {
		return nil, fmt.Errorf("only one of command or url may be set")
	}

	p.mu.Lock()
	if _, exists := p.children[params.Name]; exists {
		p.mu.Unlock()
		return nil, fmt.Errorf("mcp %q already running", params.Name)
	}
	p.mu.Unlock()

	var client Client
	if params.URL != "" {
		remote, err := remotemcp.Connect(ctx, params.Name, params.URL, params.Headers)
		if err != nil {
			return nil, err
		}
		client = remote
	} else {
		var env []string
		if len(params.Env) > 0 {
			env = mergeEnv(params.Env)
		}
		child, err := childmcp.Start(ctx, params.Name, params.Command, params.Args, env)
		if err != nil {
			return nil, err
		}
		client = child
	}

	p.mu.Lock()
	p.children[params.Name] = client
	p.mu.Unlock()

	if p.onChanged != nil {
		p.onChanged()
	}

	info := client.Info()
	toolNames := make([]string, len(client.Tools()))
	for i, t := range client.Tools() {
		toolNames[i] = t.Name
	}
	info["tools"] = toolNames
	return info, nil
}

// StopMCP stops a child MCP by name.
func (p *Proxy) StopMCP(name string) error {
	p.mu.Lock()
	client, ok := p.children[name]
	if !ok {
		p.mu.Unlock()
		return fmt.Errorf("mcp %q not found", name)
	}
	delete(p.children, name)
	p.mu.Unlock()

	client.Stop()

	if p.onChanged != nil {
		p.onChanged()
	}
	return nil
}

// ListMCPs returns info about all running child MCPs.
func (p *Proxy) ListMCPs() []map[string]any {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make([]map[string]any, 0, len(p.children))
	for _, client := range p.children {
		info := client.Info()
		toolNames := make([]string, len(client.Tools()))
		for i, t := range client.Tools() {
			toolNames[i] = t.Name
		}
		info["tools"] = toolNames
		result = append(result, info)
	}
	return result
}

// Tools returns all tools from all children, prefixed with child name.
func (p *Proxy) Tools() []mcp.Tool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var tools []mcp.Tool
	for name, client := range p.children {
		for _, t := range client.Tools() {
			tools = append(tools, mcp.Tool{
				Name:        name + "." + t.Name,
				Description: fmt.Sprintf("[%s] %s", name, t.Description),
				InputSchema: t.InputSchema,
			})
		}
	}
	return tools
}

// CallTool routes a prefixed tool call to the right child.
func (p *Proxy) CallTool(ctx context.Context, params mcp.ToolCallParams) (mcp.ToolResult, error) {
	parts := strings.SplitN(params.Name, ".", 2)
	if len(parts) != 2 {
		return mcp.ToolResult{}, fmt.Errorf("invalid proxied tool name: %s", params.Name)
	}
	childName, toolName := parts[0], parts[1]

	p.mu.RLock()
	client, ok := p.children[childName]
	p.mu.RUnlock()

	if !ok {
		return mcp.ToolResult{}, fmt.Errorf("mcp %q not found", childName)
	}

	return client.CallTool(ctx, mcp.ToolCallParams{Name: toolName, Arguments: params.Arguments})
}

// StopAll stops all child MCPs. Used during shutdown.
func (p *Proxy) StopAll() {
	p.mu.Lock()
	children := make(map[string]Client, len(p.children))
	for k, v := range p.children {
		children[k] = v
	}
	p.children = make(map[string]Client)
	p.mu.Unlock()

	for name, client := range children {
		log.Printf("stopping child mcp: %s", name)
		client.Stop()
	}
}

func mergeEnv(overrides map[string]string) []string {
	env := make(map[string]string)
	for _, e := range os.Environ() {
		if k, v, ok := strings.Cut(e, "="); ok {
			env[k] = v
		}
	}
	for k, v := range overrides {
		env[k] = v
	}
	result := make([]string, 0, len(env))
	for k, v := range env {
		result = append(result, k+"="+v)
	}
	return result
}
