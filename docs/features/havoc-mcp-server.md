---
feature: havoc-mcp-server
status: shipped
owner: Havoc teamserver maintainers
verified_against: feature/mcp-demon-command-tools 2026-11-26
---

## Intent

Expose a Havoc teamserver to MCP-capable clients (editors, agents, local
assistants) so an operator can inspect and task sessions through MCP tools.

The MCP server is a **headless operator**, not a new teamserver. It connects to
a running teamserver over the existing `/havoc/` WebSocket protocol,
authenticates with its own operator credentials, reconstructs state from the
login event replay, and forwards tool calls as ordinary operator packages.

This feature also provides a source-backed mapping from every Demon console
command to its MCP surface. Dedicated tools are used for commands whose
arguments or outputs need structured handling. Curated Class B commands remain
available through `send_command`, while Class C commands have both dedicated
tools and the existing `send_raw_command` escape hatch.

## Scope

Scope: **teamserver only**

- New command: `havoc mcp` (`teamserver/cmd/mcp.go`)
- New package: `teamserver/pkg/operator` (headless operator client)
- New package: `teamserver/pkg/mcp` (MCP protocol and tools)
- No client changes
- No payload or Demon changes
- No profile schema changes; the server uses the existing operator block

The command mapping changes are limited to `teamserver/pkg/operator` and
`teamserver/pkg/mcp`. No client, payload, Demon, listener, or profile packet
contract was changed. MCP shellcode tasks include the teamserver-consumed
singular `Argument` key and the client-compatible plural `Arguments` key so
non-empty arguments work with both existing decoders.

## Contract

### MCP Server (`havoc mcp`)

- Transport: JSON-RPC 2.0 over stdio (newline-delimited)
- Protocol version: `2025-06-18` (tools subset), compatible with
  `2024-11-05` and `2025-03-26`
- Methods: `initialize`, `notifications/initialized`, `tools/list`,
  `tools/call`, `ping`
- Error codes: `-32700` parse, `-32600` invalid request, `-32601` method not
  found, `-32602` invalid params, `-32603` internal
- Tool result format:
  `{"content":[{"type":"text","text":"..."}],"isError":false}`

### Operator Client (`pkg/operator`)

- Connects to `wss://<host>:<port>/havoc/` with TLS
- Authenticates with the SHA3-256 hex of the plaintext password
- Replays `EventsList` on login: profile, listeners, and active sessions
- Listener records merge both package shapes: `HostBind`/`PortBind` first,
  then `BindHost`/`BindPort` (DNS), then `Host`/`Port` (service listeners)
- Maintains a per-session output ring buffer (default approximately 4 MiB)
- Captures the output cursor before sending a task, so a fast agent response
  cannot be skipped by the wait path
- Reconnects with exponential backoff from 1 second to 30 seconds unless
  `--no-reconnect` is set
- Task constructors return `operator.TaskRequest`; the repository's tasking
  contract does not accept a `context.Context`

### Tools

The registry contains 29 tools: 7 read-only tools and 22 tasking or mutation
tools, including the raw-command escape hatch.

