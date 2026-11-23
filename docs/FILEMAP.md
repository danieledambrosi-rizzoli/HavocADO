# Filemap

A bounded, incrementally maintained per-file map. It exists so an agent can
locate the right file without filesystem exploration.

Format, one line per file:

```
path | language | responsibility | contract (what callers depend on) | read-with | last-verified-against
```

- **contract** is the most important field: record what must not break.
- **read-with** lists files that must be read together to avoid partial-context
  edits.
- Add an entry in the same commit as every file you create or modify. The map
  covers the build path and the print-instrumentation feature; extend it as
  more files are touched.

| path | language | responsibility | contract | read-with | last-verified-against |
|---|---|---|---|---|---|
| `makefile` | Make | Official build entrypoints (`ts-build`, `client-build`, cleanup) | `make ts-build` produces `./havoc`; `make client-build` produces `client/Havoc` with hardcoded `-j 4` | `.opencode/scripts/build-project.sh`, `teamserver/Install.sh` | `78ffe61` 2026-11-20 |
| `.opencode/scripts/build-project.sh` | Bash | Build teamserver and/or client with a configurable job count | Exit 0 only when requested binaries exist; falls back to `make dev-ts-compile` when sudo is unavailable; never prompts | `makefile` | `78ffe61` 2026-11-20 |
| `.gitignore` | Git config | Excludes build output, runtime data, and the key-bearing audit log | Must keep `teamserver/logs/` ignored; `client/Havoc`, `havoc`, and `client/Build/` stay ignored | `.opencode/scripts/INDEX.md` | `78ffe61` 2026-11-20 |
| `docs/FILEMAP.md` | Markdown | This map | One row per touched file; every touched file has a row in the same commit | `docs/features/INDEX.md` | `78ffe61` 2026-11-20 |
| `docs/features/INDEX.md` | Markdown | Feature navigation index | Every feature document has exactly one row with status and link | `docs/FILEMAP.md` | `78ffe61` 2026-11-20 |
| `docs/features/print-instrumentation.md` | Markdown | Canonical description of the AES key/IV and payload-dump file logging | Documents actual behavior: file-only `[aes]` entries in `aes.go`, console + file `[agent]` dumps in `agent.go`, shared `teamserver/logs/crypto.log`; `cmd/client.go` logs nothing | `teamserver/pkg/logger/file.go`, `teamserver/pkg/common/crypt/aes.go`, `teamserver/pkg/agent/agent.go` | `78ffe61` 2026-11-20 |
| `teamserver/pkg/logger/file.go` | Go | Shared append-only audit file writer | `FileLog(component, format, args...)` writes under `resolveTeamserverDir()/logs/crypto.log`, mode 0600, mutex-guarded, creates dirs on first use; `FileLogPath()` reports the absolute path | `teamserver/pkg/logger/logger.go`, `teamserver/pkg/logger/global.go` | `78ffe61` 2026-11-20 |
| `teamserver/pkg/common/crypt/aes.go` | Go | AES-256-CTR primitive used for both directions | `XCryptBytesAES256(data, key, iv)` writes `[aes] Encryption Key` and `[aes] IV Key` to the audit file on every call; writes nothing to stdout; returns empty slice on cipher error | `teamserver/pkg/logger/file.go`, `teamserver/pkg/agent/agent.go`, `teamserver/pkg/common/packer/packer.go` | `78ffe61` 2026-11-20 |
| `teamserver/pkg/agent/agent.go` | Go | Job packaging, header parsing, agent registration | `BuildPayloadMessage` encrypts non-empty job data, keeps the `logger.Debug` console dump, and also writes the dump to the audit file at line 177; registration decrypts at line 399 | `teamserver/pkg/handlers/handlers.go`, `teamserver/pkg/logger/file.go`, `teamserver/pkg/common/crypt/aes.go` | `78ffe61` 2026-11-20 |
| `teamserver/cmd/client.go` | Go | `havoc client` Cobra command | Launches `client/Havoc` with inherited stdio; logs no payload or key material | `teamserver/cmd/server.go` | `78ffe61` 2026-11-20 |
| `teamserver/pkg/handlers/handlers.go` | Go | Demon request dispatch: decrypt request, build response | Decrypts at line 102 before dispatch; calls `BuildPayloadMessage` for NOJOB and queued jobs | `teamserver/pkg/agent/agent.go`, `teamserver/pkg/common/parser/parser.go` | `78ffe61` 2026-11-20 |
| `.serena/project.yml` | YAML | Serena project config: enables cpp + go language servers | `language_servers: [cpp, go]`; `ls_workspace_folders: ["."]`; must stay in sync with repo languages | `.serena/project.local.yml` | `HEAD` 2026-11-23 |
| `docs/features/havoc-mcp-server.md` | Markdown | Canonical description of MCP stdio server for Havoc | Documents v1 tools, operator client contract, tasking mappings, CLI flags, profile requirements, read-only mode, audit logging, payload generation | `docs/features/INDEX.md`, `teamserver/cmd/mcp.go`, `teamserver/pkg/operator/*.go`, `teamserver/pkg/mcp/*.go` | `HEAD` 2026-11-24 |
| `teamserver/cmd/mcp.go` | Go | `havoc mcp` Cobra command; stdio MCP server entrypoint | Redirects logger to stdout early; registers flags with env var overrides; dials operator client, builds MCP server, serves stdio; parses comma-separated `--allow-tools` | `teamserver/cmd/cmd.go`, `teamserver/pkg/operator`, `teamserver/pkg/mcp` | `HEAD` 2026-11-23 |
| `teamserver/cmd/cmd.go` | Go | Root CLI and command registration | `HavocCli.AddCommand(CobraMcp)`; no `startMenu()` call for MCP | `teamserver/cmd/mcp.go`, `teamserver/cmd/server.go` | `HEAD` 2026-11-23 |
| `teamserver/pkg/operator/config.go` | Go | Operator client configuration and TLS setup | `Config` with URL, user, password, TLS options; `TLSConfig()` returns `*tls.Config` | `teamserver/pkg/operator/client.go` | `HEAD` 2026-11-23 |
| `teamserver/pkg/operator/types.go` | Go | Operator client state types and ring buffer | `Session`, `Listener`, `OutputEvent`, `OutputPage`, `Task`, `Payload`, `ringBuffer` | `teamserver/pkg/operator/client.go`, `teamserver/pkg/operator/operator.go` | `HEAD` 2026-11-24 |
| `teamserver/pkg/operator/packages.go` | Go | Havoc package encoding/decoding and auth helpers | `Package`, `Head`, `Body` structs; `LoginPackage`, `TaskPackage`, `HashPassword` (SHA3-256) | `teamserver/pkg/operator/client.go`, `teamserver/pkg/packager/types.go` | `HEAD` 2026-11-23 |
| `teamserver/pkg/operator/client.go` | Go | Headless operator WebSocket client | `Dial`, `Run`, `Close`, `Status`, `Sessions`, `Session`, `Listeners`, `GeneratePayload`; login, replay, reconnect, Gate event handling | `teamserver/pkg/operator/*.go`, `teamserver/cmd/server/teamserver.go` | `HEAD` 2026-11-24 |
| `teamserver/pkg/operator/tasking.go` | Go | Curated task constructors matching C++ client | `ShellTask`, `PowerShellTask`, `DirTask`, `CdTask`, `PwdTask`, `CatTask`, `DownloadTask`, `PsTask`, `KillTask`, `SleepTask`, `ExitTask`, `JobsTask`, `SendRawCommandTask` | `teamserver/pkg/operator/operator.go`, `client/src/Havoc/Demon/ConsoleInput.cc` | `HEAD` 2026-11-23 |
| `teamserver/pkg/operator/operator.go` | Go | Public operator API for MCP tools | `SendTask`, `ReadOutput`, `WaitOutput`, `GetTask`, `GeneratePayload`, `decodeOutput` | `teamserver/pkg/mcp/jsonrpc.go` | `HEAD` 2026-11-24 |
| `teamserver/pkg/mcp/config.go` | Go | MCP server configuration | `Config` with read-only, allowed-tools, audit-log, max-output-bytes | `teamserver/pkg/mcp/jsonrpc.go` | `HEAD` 2026-11-23 |
| `teamserver/pkg/mcp/jsonrpc.go` | Go | MCP protocol server, tool registry, all v1 handlers, audit logging | JSON-RPC 2.0 over stdio; `initialize`, `tools/list`, `tools/call`, `ping`; 14 tools (v1 + listener mgmt + payload gen); read-only enforcement; file audit logging with sensitive field redaction (mode 0600) | `teamserver/cmd/mcp.go`, `teamserver/pkg/operator/operator.go` | `HEAD` 2026-11-24 |
| `teamserver/cmd/server/dispatch.go` | Go | Teamserver event dispatch; handles Listener.Add/Edit/Remove from operators | Calls `ListenerStart` for new listeners; notifies via `ListenerStartNotify`; `ListenerRemove` for removal | `teamserver/cmd/server/listener.go`, `teamserver/cmd/server/teamserver.go`, `teamserver/pkg/events/listeners.go` | `56e4ec1` 2026-11-23 |
| `teamserver/cmd/server/teamserver.go` | Go | Teamserver core; sends listener/agent replay to new clients | `SendAllPackagesToNewClient` sends `t.Listeners` and active agents; `ClientAuthenticate` uses SHA3-256 | `teamserver/cmd/server/dispatch.go`, `teamserver/pkg/operator/client.go` | `56e4ec1` 2026-11-23 |
| `test/ephemeral/ephemeral.go` | Go | Ephemeral test directory utilities | `TempDir`, `TempDirWithFiles`, `CopyDir` with automatic cleanup via t.Cleanup; creates dirs in system temp with unique prefix | `test/ephemeral/ephemeral_test.go` | `HEAD` 2026-11-23 |
| `test/ephemeral/ephemeral_test.go` | Go | Tests for ephemeral utilities | Verifies temp dir creation, file population, and cleanup | `test/ephemeral/ephemeral.go` | `HEAD` 2026-11-23 |
| `test/unit/mcp_unit_test.go` | Go | MCP unit tests | Tests MCP config, JSON-RPC requests, tool registration logic, read-only enforcement | `teamserver/pkg/mcp/*.go` | `HEAD` 2026-11-23 |
| `test/unit/operator_unit_test.go` | Go | Operator unit tests | Tests operator config validation, password hashing (SHA3-256), task request preparation | `teamserver/pkg/operator/*.go` | `HEAD` 2026-11-23 |
| `test/unit/profile_unit_test.go` | Go | Profile parsing unit tests | Tests parsing havoc.yaotl, dns_example, http_smb_dns, webhook_example profiles | `teamserver/pkg/profile/profile.go`, `teamserver/profiles/*.yaotl` | `HEAD` 2026-11-23 |
| `test/integration/mcp_integration_test.go` | Go | MCP integration tests | Tests read-only mode, audit logging, --allow-tools override, teamserver_status, generate_payload tool; requires running teamserver | `teamserver/cmd/mcp.go`, `teamserver/pkg/mcp/*.go` | `HEAD` 2026-11-24 |
| `test/integration/beacon_test.go` | Go | Beacon/payload generation integration tests | Tests Windows EXE, DLL, shellcode, service EXE generation in ephemeral dirs; uses profile config | `teamserver/pkg/common/builder/builder.go`, `teamserver/pkg/profile/profile.go` | `HEAD` 2026-11-23 |
| `test/runner/main.go` | Go | Go-based test runner with JSON report | Runs all test packages, parses JSON output, generates test-report.json with pass/fail/skip counts | `test/ephemeral/`, `test/unit/`, `test/integration/` | `HEAD` 2026-11-23 |
| `test/run_tests.sh` | Bash | Shell test runner | Runs ephemeral, unit, integration, and core package tests; generates timestamped JSON report | `test/runner/main.go` | `HEAD` 2026-11-23 |
| `test/go.mod` | Go | Test module definition | Module Havoc/test with replace directive to teamserver for local testing | `teamserver/go.mod` | `HEAD` 2026-11-23 |
