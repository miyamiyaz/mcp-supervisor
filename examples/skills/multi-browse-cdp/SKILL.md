---
name: multi-browse-cdp
description: Open multiple browser sessions via CDP endpoints for comparing pages side by side. Requires multiple Chrome instances running with --remote-debugging-port.
argument-hint: "[port1] [port2]"
---

## Setup

Launch two Playwright MCP instances through the supervisor:

1. `start_mcp(name="browser-$0", command="npx", args=["-y", "@playwright/mcp@latest", "--cdp-endpoint", "http://127.0.0.1:$0"])`
2. `start_mcp(name="browser-$1", command="npx", args=["-y", "@playwright/mcp@latest", "--cdp-endpoint", "http://127.0.0.1:$1"])`

Each browser's tools are prefixed: `browser-$0.browser_navigate`, `browser-$1.browser_navigate`, etc.

## Teardown

Stop both when done:
- `stop_mcp(name="browser-$0")`
- `stop_mcp(name="browser-$1")`

## Notes

- Both Chrome instances must already be running with their respective `--remote-debugging-port`.