| Tool | Read-only | Args | Result |
|---|---|---|---|
| `teamserver_status` | yes | — | Connection, user, server, profile, last error |
| `list_sessions` | yes | — | Inventory of active sessions |
| `get_session` | yes | `session_id` | Session details or error |
| `list_listeners` | yes | — | Listener inventory |
| `get_output` | yes | `session_id`, `cursor`, `max_bytes` | Output page |
| `wait_for_output` | yes | `session_id`, `cursor`, `timeout_seconds` | Output page |
| `get_task` | yes | `task_id` | Task metadata and output since send |
| `send_command` | no | `session_id`, `command` (32-value enum), `args` | `{task_id, cursor}` |
| `send_raw_command` | no | `session_id`, `command_id`, `info`, `command_line` | `{task_id, cursor}` |
| `screenshot` | no | `session_id` | Task metadata, output events, screenshot payload metadata |
| `socks` | no | `session_id`, `action`, `param` | Task metadata and socket output |
| `rportfwd` | no | `session_id`, `action`, forwarding fields | Task metadata and socket output |
| `token` | no | `session_id`, `subcommand`, token fields | Task metadata and token-vault output |
| `shellcode` | no | `session_id`, `action`, `arch`, `pid`, `file_path`, `args` | Task metadata and injection output |
| `dotnet` | no | `session_id`, `action`, `assembly_path`, `args` | Task metadata and .NET output |
| `inline_execute` | no | `session_id`, `bof_path`, `args` | Task metadata and object-file output |
| `dll` | no | `session_id`, `action`, `pid`, `file_path`, `args` | Task metadata and reflective-DLL output |
| `download` | no | `session_id`, `path` | Task metadata, completion message, completed flag, teamserver path, limitation |
| `checkin` | no | `session_id` | Task metadata and check-in output |
| `job` | no | `session_id`, `action`, `job_id` | Task metadata and job-manager output |
| `proc` | no | `session_id`, `action`, process fields | Task metadata and process output |
| `spawn` | no | `session_id`, `state`, process fields | Task metadata and process-spawn output |
| `transfer` | no | `session_id`, `action`, `file_id` | Task metadata and transfer output |
| `net` | no | `session_id`, `action`, `target` | Task metadata and network output |
| `pivot` | no | `session_id`, `action`, pivot fields | Task metadata and pivot output |
| `start_listener` | no | `name`, `protocol`, `config` | Listener info |
| `edit_listener` | no | `name`, `config` | Listener info |
| `remove_listener` | no | `name` | Listener info |
| `generate_payload` | no | payload fields | Base64 payload and metadata |

### Class B command vocabulary (`send_command`)

`send_command` exposes these 31 Class B command names and aliases:

`shell`, `powershell`, `dir`, `ls`, `cd`, `cp`, `mv`, `move`, `remove`, `rm`,
`mkdir`, `pwd`, `cat`, `type`, `upload`, `ps`, `kill`, `sleep`, `interactive`,
`exit`, `jobs`, `config`, `luid`, `klist`, `purge`, `ptt`, `getprivs`, `whoami`,
`ipconfig`, `sysinfo`, `env`.

The enum also retains `download` for compatibility with the original v1 tool;
the dedicated `download` tool is the canonical interface and documents its
byte-stream limitation.

`whoami`, `ipconfig`, `sysinfo`, and `env` are shell aliases implemented with
`ShellTask`: `whoami`, `ipconfig /all`, `systeminfo`, and `set`, respectively.
`getprivs` uses `TokenTask` with `SubCommand=privs-list`. `rm` uses the FS
`remove` subcommand. `jobs` uses job `list`, and `kill` uses proc `kill`.

### Class A and Class C dedicated tools

Class A commands have dedicated tools because their inputs or outputs need
special handling: `dll`, `shellcode`, `token`, `inline-execute`, `dotnet`,
`rportfwd`, `socks`, `download`, and `screenshot`.

Class C commands have dedicated tools and remain available through
`send_raw_command`: `checkin`, `job`, `proc`, `spawn`, `transfer`, `net`, and
`pivot`.

Task command lines follow the source console patterns where the local artifact
path is known, including `shellcode`, `dotnet`, `inline-execute`, `dll`, and
`upload`. The wire payload still carries the file bytes rather than the local
path.

Every Class A and Class C handler sends the task and then waits for output.
Most handlers use `WaitOutput` for the first event. The dedicated `download`
handler uses a terminal-event wait so it does not return the "Started download"
message before the file is complete. Its result has this shape:

```json
{
  "task_id": "0123ABCD",
  "cursor": 42,
  "events": [],
  "next_cursor": 42,
  "truncated": false,
  "timed_out": false,
  "text": "Finished download of file: C:\\dir\\file.txt",
  "completed": true,
  "teamserver_path": "/srv/havoc/data/loot/run/agents/session/Download/C:/dir/file.txt",
  "limitation": "The operator WebSocket does not carry downloaded file bytes; retrieve the file from teamserver_path."
}
```

