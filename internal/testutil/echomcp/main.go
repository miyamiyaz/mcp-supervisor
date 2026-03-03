// echomcp is a minimal MCP server for testing. It exposes one tool ("echo")
// that returns its input arguments as text.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		var req request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			continue
		}
		// Notifications (no ID) — ignore
		if req.ID == nil {
			continue
		}

		var result json.RawMessage
		switch req.Method {
		case "initialize":
			result = mustJSON(map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
				"serverInfo":      map[string]any{"name": "echomcp", "version": "0.0.1"},
			})
		case "tools/list":
			result = mustJSON(map[string]any{
				"tools": []any{
					map[string]any{
						"name":        "echo",
						"description": "Returns its arguments as text",
						"inputSchema": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"message": map[string]any{"type": "string"},
							},
						},
					},
				},
			})
		case "tools/call":
			var params struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			}
			_ = json.Unmarshal(req.Params, &params)
			result = mustJSON(map[string]any{
				"content": []any{
					map[string]any{"type": "text", "text": string(params.Arguments)},
				},
			})
		case "ping":
			result = mustJSON(struct{}{})
		default:
			resp := response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &rpcError{Code: -32601, Message: fmt.Sprintf("method not found: %s", req.Method)},
			}
			writeLine(resp)
			continue
		}

		writeLine(response{JSONRPC: "2.0", ID: req.ID, Result: result})
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		fmt.Fprintf(os.Stderr, "echomcp: read error: %v\n", err)
		os.Exit(1)
	}
}

func writeLine(v any) {
	data, _ := json.Marshal(v)
	os.Stdout.Write(data)
	os.Stdout.Write([]byte("\n"))
}

func mustJSON(v any) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}
