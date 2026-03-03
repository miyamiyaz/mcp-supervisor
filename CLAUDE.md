# mcp-supervisor

MCP server (stdio) that dynamically starts/stops/proxies other MCP servers. No external dependencies — standard library only.

## Architecture

```
cmd/supervisor-mcp/main.go  — entrypoint, tool definitions, dispatch
internal/mcp/               — MCP protocol (JSON-RPC 2.0 over stdio)
  types.go                  — request/response/tool types
  transport.go              — newline-delimited message reader/writer
  server.go                 — MCP server (initialize, tools/list, tools/call, ping)
internal/childmcp/          — child MCP process lifecycle
  child.go                  — start, initialize handshake, tool call forwarding, stop
internal/proxy/             — child MCP registry and tool routing
  proxy.go                  — start/stop/list MCPs, prefix tools, dispatch calls
```

## Build & run

```
mise run build        # go build -o supervisor-mcp ./cmd/supervisor-mcp
mise run lint         # go vet ./...
mise run release-dry  # goreleaser snapshot
```

## Key design decisions

- Child MCP tools are prefixed with `{name}.` (e.g. `pw.browser_click`) to avoid collisions.
- `notifications/tools/list_changed` is sent to the client when child MCPs start or stop.
- Child process env inherits parent env and merges overrides (never replaces).
- Debug logs go to stderr to avoid corrupting the stdio MCP protocol.
- Graceful shutdown on SIGINT/SIGTERM stops all children.

## Adding tools

Supervisor tools are defined in `cmd/supervisor-mcp/main.go` (`supervisorTools()` function). Each tool needs a name, description, and JSON Schema for `inputSchema`.

## Testing

No test framework yet. Build verification: `go build ./...`
