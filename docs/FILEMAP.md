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
