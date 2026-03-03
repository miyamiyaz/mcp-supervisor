package mcp

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
)

// ToolHandler is called when a tools/call request arrives.
type ToolHandler func(params ToolCallParams) (ToolResult, error)

// ToolsProvider returns the current list of tools.
type ToolsProvider func() []Tool

// Server implements an MCP server over stdio.
type Server struct {
	transport     *Transport
	toolsProvider ToolsProvider
	toolHandler   ToolHandler
	info          ServerInfo
}

func NewServer(transport *Transport, info ServerInfo, tp ToolsProvider, th ToolHandler) *Server {
	return &Server{
		transport:     transport,
		info:          info,
		toolsProvider: tp,
		toolHandler:   th,
	}
}

// NotifyToolsChanged sends notifications/tools/list_changed to the client.
func (s *Server) NotifyToolsChanged() error {
	return s.transport.WriteMessage(Notification{
		JSONRPC: "2.0",
		Method:  "notifications/tools/list_changed",
	})
}

// Serve reads requests from transport and dispatches them. Blocks until EOF or error.
func (s *Server) Serve() error {
	for {
		raw, err := s.transport.ReadMessage()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		var req Request
		if err := json.Unmarshal(raw, &req); err != nil {
			log.Printf("invalid json-rpc message: %v", err)
			continue
		}

		// Notifications (no ID) — just acknowledge
		if req.ID == nil {
			continue
		}

		resp, err := s.dispatch(req)
		if err != nil {
			s.sendError(req.ID, -32603, err.Error())
			continue
		}
		if err := s.transport.WriteMessage(resp); err != nil {
			return fmt.Errorf("send response: %w", err)
		}
	}
}

func (s *Server) dispatch(req Request) (*Response, error) {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolsCall(req)
	case "ping":
		return s.handlePing(req)
	default:
		return nil, fmt.Errorf("method not found: %s", req.Method)
	}
}

func (s *Server) handleInitialize(req Request) (*Response, error) {
	result := InitializeResult{
		ProtocolVersion: "2024-11-05",
		Capabilities: ServerCapabilities{
			Tools: &ToolsCapability{ListChanged: true},
		},
		ServerInfo: s.info,
	}
	return jsonResponse(req.ID, result)
}

func (s *Server) handlePing(req Request) (*Response, error) {
	return jsonResponse(req.ID, struct{}{})
}

func (s *Server) handleToolsList(req Request) (*Response, error) {
	result := ToolsListResult{Tools: s.toolsProvider()}
	return jsonResponse(req.ID, result)
}

func (s *Server) handleToolsCall(req Request) (*Response, error) {
	var params ToolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil, fmt.Errorf("invalid tools/call params: %w", err)
	}
	result, err := s.toolHandler(params)
	if err != nil {
		result = ToolResult{
			Content: []Content{{Type: "text", Text: err.Error()}},
			IsError: true,
		}
	}
	return jsonResponse(req.ID, result)
}

func (s *Server) sendError(id json.RawMessage, code int, msg string) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &RPCError{Code: code, Message: msg},
	}
	_ = s.transport.WriteMessage(resp)
}

func jsonResponse(id json.RawMessage, result any) (*Response, error) {
	data, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return &Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  data,
	}, nil
}
