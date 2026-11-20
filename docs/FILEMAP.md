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
  starts with the build path and the print-instrumentation feature; extend it
  as more files are touched.

| path | language | responsibility | contract | read-with | last-verified-against |
|---|---|---|---|---|---|
| `makefile` | Make | Official build entrypoints (`ts-build`, `client-build`, cleanup) | `make ts-build` produces `./havoc`; `make client-build` produces `client/Havoc` with hardcoded `-j 4` | `docs/build-project.sh`, `teamserver/Install.sh` | `62e8de9` 2026-11-20 |
| `docs/build-project.sh` | Bash | Build teamserver and/or client with a configurable job count | Exit 0 only when requested binaries exist; falls back to `make dev-ts-compile` when sudo is unavailable; never prompts | `makefile` | `62e8de9` 2026-11-20 |
| `docs/FILEMAP.md` | Markdown | This map | One row per touched file; every touched file has a row in the same commit | `docs/features/INDEX.md` | `62e8de9` 2026-11-20 |
| `docs/features/INDEX.md` | Markdown | Feature navigation index | Every feature document has exactly one row with status and link | `docs/FILEMAP.md` | `62e8de9` 2026-11-20 |
| `docs/features/print-instrumentation.md` | Markdown | Canonical description of the AES key/IV and payload-dump instrumentation | Documents actual behavior: `fmt.Println` unconditional in `aes.go`, `logger.Debug` gated dump in `agent.go`; `cmd/client.go` prints nothing | `teamserver/pkg/common/crypt/aes.go`, `teamserver/pkg/agent/agent.go` | `62e8de9` 2026-11-20 |
| `teamserver/pkg/common/crypt/aes.go` | Go | AES-256-CTR primitive used for both directions | `XCryptBytesAES256(data, key, iv)` prints `Encryption Key <hex>` and `IV Key <hex>` to stdout on every call; returns empty slice on cipher error | `teamserver/pkg/agent/agent.go`, `teamserver/pkg/common/packer/packer.go` | `2d0a982` 2026-11-20 |
| `teamserver/pkg/agent/agent.go` | Go | Job packaging, header parsing, agent registration | `BuildPayloadMessage` encrypts non-empty job data and `logger.Debug`-dumps the full package at line 176; registration decrypts at line 399 | `teamserver/pkg/handlers/handlers.go`, `teamserver/pkg/common/crypt/aes.go` | `2d0a982` 2026-11-20 |
| `teamserver/cmd/client.go` | Go | `havoc client` Cobra command | Launches `client/Havoc` with inherited stdio; prints no payload or key material | `teamserver/cmd/server.go` | `62e8de9` 2026-11-20 |
| `teamserver/pkg/handlers/handlers.go` | Go | Demon request dispatch: decrypt request, build response | Decrypts at line 102 before dispatch; calls `BuildPayloadMessage` for NOJOB and queued jobs | `teamserver/pkg/agent/agent.go`, `teamserver/pkg/common/parser/parser.go` | `62e8de9` 2026-11-20 |
