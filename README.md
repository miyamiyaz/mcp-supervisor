<h4 align="center">MCP server that dynamically starts and manages other MCP servers</h4>

<h1 align="center">
  <img src="icon.png" width="180"/>
  <br/>
  MCP Supervisor
</h1>

<p align="center">
  <a href="#why">Why</a> ⚙
  <a href="#install">Install</a> ⚙
  <a href="#configure">Configure</a> ⚙
  <a href="#skills">Skills</a> ⚙
  <a href="#tools">Tools</a> ⚙
  <a href="https://modelcontextprotocol.io">About MCP ↗</a>
</p>

<p align="center">
  <a href="https://github.com/miyamiyaz/mcp-supervisor/actions/workflows/ci.yml"><img src="https://github.com/miyamiyaz/mcp-supervisor/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/miyamiyaz/mcp-supervisor/releases/latest"><img src="https://img.shields.io/github/v/release/miyamiyaz/mcp-supervisor?logo=github&color=22ff22" alt="latest release"></a>
  <a href="https://github.com/miyamiyaz/mcp-supervisor/blob/main/LICENSE"><img src="https://img.shields.io/github/license/miyamiyaz/mcp-supervisor" alt="license"></a>
</p>

## Why

MCP client configs are static. If a child MCP server needs dynamic arguments (e.g. a CDP endpoint that changes every launch), you have to edit the config and restart your client every time.

`mcp-supervisor` solves this. Configure it once as your MCP server. It starts child MCP servers on demand with whatever arguments you need, and proxies their tools through a single fixed connection.

Also, statically configured MCP servers load all their tools into context at startup — even when you don't need them. With mcp-supervisor, child MCPs are started only when needed and stopped when done, keeping your context window clean.

## Install

### Coding agent (Claude Code, Codex, OpenCode, etc.)

Paste this into your agent:

> Install mcp-supervisor using https://github.com/miyamiyaz/mcp-supervisor and register it with `claude mcp add supervisor -- npx -y mcp-supervisor`

### npx (recommended)

No install needed — runs directly via Node.js:

```bash
claude mcp add supervisor -- npx -y mcp-supervisor
```

Or add to `.mcp.json`:

```json
{
  "mcpServers": {
    "supervisor": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "mcp-supervisor"]
    }
  }
}
```

### mise

```bash
mise use -g "github:miyamiyaz/mcp-supervisor[exe=supervisor-mcp]"
```

### GitHub Releases

Download a binary from [GitHub Releases](https://github.com/miyamiyaz/mcp-supervisor/releases) and place it somewhere in your PATH.

### go install

```bash
go install github.com/miyamiyaz/mcp-supervisor/cmd/supervisor-mcp@latest
```

## Configure

### Claude Code

```bash
claude mcp add supervisor -- npx -y mcp-supervisor
```

### Claude Desktop

Add to `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "supervisor": {
      "command": "npx",
      "args": ["-y", "mcp-supervisor"]
    }
  }
}
```

## Skills

mcp-supervisor is designed to be used with [Claude Code skills](https://code.claude.com/docs/en/skills.md). A skill teaches Claude how to launch and use specific MCP servers through the supervisor.

Give Claude the MCP's GitHub URL and ask it to create a skill:

> Create a skill for mcp-supervisor using https://github.com/microsoft/playwright-mcp — put it in .claude/skills/browse-cdp/SKILL.md

Claude will read the repo, understand the MCP's tools and arguments, and generate the skill file for you.

See [`examples/skills/`](examples/skills/) for ready-made examples.

## Tools

| Tool | Description |
|---|---|
| `start_mcp` | Start a child MCP server and proxy its tools |
| `stop_mcp` | Stop a child MCP server and remove its tools |
| `list_mcps` | List all running child MCP servers |

When a child MCP is started with `name: "pw"`, all its tools become available with a `pw.` prefix (e.g. `pw.browser_click`, `pw.browser_navigate`).

### start_mcp

Exactly one of `command` (stdio) or `url` (HTTP) must be provided.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Unique name (used as tool prefix) |
| `command` | string | one of | Command to run (stdio transport) |
| `args` | string[] | no | Command arguments (stdio only) |
| `env` | object | no | Extra env vars, merged with parent (stdio only) |
| `url` | string | one of | HTTP endpoint URL (Streamable HTTP transport) |
| `headers` | object | no | HTTP headers, e.g. `{"Authorization": "Bearer <token>"}` |

### stop_mcp

| Parameter | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Name of the MCP to stop |

### list_mcps

No parameters.

## Security

This server can start arbitrary processes. Run only locally on trusted machines. Do not expose to networks.

## License

MIT
