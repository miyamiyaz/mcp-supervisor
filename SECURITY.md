# Security Policy

## Supported Versions

Security updates are provided for the latest minor release only.

| Version | Supported          |
| ------- | ------------------ |
| 0.5.x   | :white_check_mark: |
| < 0.5   | :x:                |

## Reporting a Vulnerability

If you discover a security vulnerability in mcp-supervisor, please **do not** open a public GitHub issue.

Instead, report it privately via [GitHub Security Advisories](https://github.com/miyamiyaz/mcp-supervisor/security/advisories/new).

You can expect:

- **Acknowledgement** within 3 business days.
- **Status update** within 7 days, confirming whether the report is accepted or declined.
- If accepted, a fix will be released as soon as possible, and you will be credited (unless you prefer to remain anonymous).

## Security Considerations

mcp-supervisor can start arbitrary processes on the host machine. It should only be run **locally on trusted machines** and must **never be exposed to untrusted networks**.