The output stream is team-wide and most agent messages do not carry a task
ID. An event returned after a send may therefore have originated from another
operator. `send_command` keeps the original immediate `{task_id, cursor}`
behavior; callers can use `wait_for_output` when they need the result.

### Binary output

Screenshot output is preserved as structured metadata by the operator layer.
The MCP event contains:

- `payload_b64`: base64-encoded screenshot bytes
- `payload_size`: decoded byte count
- `file_extension`: extension from the agent's `MiscData2` file name
- `content_type`: `image/bmp`

Screenshot bytes are never copied into the event's `text` field. The text is
the agent's status message.

Downloaded file bytes are a hard limitation: the operator WebSocket carries
the download status messages, not the file content. The dedicated `download`
tool waits for the terminal "Finished download of file" message and returns
that agent `Message`, a `completed` flag, an existing teamserver-side path
when it can be resolved, and an explicit `limitation` field. It never returns
`payload_b64` and never fabricates file bytes or a nonexistent path.

The teamserver writes normal agent downloads below its run's
`agents/<session>/Download` directory. `--loot-root` selects the directory
containing run artifacts and defaults to `data/loot`. Resolution supports the
normal nested Windows-path layout and the callback download layout that uses
the full remote path as one filename. The MCP process must be able to see the
configured loot directory; otherwise `teamserver_path` is empty and the
limitation states that the completed artifact was not visible.

### Read-only mode

When `--read-only` is enabled, every tool whose registry entry has
`ReadOnly: false` is omitted from `tools/list` and cannot be called. This
includes `send_command`, `send_raw_command`, all 16 dedicated Class A/C
tools, listener management, and payload generation.

`--allow-tools` restricts the registry to the named tools. It does not make a
tasking tool available while read-only mode is enabled.

### Audit logging

Audit logging remains centralized in `handleToolsCall`: each accepted tool call
records a `start`, `success`, or `error` entry. The audit file is append-only
with mode `0600`; parent directories use mode `0700`.

Sensitive arguments are flattened into top-level tool fields so the existing
redaction set catches them. In particular, `token.password` is exposed as the
top-level `password` argument and is recorded as `[REDACTED]`. The existing
redacted fields are `password`, `secret`, `key`, `token`, `command_line`, and
`config`.

### CLI flags

| Flag | Env | Default |
|---|---|---|
| `--teamserver` | `HAVOC_MCP_TEAMSERVER` | `wss://127.0.0.1:40056` |
| `--user` | `HAVOC_MCP_USER` | `mcp` |
| `--password` | `HAVOC_MCP_PASSWORD` | required |
| `--tls-ca` | `HAVOC_MCP_TLS_CA` | — |
| `--insecure` | — | `false` |
| `--read-only` | — | `false` |
| `--allow-tools` | — | all |
| `--audit-log` | — | `stderr` (or file path) |
| `--max-output-bytes` | — | `262144` |
| `--no-reconnect` | — | `false` |
| `--loot-root` | `HAVOC_MCP_LOOT_ROOT` | `data/loot` |

## Command inventory

The inventory contains 51 names: 47 resolved commands or aliases and 4
unresolved external names. Command IDs and `info` encodings come from the C++
dispatch path and the teamserver's `TaskPrepare` implementation.

