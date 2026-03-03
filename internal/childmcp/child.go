package childmcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os/exec"
	"sync"
	"sync/atomic"

	"github.com/miyamiyaz/mcp-supervisor/internal/mcp"
)

// Child represents a running child MCP server process.
type Child struct {
	Name    string
	Command string
	Args    []string
	Env     []string
	Tools   []mcp.Tool // tools reported by the child

	cmd       *exec.Cmd
	transport *mcp.Transport
	stdin     io.WriteCloser
	nextID    atomic.Int64
	mu        sync.Mutex
	pending   map[int64]chan *mcp.Response
	done      chan struct{}
	stderrBuf *ringBuffer
}

type ringBuffer struct {
	lines []string
	head  int
	count int
	mu    sync.Mutex
}

func newRingBuffer(size int) *ringBuffer {
	return &ringBuffer{lines: make([]string, size)}
}

func (rb *ringBuffer) Write(line string) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.lines[rb.head] = line
	rb.head = (rb.head + 1) % len(rb.lines)
	if rb.count < len(rb.lines) {
		rb.count++
	}
}

func (rb *ringBuffer) Lines() []string {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	out := make([]string, 0, rb.count)
	start := rb.head - rb.count
	if start < 0 {
		start += len(rb.lines)
	}
	for i := 0; i < rb.count; i++ {
		idx := (start + i) % len(rb.lines)
		out = append(out, rb.lines[idx])
	}
	return out
}

// Start launches the child MCP process, performs the initialize handshake,
// and fetches the child's tool list.
func Start(ctx context.Context, name, command string, args, env []string) (*Child, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	if len(env) > 0 {
		cmd.Env = env
	}

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", command, err)
	}

	c := &Child{
		Name:      name,
		Command:   command,
		Args:      args,
		Env:       env,
		cmd:       cmd,
		transport: mcp.NewTransport(stdoutPipe, stdinPipe),
		stdin:     stdinPipe,
		pending:   make(map[int64]chan *mcp.Response),
		done:      make(chan struct{}),
		stderrBuf: newRingBuffer(100),
	}

	// Drain stderr in background
	go c.drainStderr(stderrPipe)

	// Read responses in background
	go c.readLoop()

	// Initialize handshake
	if err := c.initialize(ctx); err != nil {
		c.Stop()
		return nil, fmt.Errorf("initialize child %s: %w", name, err)
	}

	// Fetch tools
	tools, err := c.fetchTools(ctx)
	if err != nil {
		c.Stop()
		return nil, fmt.Errorf("fetch tools from %s: %w", name, err)
	}
	c.Tools = tools

	return c, nil
}

func (c *Child) drainStderr(r io.Reader) {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			line := string(buf[:n])
			c.stderrBuf.Write(line)
			log.Printf("[child:%s:stderr] %s", c.Name, line)
		}
		if err != nil {
			return
		}
	}
}

func (c *Child) readLoop() {
	defer close(c.done)
	for {
		raw, err := c.transport.ReadMessage()
		if err != nil {
			if err != io.EOF {
				log.Printf("[child:%s] read error: %v", c.Name, err)
			}
			return
		}

		var resp mcp.Response
		if err := json.Unmarshal(raw, &resp); err != nil {
			log.Printf("[child:%s] invalid response: %v", c.Name, err)
			continue
		}

		// Notifications from child (no ID)
		if resp.ID == nil {
			continue
		}

		var id int64
		if err := json.Unmarshal(resp.ID, &id); err != nil {
			log.Printf("[child:%s] non-integer response id: %s", c.Name, resp.ID)
			continue
		}

		c.mu.Lock()
		ch, ok := c.pending[id]
		if ok {
			delete(c.pending, id)
		}
		c.mu.Unlock()

		if ok {
			ch <- &resp
		}
	}
}

func (c *Child) call(ctx context.Context, method string, params any) (*mcp.Response, error) {
	id := c.nextID.Add(1)
	paramsData, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	req := mcp.Request{
		JSONRPC: "2.0",
		ID:      mustMarshal(id),
		Method:  method,
		Params:  paramsData,
	}

	ch := make(chan *mcp.Response, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	if err := c.transport.WriteMessage(req); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("send %s: %w", method, err)
	}

	select {
	case resp := <-ch:
		if resp.Error != nil {
			return nil, fmt.Errorf("child error: %s (code %d)", resp.Error.Message, resp.Error.Code)
		}
		return resp, nil
	case <-c.done:
		return nil, fmt.Errorf("child %s exited", c.Name)
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	}
}

func (c *Child) initialize(ctx context.Context) error {
	params := map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]string{
			"name":    "mcp-supervisor",
			"version": "0.1.0",
		},
	}
	resp, err := c.call(ctx, "initialize", params)
	if err != nil {
		return err
	}
	_ = resp // we don't need the child's capabilities for now

	// Send initialized notification
	notif := mcp.Request{
		JSONRPC: "2.0",
		Method:  "initialized",
	}
	return c.transport.WriteMessage(notif)
}

func (c *Child) fetchTools(ctx context.Context) ([]mcp.Tool, error) {
	resp, err := c.call(ctx, "tools/list", struct{}{})
	if err != nil {
		return nil, err
	}
	var result mcp.ToolsListResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("parse tools/list: %w", err)
	}
	return result.Tools, nil
}

// CallTool forwards a tools/call to the child MCP.
func (c *Child) CallTool(ctx context.Context, params mcp.ToolCallParams) (mcp.ToolResult, error) {
	resp, err := c.call(ctx, "tools/call", params)
	if err != nil {
		return mcp.ToolResult{}, err
	}
	var result mcp.ToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return mcp.ToolResult{}, fmt.Errorf("parse tools/call result: %w", err)
	}
	return result, nil
}

// Pid returns the child process PID.
func (c *Child) Pid() int {
	if c.cmd.Process != nil {
		return c.cmd.Process.Pid
	}
	return 0
}

// Stop terminates the child process.
func (c *Child) Stop() {
	c.stdin.Close()
	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	_ = c.cmd.Wait()
}

// Stderr returns recent stderr output from the child.
func (c *Child) Stderr() []string {
	return c.stderrBuf.Lines()
}

func mustMarshal(v any) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}
