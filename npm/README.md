<p align="center">
  <img src="https://raw.githubusercontent.com/miyamiyaz/mcp-supervisor/main/icon.png" width="140" />
</p>

<h1 align="center">mcp-supervisor</h1>

<p align="center">MCP server that dynamically starts and manages other MCP servers.</p>

MCP client configs are static — if a child MCP server needs dynamic arguments (e.g. a CDP endpoint that changes every launch), you have to edit the config and restart your client. mcp-supervisor solves this by starting child MCP servers on demand and proxying their tools through a single connection.

## Install

```bash
npx mcp-supervisor
```

Or install globally:

```bash
npm install -g mcp-supervisor
```

## Configure

### Claude Code

```bash
claude mcp add supervisor -- npx mcp-supervisor
```

### Claude Desktop

Add to `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "supervisor": {
      "command": "npx",
      "args": ["mcp-supervisor"]
    }
  }
}
```

## Tools

| Tool | Description |
|---|---|
| `start_mcp` | Start a child MCP server and proxy its tools |
| `stop_mcp` | Stop a child MCP server and remove its tools |
| `list_mcps` | List all running child MCP servers |

When a child MCP is started with `name: "pw"`, all its tools become available with a `pw.` prefix (e.g. `pw.browser_click`).

## Documentation

See the [GitHub repository](https://github.com/miyamiyaz/mcp-supervisor) for full documentation, examples, and skills.

## License

MIT