| command name | source file:line | command ID | info argument pattern | output type | class |
|---|---|---:|---|---|---|
| sleep | `client/src/Havoc/Demon/ConsoleInput.cc:546` | 11 | `Arguments=delay;jitter` | text | B |
| interactive | `client/src/Havoc/Demon/ConsoleInput.cc:596` | 11 | `Arguments=0;0` | text | B |
| checkin | `client/src/Havoc/Demon/ConsoleInput.cc:607` | 100 | no extra fields | structured | C |
| job | `client/src/Havoc/Demon/ConsoleInput.cc:641` | 21 | `Command=list|suspend|resume|kill`, `Param=job-id` | structured | C |
| dir | `client/src/Havoc/Demon/ConsoleInput.cc:703` | 15 | `SubCommand=dir`, `Arguments=path;subdirs;filesOnly;dirsOnly;listOnly;starts;contains;ends` | text | B |
| ls | `client/src/Havoc/Demon/ConsoleInput.cc:703` | 15 | alias of `dir` | text | B |
| cd | `client/src/Havoc/Demon/ConsoleInput.cc:792` | 15 | `SubCommand=cd`, `Arguments=path` | text | B |
| cp | `client/src/Havoc/Demon/ConsoleInput.cc:806` | 15 | `SubCommand=cp`, `Arguments=base64(source);base64(destination)` | text | B |
| mv | `client/src/Havoc/Demon/ConsoleInput.cc:822` | 15 | `SubCommand=mv`, `Arguments=base64(source);base64(destination)` | text | B |
| move | `client/src/Havoc/Demon/ConsoleInput.cc:822` | 15 | alias of `mv` | text | B |
| remove | `client/src/Havoc/Demon/ConsoleInput.cc:838` | 15 | `SubCommand=remove`, `Arguments=path` | text | B |
| rm | `teamserver/pkg/operator/tasking.go:824` | 15 | alias of `remove` | text | B |
| mkdir | `client/src/Havoc/Demon/ConsoleInput.cc:852` | 15 | `SubCommand=mkdir`, `Arguments=path` | text | B |
| pwd | `client/src/Havoc/Demon/ConsoleInput.cc:866` | 15 | `SubCommand=pwd`, `Arguments=""` | text | B |
| shell | `client/src/Havoc/Demon/ConsoleInput.cc:872` | 4112 | `ProcCommand=4`, `Args=0;FALSE;TRUE;cmd.exe;base64(/c command)` | text | B |
| proc | `client/src/Havoc/Demon/ConsoleInput.cc:892` | 12 or 4112 | list: `FromProcessManager=false`; otherwise `ProcCommand=2|3|4|5|6|7`, typed `Args` | structured | C |
| ps | `client/src/Havoc/Demon/ConsoleInput.cc:1091` | 12 | `FromProcessManager=false` | text | B |
| dll | `client/src/Havoc/Demon/ConsoleInput.cc:1103` | 22 or 26 | inject: `Binary`, `Arguments`, `PID`; spawn: `Binary`, base64 `Arguments` | structured | A |
| shellcode | `client/src/Havoc/Demon/ConsoleInput.cc:1170` | 24 | `Way=Inject|Spawn|Execute`, `Technique`, `Binary`, teamserver `Argument` plus client-compatible `Arguments`, optional `PID`, `Arch` | text | A |
| token | `client/src/Havoc/Demon/ConsoleInput.cc:1304` | 40 | `SubCommand`, `Arguments`; make packs base64 domain, user, password, and logon type | structured | A |
| inline-execute | `client/src/Havoc/Demon/ConsoleInput.cc:1491` | 20 | `HasCallback=false`, `FunctionName=go`, `Binary`, base64 `Arguments`, `Flags=default` | text | A |
| dotnet | `client/src/Havoc/Demon/ConsoleInput.cc:1519` | 8193 or 8195 | inline: `Binary`, plain `Arguments`; versions: no extra fields | text | A |
| rportfwd | `client/src/Havoc/Demon/ConsoleInput.cc:1571` | 2540 | `Command=rportfwd add|list|remove|clear`, `Params=bind-host;bind-port;forward-host;forward-port` | structured | A |
| socks | `client/src/Havoc/Demon/ConsoleInput.cc:1631` | 2540 | `Command=socks add|list|kill|clear`, `Params=port|proxy-id` | structured | A |
| transfer | `client/src/Havoc/Demon/ConsoleInput.cc:1685` | 2530 | `Command=list|stop|resume|remove`, `FileID` | structured | C |
| download | `client/src/Havoc/Demon/ConsoleInput.cc:1740` | 15 | `SubCommand=download`, `Arguments=base64(path)` | binary (not carried by WebSocket) | A |
| cat | `client/src/Havoc/Demon/ConsoleInput.cc:1757` | 15 | `SubCommand=cat`, `Arguments=base64(path)` | text | B |
| type | `client/src/Havoc/Demon/ConsoleInput.cc:1757` | 15 | alias of `cat` | text | B |
| upload | `client/src/Havoc/Demon/ConsoleInput.cc:1774` | 15 | `SubCommand=upload`, `Arguments=base64(remote-path);base64(file-content)` | text | B |
| powershell | `client/src/Havoc/Demon/ConsoleInput.cc:1815` | 4112 | `ProcCommand=4`, `Args=0;FALSE;TRUE;powershell.exe;base64(-C command)` | text | B |
| config | `client/src/Havoc/Demon/ConsoleInput.cc:1833` | 2500 | `ConfigKey=key`, `ConfigVal=value` | text | B |
| screenshot | `client/src/Havoc/Demon/ConsoleInput.cc:2032` | 2510 | no extra fields | binary | A |
| net | `client/src/Havoc/Demon/ConsoleInput.cc:2039` | 2100 | `NetCommand=1|2|3|6|7|8|9`, `Param` | structured | C |
| pivot | `client/src/Havoc/Demon/ConsoleInput.cc:2134` | 2520 | `Command=1|10|11`, `Param` | structured | C |
| luid | `client/src/Havoc/Demon/ConsoleInput.cc:2187` | 2550 | `Command=luid` | text | B |
| klist | `client/src/Havoc/Demon/ConsoleInput.cc:2194` | 2550 | `Command=klist`, `Argument1=/all|/luid`, `Argument2=luid` | text | B |
| purge | `client/src/Havoc/Demon/ConsoleInput.cc:2236` | 2550 | `Command=purge`, `Argument=luid` | text | B |
| ptt | `client/src/Havoc/Demon/ConsoleInput.cc:2267` | 2550 | `Command=ptt`, base64 `Ticket`, `Luid` | text | B |
| exit | `client/src/Havoc/Demon/ConsoleInput.cc:2307` | 92 | `ExitMethod=thread|process` | text | B |
| jobs | `teamserver/pkg/operator/tasking.go:286` | 21 | alias of `job list`: `Command=list`, `Param=0` | text | B |
| kill | `teamserver/pkg/operator/tasking.go:239` | 4112 | alias of `proc kill`: `ProcCommand=7`, `Args=pid` | text | B |
| spawn | `teamserver/pkg/operator/tasking.go:366` | 4112 | raw proc-create encoding: `ProcCommand=4`, `Args=flags;verbose;piped;program;base64(args)` | structured | C |
| getprivs | `teamserver/pkg/operator/tasking.go:573` | 40 | alias of `token privs-list`: `SubCommand=privs-list`, `Arguments=""` | text | B |
| whoami | `teamserver/pkg/operator/tasking.go:95` | 4112 | shell alias: `ProcCommand=4`, base64 `/c whoami` | text | B |
| ipconfig | `teamserver/pkg/operator/tasking.go:95` | 4112 | shell alias: `ProcCommand=4`, base64 `/c ipconfig /all` | text | B |
| sysinfo | `teamserver/pkg/operator/tasking.go:95` | 4112 | shell alias: `ProcCommand=4`, base64 `/c systeminfo` | text | B |
| env | `teamserver/pkg/operator/tasking.go:95` | 4112 | shell alias: `ProcCommand=4`, base64 `/c set` | text | B |
| hashdump | external reference only | UNRESOLVED | UNRESOLVED | UNRESOLVED | unresolved |
| lsass | external reference only | UNRESOLVED | UNRESOLVED | UNRESOLVED | unresolved |
| spawnas | external reference only | UNRESOLVED | UNRESOLVED | UNRESOLVED | unresolved |
| mimi | external reference only | UNRESOLVED | UNRESOLVED | UNRESOLVED | unresolved |

