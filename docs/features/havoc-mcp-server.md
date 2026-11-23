---
feature: havoc-mcp-server
status: shipped
owner: TBD
verified_against: 56e4ec1 2026-11-23
---

## Intent

Expose a Havoc teamserver to MCP-capable clients (editors, agents, local
assistants) so an operator can inspect and task sessions through MCP tools.

The MCP server is a **headless operator**, not a new teamserver. It connects to
a running teamserver over the existing `/havoc/` WebSocket protocol, authenticates
with its own operator credentials, reconstructs state from the login event
replay, and forwards tool calls as ordinary operator packages.

## Scope (teamserver / client / payload / cross-boundary)

Scope: **teamserver only**

- New command: `havoc mcp` (teamserver/cmd/mcp.go)
- New package: `teamserver/pkg/operator` (headless operator client)
- New package: `teamserver/pkg/mcp` (MCP protocol, tools)
- No client changes
- No payload changes
- No profile schema changes (uses existing operator block)

## Contract

### MCP Server (`havoc mcp`)

- Transport: JSON-RPC 2.0 over stdio (newline-delimited)
- Protocol version: `2025-06-18` (tools subset), compatible with `2024-11-05` and `2025-03-26`
- Methods: `initialize`, `notifications/initialized`, `tools/list`, `tools/call`, `ping`
- Error codes: `-32700` parse, `-32600` invalid request, `-32601` method not found, `-32602` invalid params, `-32603` internal
- Tool result format: `{"content":[{"type":"text","text":"..."}],"isError":false}`

### Operator Client (`pkg/operator`)

- Connects to `wss://<host>:<port>/havoc/` with TLS
- Auth: SHA3-256 hex of plaintext password (matches `teamserver/pkg/packager` logic)
- Replays `EventsList` on login: profile, listeners, active sessions
- Maintains per-session ring buffer of output events (default ~4MB)
- Exponential backoff reconnect (1s → 30s) unless `--no-reconnect`

### Tools (v1)

| Tool | Read-Only | Args | Returns |
|---|---|---|---|
| `teamserver_status` | yes | — | Connection, user, server, profile, last error |
| `list_sessions` | yes | — | Inventory of active sessions |
| `get_session` | yes | `session_id` | Session details or error |
| `list_listeners` | yes | — | Listener inventory |
| `send_command` | no | `session_id`, `command` (enum), `args` | `{task_id, cursor}` |
| `send_raw_command` | no | `session_id`, `command_id`, `info`, `command_line` | `{task_id, cursor}` |
| `get_output` | yes | `session_id`, `cursor`, `max_bytes` | Output page |
| `wait_for_output` | yes | `session_id`, `cursor`, `timeout_seconds` | Output page |
| `get_task` | yes | `task_id` | Task metadata + output since send |

### Command vocabulary (`send_command`)

`shell`, `powershell`, `dir`, `ls`, `cd`, `pwd`, `cat`, `download`, `ps`, `kill`, `sleep`, `exit`, `jobs`

Mappings match C++ client (`client/src/Havoc/Demon/ConsoleInput.cc`).

### CLI Flags

| Flag | Env | Default |
|---|---|---|
| `--teamserver` | `HAVOC_MCP_TEAMSERVER` | `wss://127.0.0.1:40056` |
| `--user` | `HAVOC_MCP_USER` | `mcp` |
| `--password` | `HAVOC_MCP_PASSWORD` | required |
| `--tls-ca` | `HAVOC_MCP_TLS_CA` | — |
| `--insecure` | — | `false` |
| `--read-only` | — | `false` |
| `--allow-tools` | — | all |
| `--audit-log` | — | `stderr` |
| `--max-output-bytes` | — | `262144` |
| `--no-reconnect` | — | `false` |

## Profile impact

Requires a dedicated operator entry in the profile:

```hcl
Operators {
    user "mcp" {
        Password = "change-me"
    }
}
```

The MCP operator name must be distinct from any connected GUI operator.

## Verification

1. Build: `make ts-build` (or `cd teamserver && go build ...`)
2. Unit tests: `go test -race ./pkg/operator/... ./pkg/mcp/...`
3. Manual E2E:
   ```bash
   ./havoc server --profile ./profiles/havoc.yaotl -v --debug
   # In another terminal (with "mcp" operator in profile):
   ./havoc mcp --teamserver wss://127.0.0.1:40056 --user mcp --password change-me --tls-ca data/server.cert
   ```
4. Verify `initialize`, `tools/list`, `teamserver_status`, `list_sessions`, `list_listeners`

## Open questions

1. Default access: should `--read-only` be the default, requiring an explicit `--allow-tasking` flag?
2. Should MCP output include `HAVOC_CONSOLE_MESSAGE` (status noise) by default, or only agent output with an opt-in flag?
3. Payload generation writes executables to disk. Is that in scope at all, and where should artifacts live (`data/loot/` vs a user-supplied directory)?
4. Should listener passwords (proxy credentials) be accepted as tool args, or only from a config file? Tool args end up in conversation logs.