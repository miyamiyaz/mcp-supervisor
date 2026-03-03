---
name: browse-cdp
description: Browse websites via Playwright MCP connected to Chrome's CDP endpoint. Requires Chrome running with --remote-debugging-port=9222.
argument-hint: "[url]"
---

## Setup

1. Use `start_mcp` to launch Playwright MCP with CDP connection:
   - name: "pw"
   - command: "npx"
   - args: ["-y", "@playwright/mcp@latest", "--cdp-endpoint", "http://127.0.0.1:9222"]

2. Once started, Playwright tools are available with the `pw.` prefix.

## Usage

- Navigate: use `pw.browser_navigate`
- Click: use `pw.browser_click`
- Type: use `pw.browser_type`
- Screenshot: use `pw.browser_screenshot`

## Teardown

When done browsing, call `stop_mcp` with name "pw".

## Notes

- Chrome must already be running with `--remote-debugging-port=9222`.
- If the user specifies a different port, adjust the `--cdp-endpoint` accordingly.
- Navigate to $ARGUMENTS if a URL is provided.