`help`, `task`, `__debug`, and `clear` were checked but excluded: they are
client or teamserver UI commands, not Demon agent commands.

The four unresolved names have no source-backed command ID or `info` encoding
in this fork. No constructor or tool was written for them.

## OpenCode integration

Build the teamserver binary, start a teamserver with a dedicated `mcp`
operator, export the password in the shell, and add the local MCP server to
the project `.opencode/opencode.json`:

```jsonc
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "servers": {
      "havoc": {
        "type": "local",
        "command": [
          "./havoc",
          "mcp",
          "--teamserver",
          "wss://127.0.0.1:40056",
          "--loot-root",
          "data/loot",
          "--user",
          "mcp",
          "--tls-ca",
          "data/server.cert"
        ],
        "cwd": ".",
        "environment": {
          "HAVOC_MCP_PASSWORD": "{env:HAVOC_MCP_PASSWORD}"
        }
      }
    }
  }
}
```

OpenCode starts local MCP servers automatically and exposes their tools as
`havoc_<tool>`, for example `havoc_send_command` and `havoc_screenshot`.
Keep the password in the environment rather than in the configuration file.
Set `--loot-root` (or `HAVOC_MCP_LOOT_ROOT`) to the teamserver's `data/loot`
directory when the MCP process does not run from the teamserver working
directory. Use a distinct teamserver operator name for the MCP client so it
does not collide with a connected GUI operator.

