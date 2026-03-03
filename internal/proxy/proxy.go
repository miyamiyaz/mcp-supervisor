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
)

// Proxy manages child MCP servers and proxies their tools.
type Proxy struct {
	mu        sync.RWMutex
	children  map[string]*childmcp.Child // keyed by name
	onChanged func()                     // called when tool list changes
}

func New(onChanged func()) *Proxy {
	return &Proxy{
		children:  make(map[string]*childmcp.Child),
		onChanged: onChanged,
	}
}

// SetOnChanged sets the callback invoked when the tool list changes.
func (p *Proxy) SetOnChanged(fn func()) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onChanged = fn
}

// StartMCP params
type StartParams struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

// StartMCP launches a child MCP, registers its tools, and notifies the client.
func (p *Proxy) StartMCP(ctx context.Context, params StartParams) (map[string]any, error) {
	if params.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if params.Command == "" {
		return nil, fmt.Errorf("command is required")
	}

	p.mu.Lock()
	if _, exists := p.children[params.Name]; exists {
		p.mu.Unlock()
		return nil, fmt.Errorf("mcp %q already running", params.Name)
	}
	p.mu.Unlock()

	// Build env slice: inherit parent env + overrides
	var env []string
	if len(params.Env) > 0 {
		env = mergeEnv(params.Env)
	}

	child, err := childmcp.Start(ctx, params.Name, params.Command, params.Args, env)
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	p.children[params.Name] = child
	p.mu.Unlock()

	if p.onChanged != nil {
		p.onChanged()
	}

	toolNames := make([]string, len(child.Tools))
	for i, t := range child.Tools {
		toolNames[i] = t.Name
	}

	return map[string]any{
		"name":   params.Name,
		"pid":    child.Pid(),
		"tools":  toolNames,
		"status": "running",
	}, nil
}

// StopMCP stops a child MCP by name.
func (p *Proxy) StopMCP(name string) error {
	p.mu.Lock()
	child, ok := p.children[name]
	if !ok {
		p.mu.Unlock()
		return fmt.Errorf("mcp %q not found", name)
	}
	delete(p.children, name)
	p.mu.Unlock()

	child.Stop()

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
	for name, child := range p.children {
		toolNames := make([]string, len(child.Tools))
		for i, t := range child.Tools {
			toolNames[i] = t.Name
		}
		result = append(result, map[string]any{
			"name":    name,
			"command": child.Command,
			"status":  "running",
			"tools":   toolNames,
		})
	}
	return result
}

// Tools returns all tools from all children, prefixed with child name.
func (p *Proxy) Tools() []mcp.Tool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var tools []mcp.Tool
	for name, child := range p.children {
		for _, t := range child.Tools {
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
	child, ok := p.children[childName]
	p.mu.RUnlock()

	if !ok {
		return mcp.ToolResult{}, fmt.Errorf("mcp %q not found", childName)
	}

	childParams := mcp.ToolCallParams{
		Name:      toolName,
		Arguments: params.Arguments,
	}
	return child.CallTool(ctx, childParams)
}

// StopAll stops all child MCPs. Used during shutdown.
func (p *Proxy) StopAll() {
	p.mu.Lock()
	children := make(map[string]*childmcp.Child, len(p.children))
	for k, v := range p.children {
		children[k] = v
	}
	p.children = make(map[string]*childmcp.Child)
	p.mu.Unlock()

	for name, child := range children {
		log.Printf("stopping child mcp: %s", name)
		child.Stop()
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
