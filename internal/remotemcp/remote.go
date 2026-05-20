// Package remotemcp connects to MCP servers over Streamable HTTP (MCP 2025-03-26).
package remotemcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/miyamiyaz/mcp-supervisor/internal/mcp"
)

// Remote is a connection to a remote MCP server using Streamable HTTP transport.
type Remote struct {
	Name string
	URL  string

	headers    map[string]string
	httpClient *http.Client
	sessionID  string
	nextID     atomic.Int64
	tools      []mcp.Tool
}

// Connect initializes a Streamable HTTP session with a remote MCP server.
func Connect(ctx context.Context, name, url string, headers map[string]string) (*Remote, error) {
	r := &Remote{
		Name:       name,
		URL:        url,
		headers:    headers,
		httpClient: &http.Client{},
	}

	if err := r.initialize(ctx); err != nil {
		return nil, fmt.Errorf("initialize remote %s: %w", name, err)
	}

	tools, err := r.fetchTools(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch tools from %s: %w", name, err)
	}
	r.tools = tools

	return r, nil
}

func (r *Remote) post(ctx context.Context, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.URL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if r.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", r.sessionID)
	}
	for k, v := range r.headers {
		req.Header.Set(k, v)
	}
	return r.httpClient.Do(req)
}

func (r *Remote) call(ctx context.Context, method string, params any) (*mcp.Response, error) {
	id := r.nextID.Add(1)
	paramsData, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	reqMsg := mcp.Request{
		JSONRPC: "2.0",
		ID:      mustMarshal(id),
		Method:  method,
		Params:  paramsData,
	}
	body, err := json.Marshal(reqMsg)
	if err != nil {
		return nil, err
	}

	resp, err := r.post(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("post %s: %w", method, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("http %d for %s %s", resp.StatusCode, method, r.URL)
	}

	if method == "initialize" {
		if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
			r.sessionID = sid
		}
	}

	ct := resp.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "text/event-stream") {
		return readSSEResponse(resp.Body)
	}

	var mcpResp mcp.Response
	if err := json.NewDecoder(resp.Body).Decode(&mcpResp); err != nil {
		return nil, fmt.Errorf("decode %s response: %w", method, err)
	}
	if mcpResp.Error != nil {
		return nil, fmt.Errorf("remote error: %s (code %d)", mcpResp.Error.Message, mcpResp.Error.Code)
	}
	return &mcpResp, nil
}

// sendNotification fires a JSON-RPC notification (no ID). Errors are logged, not returned.
func (r *Remote) sendNotification(ctx context.Context, method string) {
	notif := mcp.Request{JSONRPC: "2.0", Method: method}
	body, err := json.Marshal(notif)
	if err != nil {
		return
	}
	resp, err := r.post(ctx, body)
	if err != nil {
		log.Printf("[remote:%s] notification %s: %v", r.Name, method, err)
		return
	}
	resp.Body.Close()
}

// readSSEResponse reads an SSE stream and returns the first JSON-RPC response with an ID.
func readSSEResponse(r io.Reader) (*mcp.Response, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // match transport.go
	var data strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "data: "):
			data.WriteString(strings.TrimPrefix(line, "data: "))
		case line == "" && data.Len() > 0:
			var resp mcp.Response
			if err := json.Unmarshal([]byte(data.String()), &resp); err == nil && resp.ID != nil {
				return &resp, nil
			}
			data.Reset()
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("sse read: %w", err)
	}
	return nil, fmt.Errorf("no response in SSE stream")
}

func (r *Remote) initialize(ctx context.Context) error {
	params := map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]string{"name": "mcp-supervisor", "version": "0.1.0"},
	}
	if _, err := r.call(ctx, "initialize", params); err != nil {
		return err
	}
	r.sendNotification(ctx, "notifications/initialized")
	return nil
}

func (r *Remote) fetchTools(ctx context.Context) ([]mcp.Tool, error) {
	resp, err := r.call(ctx, "tools/list", struct{}{})
	if err != nil {
		return nil, err
	}
	var result mcp.ToolsListResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("parse tools/list: %w", err)
	}
	return result.Tools, nil
}

// CallTool forwards a tool call to the remote MCP server.
func (r *Remote) CallTool(ctx context.Context, params mcp.ToolCallParams) (mcp.ToolResult, error) {
	resp, err := r.call(ctx, "tools/call", params)
	if err != nil {
		return mcp.ToolResult{}, err
	}
	var result mcp.ToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return mcp.ToolResult{}, fmt.Errorf("parse tools/call result: %w", err)
	}
	return result, nil
}

// Tools returns the tools available on this remote MCP server.
func (r *Remote) Tools() []mcp.Tool { return r.tools }

// Stop closes idle HTTP connections for this remote MCP.
func (r *Remote) Stop() {
	r.httpClient.CloseIdleConnections()
	log.Printf("[remote:%s] disconnected", r.Name)
}

// Info returns display info for list_mcps.
func (r *Remote) Info() map[string]any {
	return map[string]any{
		"name":      r.Name,
		"url":       r.URL,
		"transport": "http",
		"status":    "connected",
	}
}

func mustMarshal(v any) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}
