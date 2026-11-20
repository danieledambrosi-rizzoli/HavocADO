---
feature: print-instrumentation
status: active
owner: human
---

## Intent

Capture the teamserver's per-session AES material and the exact payload
packages it builds in a dedicated file under `teamserver/`, so an operator or
developer can inspect them after the fact without scraping terminal output.

The instrumentation originated in commit `2d0a982` (stdout prints). This change
moves the AES output into a file and adds the payload package to the same file.

## Scope (teamserver / client / payload / cross-boundary)

Teamserver only. No client, profile, or Demon change is involved.

Touched files:

- `teamserver/pkg/common/crypt/aes.go` — key/IV go to the file (stdout print
  removed).
- `teamserver/pkg/agent/agent.go` — payload package keeps its `logger.Debug`
  console line and is also written to the file.
- `teamserver/pkg/logger/file.go` — shared, concurrency-safe file writer.
- `.gitignore` — ignores the generated log directory.

## Contract

### Log location

```text
<teamserver>/logs/crypto.log
```

- Resolution (`logger.resolveTeamserverDir`): if the executable's directory
  contains a `teamserver/` directory, that is used (normal layout:
  `<repo>/havoc` and `<repo>/teamserver`). Otherwise the process walks up from
  the working directory looking for a `teamserver/` directory, which covers
  `go run` from `teamserver/`.
- Directory is created on first write with mode `0700`; the file is opened
  append-only with mode `0600` because it contains live session keys.
- One shared file for both components; a mutex serialises writes so a
  multi-line hex dump is never interleaved with another entry.

### Entry format

```text
YYYY-MM-DD HH:MM:SS [aes|agent] <message>
```

### `crypt.XCryptBytesAES256` (aes.go:12)

- Writes exactly two lines per call, after the CTR XOR and before returning:

  ```text
  2026-11-20 19:20:18 [aes] Encryption Key <64 lower-case hex chars>
  2026-11-20 19:20:18 [aes] IV Key <32 lower-case hex chars>
  ```

- Nothing is written to stdout by this function any more.
- The write is **unconditional**: it does not depend on `--debug`.
- AES-CTR is symmetric, so the same function is used for both encryption and
  decryption. The entries also appear when the teamserver **decrypts** an
  inbound agent buffer; the component tag does not distinguish direction.
- Every caller therefore logs: agent traffic, parser decryption, and packer
  output.

Call graph (non-test):

| Caller | File:line | Direction |
|---|---|---|
| `Agent.TaskDispatch` path via `BuildPayloadMessage` | `agent/agent.go:170` | encrypt outbound job data |
| `ParseDemonRegisterRequest` | `agent/agent.go:399` | decrypt `DEMON_INIT` body |
| `handlers.handleDemonAgent` | `handlers/handlers.go:102` | decrypt follow-up request body |
| `packer.(*Packer).Build` | `common/packer/packer.go:89` | encrypt when a key is set |
| `parser.(*Parser).DecryptBuffer` | `common/parser/parser.go:206` | generic decrypt |

### `agent.BuildPayloadMessage` (agent.go:29)

- The `logger.Debug("PayloadPackage:\n", hex.Dump(...))` console line is kept.
- The same dump is appended to the file, unconditionally:

  ```text
  2026-11-20 19:20:18 [agent] PayloadPackage:
  00000000  0a 00 00 00 00 00 00 00  00 00 00 00              |............|
  ```

- The file write is **not** gated by `--debug`; the console line still is.
- When the job carries data, the dump contains the AES-CTR-encrypted
  `DataPayload` (12-byte plaintext envelope: CommandID, RequestID, size, then
  ciphertext). When the job carries no data (`COMMAND_NOJOB`), no encryption
  happens and the dump is the 12-byte plaintext envelope only.

### When entries appear in practice

| Event | `[aes]` lines | `[agent]` dump |
|---|---|---|
| Teamserver startup | no | no |
| Payload generation in the client | no | no |
| New agent registration (`DEMON_INIT`) | yes, twice (decrypt body + encrypt reply) | no (registration reply is built by `Packer`) |
| Agent beacon with no queued job (`COMMAND_GET_JOB`) | yes, once (request decrypt) | yes, 12-byte NOJOB envelope |
| Agent beacon while a task is queued | yes, twice (request decrypt + job encrypt) | yes, includes ciphertext |
| Agent sends a task result | yes, once or more (request decrypt) plus response encryption | yes |
| SMB pivot traffic | yes | yes |

Not logged during: startup, profile parsing, payload building (`builder.go`
creates `packer.NewPacker(nil, nil)` at line 563, so `Packer.Build` returns
before the XOR), certificate generation, or client launch.

### `teamserver/cmd/client.go`

This file does **not** print or log an encrypted payload. It is a 26-line Cobra
command that calls `startMenu()` and then executes `client/Havoc`. The
encrypted-payload dump the original task referred to is the
`agent.BuildPayloadMessage` dump above, in `teamserver/pkg/agent/agent.go`.

## Profile impact

None. No profile key was added or changed. The log path is derived from the
repository layout, not from a profile setting.

## Verification

- Build gate: `.opencode/scripts/build-project.sh --target teamserver
  --no-setcap` succeeds and produces `./havoc`.
- Direct harness (`logger.FileLogPath`, `XCryptBytesAES256`,
  `BuildPayloadMessage`): wrote `[aes]` pairs and an `[agent]` dump to
  `/home/kali/workspace/Havoc/teamserver/logs/crypto.log`; directory `0700`,
  file `0600`.
- Live teamserver (`--profile`, `--debug`) with a crafted `DEMON_INIT` and
  `COMMAND_GET_JOB`:
  - `crypto.log` received three key/IV pairs (registration decrypt,
    registration reply encrypt, beacon decrypt) and the NOJOB payload dump;
  - the teamserver stdout contained **zero** `Encryption Key` / `IV Key`
    lines;
  - stdout still contained the `[DBUG] [agent.BuildPayloadMessage:176]`
    dump, confirming the console path was preserved.
- A payload with data produced a 33-byte package dump
  (`12-byte envelope + 21-byte ciphertext`) in the file.

## Open questions

- The file is append-only and never rotated; it grows for the life of the
  checkout and contains live session keys in clear text. Mode `0600` and the
  `.gitignore` entry reduce exposure, but this is intended for debugging only.
- This audit log is independent of the existing timestamped
  `data/loot/<timestamp>/teamserver.log` written by `pkg/logr`; the two are
  not merged.
- The directory name is `teamserver/logs` because the task asked for "a
  directory under teamserver". If a different name is required, only
  `teamserver/pkg/logger/file.go` needs to change.
- The original task description mentioned `client.go` printing the encrypted
  payload. The code does not do that; the dump is in `agent.go`, matching the
  commit message of `2d0a982`. Documented here as the code behaves.
