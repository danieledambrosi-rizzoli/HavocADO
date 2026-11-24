---
feature: havoc-mcp-server
status: shipped
owner: TBD
verified_against: HEAD 2026-11-24
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
- Listener records merge both package shapes: `HostBind`/`PortBind` first,
  then `BindHost`/`BindPort` (DNS), then `Host`/`Port` (service listeners)
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
| `start_listener` | no | `name`, `protocol`, `config` | Listener info |
| `edit_listener` | no | `name`, `config` | Listener info |
| `remove_listener` | no | `name` | Listener info |
| `generate_payload` | no | `agent_type`, `listener`, `arch`, `format`, `config` | `{agent_type, listener, arch, format, file_name, payload_b64, payload_size, status}` |

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
| `--allow-tools` | — | all (comma-separated list) |
| `--audit-log` | — | `stderr` (or file path) |
| `--max-output-bytes` | — | `262144` |
| `--no-reconnect` | — | `false` |

### Read-Only Mode

When `--read-only` is enabled, all tasking tools (`send_command`, `send_raw_command`, `start_listener`, `edit_listener`, `remove_listener`, `generate_payload`) are disabled. Only read-only tools (`teamserver_status`, `list_sessions`, `get_session`, `list_listeners`, `get_output`, `wait_for_output`, `get_task`) remain available.

The `--allow-tools` flag accepts a comma-separated list of tool names to explicitly allow, overriding the read-only restriction for those specific tools. For example: `--read-only --allow-tools=send_command,get_output`.

### Payload Generation (`generate_payload`)

The `generate_payload` tool generates a Demon agent payload (Windows EXE, DLL, shellcode, etc.) and returns the payload as a base64-encoded blob. This is a **perfect alternative to the GUI payload builder** — it uses the same teamserver-side builder (`pkg/common/builder`) and produces identical output.

**Arguments:**
- `agent_type` (string, enum): Currently only `"Demon"`
- `listener` (string): Name of an existing listener (must be running)
- `arch` (string, enum): `"x64"` or `"x86"`
- `format` (string, enum): One of:
  - `"Windows Exe"` — Standard Windows executable
  - `"Windows Service Exe"` — Windows service executable
  - `"Windows Dll"` — Windows DLL
  - `"Windows Reflective Dll"` — Reflective DLL
  - `"Windows Shellcode"` — Raw shellcode (binary)
- `config` (string): JSON configuration for the payload (sleep, jitter, injection, etc.)

**Returns:**
- `agent_type`, `listener`, `arch`, `format` — Echo of input
- `file_name` — Generated filename (e.g., `demon.x64.exe`)
- `payload_b64` — Base64-encoded payload bytes
- `payload_size` — Size in bytes
- `status` — `"generated"`

**Example config:**
```json
{
  "Sleep": "2",
  "Jitter": "15",
  "Sleep Technique": "WaitForSingleObjectEx",
  "Sleep Jmp Gadget": "jmp rax",
  "Stack Duplication": false,
  "Proxy Loading": "None (LdrLoadDll)",
  "Amsi/Etw Patch": "Hardware breakpoints",
  "Injection": {
    "Alloc": "Win32",
    "Execute": "Win32",
    "Spawn64": "C:\\Windows\\System32\\notepad.exe",
    "Spawn32": "C:\\Windows\\SysWOW64\\notepad.exe"
  }
}
```

The tool applies profile patches (from `Profile.Config.Demon.Binary`) automatically, just like the GUI.

### Audit Logging

The `--audit-log` flag controls audit logging destination:
- `stderr` (default): Write audit entries to stderr
- File path: Write audit entries to the specified file (append-only, mode 0600)

Audit log format (security-compliant, precise):
```
2026-11-23 15:04:05 [mcp-audit] user=mcp tool=send_command status=start args={"session_id":"abc123","command":"shell","args":{"command":"[REDACTED]"}
2026-11-23 15:04:06 [mcp-audit] user=mcp tool=send_command status=success args={"session_id":"abc123","command":"shell","args":{"command":"[REDACTED]"}
2026-11-23 15:04:07 [mcp-audit] user=mcp tool=generate_payload status=error args={"agent_type":"Demon","listener":"http","arch":"x64","format":"Windows Exe","config":"[REDACTED]"} error="listener not found"
```

Fields logged per entry:
- `timestamp` - ISO-like timestamp with second precision
- `user` - Operator username from teamserver authentication
- `tool` - MCP tool name invoked
- `status` - One of `start`, `success`, `error`
- `args` - JSON arguments with sensitive fields redacted (`password`, `secret`, `key`, `token`, `command_line`, `config`)
- `error` - Error message (only on `status=error`)

The audit log file is created with mode 0600 and parent directories with mode 0700, following the teamserver's security practices for sensitive logs (see `teamserver/pkg/logger/file.go`).

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
5. Verify read-only mode: `--read-only` should hide tasking tools from `tools/list`
6. Verify audit logging: `--audit-log /tmp/mcp-audit.log` should create file with entries on tool calls
7. Verify `--allow-tools` overrides read-only for specific tools

## Open questions

1. Default access: should `--read-only` be the default, requiring an explicit `--allow-tasking` flag?
2. Should MCP output include `HAVOC_CONSOLE_MESSAGE` (status noise) by default, or only agent output with an opt-in flag?
3. Should listener passwords (proxy credentials) be accepted as tool args, or only from a config file? Tool args end up in conversation logs.

## Resolved (this commit)

- ✅ `--allow-tools` now properly parses comma-separated tool names
- ✅ Audit logging to file implemented with security-compliant format (mode 0600, sensitive field redaction, precise status tracking)
- ✅ Read-only mode fully enforced at tool registration time
- ✅ `generate_payload` tool returns payload bytes directly (base64-encoded) instead of requiring `get_output` polling
- ✅ `generate_payload` uses the same teamserver-side builder as the GUI, ensuring identical output
- ✅ Profile patches (`Profile.Config.Demon.Binary`) applied automatically
- ✅ Integration test for `generate_payload` tool added