For a restricted setup, add `--read-only` to the command or use OpenCode
permission rules to deny normalized `havoc_*` tools. The MCP server's own
read-only mode is the stronger control because it removes tasking tools from
the registry.

## Profile impact

No profile schema changes are required. The teamserver profile needs a
dedicated operator entry:

```hcl
Operators {
    user "mcp" {
        Password = "change-me"
    }
}
```

The MCP operator name must be distinct from any connected GUI operator.

## Verification

1. Build: `cd teamserver && go build ./...`
2. Race tests: `cd teamserver && go test -race ./pkg/operator/... ./pkg/mcp/...`
3. Constructor coverage: the operator test invokes all 39 exported task
   constructors and verifies every result is a populated `TaskRequest`.
4. Tool registry: the MCP test verifies 29 registered tools, the exact 16
   dedicated Class A/C tools, and the exact 7 read-only tools.
5. Command enum: the MCP test verifies the complete 32-value `send_command`
   vocabulary, including every Class B alias.
6. Validation: tests reject unknown dedicated-tool actions, malformed
   `send_command` arguments, invalid PTT base64, and raw commands without
   required info.
7. Output contract: tests verify screenshot preservation and payload metadata,
   ordinary output decoding, pre-send cursor capture, terminal download waits,
   existing-file path discovery, the explicit download limitation, and the
   absence of fabricated download bytes.
8. Audit contract: the MCP test invokes the centralized tool-call path and
   verifies start/success entries plus top-level password redaction.
9. Structural checks:
   - `grep -c '^func [A-Za-z0-9_]*Task(' teamserver/pkg/operator/tasking.go`
     returns 41, including two internal helpers.
   - `grep -c 'Task(ctx' teamserver/pkg/operator/tasking.go` returns 0. The
     repository's constructors do not take `context.Context`; the equivalent
     coverage is the 39-constructor test above.
10. Serena diagnostics are clean for every edited Go file.
11. The repository `make ts-build` gate reaches the final privileged
    `sudo setcap cap_net_bind_service=+ep ./havoc` step, then fails because this
    environment has no password-capable sudo TTY. `make dev-ts-compile` and the
    direct Go build above pass.
12. A live Demon session was not available in the build environment. Screenshot
    metadata and download completion are covered by source-derived unit tests;
    download byte carriage is documented as unsupported rather than inferred.

## Open questions

1. `hashdump`, `lsass`, `spawnas`, and `mimi` remain unresolved until a source
   or protocol definition for this fork is available.
2. The agent names screenshot files with a `.png` suffix while the required MCP
   content type is `image/bmp`. The current contract preserves the source
   extension in `file_extension` and sets `content_type` to `image/bmp`.
3. The download tool resolves the teamserver-side artifact through the
   configurable `--loot-root` and returns only an existing file. Carrying the
   authoritative absolute path in the teamserver event would require a packet
   contract change and is outside this feature's scope; a remote MCP process
   still needs access to the configured loot directory.
