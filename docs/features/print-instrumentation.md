---
feature: print-instrumentation
status: active
owner: human
---

## Intent

Expose the teamserver's per-session AES material and the exact bytes it sends
to an agent, so the operator can correlate teamserver output with on-target
traffic while debugging transport or crypto problems.

The instrumentation was added in commit `2d0a982` ("human: edited aes.go and
agent.go to print encrypted payload and the cryptographic keys").

## Scope (teamserver / client / payload / cross-boundary)

Teamserver only. No client, profile, or Demon change is involved.

Instrumented files:

- `teamserver/pkg/common/crypt/aes.go` — key/IV print in `XCryptBytesAES256`.
- `teamserver/pkg/agent/agent.go` — payload-package hex dump in
  `BuildPayloadMessage`.

## Contract

### `crypt.XCryptBytesAES256` (aes.go:12)

- Writes exactly two lines to **stdout**, once per call, after the CTR XOR and
  before returning:

  ```text
  Encryption Key <64 lower-case hex chars>
  IV Key <32 lower-case hex chars>
  ```

- The print is **unconditional**: it uses `fmt.Println`, not `logger.Debug`, so
  it appears even without `--debug`.
- AES-CTR is symmetric, so the same function is used for both encryption and
  decryption. The lines therefore also appear when the teamserver **decrypts**
  an inbound agent buffer; the labels do not distinguish the direction.
- Because it sits inside the primitive, every caller prints: agent traffic,
  parser decryption, and packer output. A single HTTP request normally prints
  the pair twice (once decrypting the request, once encrypting the response
  if the response carries data).

Call graph (non-test):

| Caller | File:line | Direction |
|---|---|---|
| `Agent.TaskDispatch` path via `BuildPayloadMessage` | `agent/agent.go:170` | encrypt outbound job data |
| `ParseDemonRegisterRequest` | `agent/agent.go:399` | decrypt `DEMON_INIT` body |
| `handlers.handleDemonAgent` | `handlers/handlers.go:102` | decrypt follow-up request body |
| `packer.(*Packer).Build` | `common/packer/packer.go:89` | encrypt when a key is set |
| `parser.(*Parser).DecryptBuffer` | `common/parser/parser.go:206` | generic decrypt |

### `agent.BuildPayloadMessage` (agent.go:29)

- After assembling the package it logs, through `logger.Debug`, the whole
  `PayloadPackage`:

  ```text
  [hh:mm:ss] [DBUG] [agent.BuildPayloadMessage:176]: PayloadPackage:
   00000000  ...  |....|
  ```

- The dump appears **only when the teamserver runs with `--debug`**
  (`logger.SetDebug(true)` in `teamserver/cmd/server.go:58`); `logger.Debug` is
  a no-op otherwise.
- When the job carries data, the dumped package contains the AES-CTR-encrypted
  `DataPayload` (12-byte plaintext envelope: CommandID, RequestID, size, then
  ciphertext). When the job carries no data (`COMMAND_NOJOB`), no encryption
  happens and the dump is the 12-byte plaintext envelope only.

### When the output appears in practice

| Event | Key/IV lines | `PayloadPackage` dump |
|---|---|---|
| Teamserver startup | no | no |
| Payload generation in the client | no | no |
| New agent registration (`DEMON_INIT`) | yes, twice (decrypt body + encrypt reply) | no (registration reply is built by `Packer`, not `BuildPayloadMessage`) |
| Agent beacon with no queued job (`COMMAND_GET_JOB`) | yes, once (request decrypt) | yes, 12-byte NOJOB envelope |
| Agent beacon while a task is queued | yes, twice (request decrypt + job encrypt) | yes, includes ciphertext |
| Agent sends a task result | yes, once or more (request decrypt) plus response encryption | yes |
| SMB pivot traffic | yes | yes |

Not printed during: startup, profile parsing, payload building (`builder.go`
creates `packer.NewPacker(nil, nil)` at line 563, so `Packer.Build` returns
before the XOR), certificate generation, or client launch.

### `teamserver/cmd/client.go`

This file does **not** print an encrypted payload. It is a 26-line Cobra
command that calls `startMenu()` and then executes `client/Havoc`. The
encrypted-payload dump the task refers to is the `agent.BuildPayloadMessage`
dump above, in `teamserver/pkg/agent/agent.go`.

## Profile impact

None. No profile key was added or changed.

## Verification

- Build gate: `docs/build-project.sh` (teamserver via `make dev-ts-compile`
  when sudo is unavailable, client via the `make client-build` steps with a
  configurable `-j`) completes and both `./havoc` and `client/Havoc` exist.
- Unit-level: calling `BuildPayloadMessage` with a 21-byte job payload printed
  the key/IV pair and a 33-byte package dump
  (`12-byte envelope + 21-byte ciphertext`).
- Live: teamserver started with `--debug`; a crafted `DEMON_INIT` plus a
  `COMMAND_GET_JOB` beacon produced, in order:
  - registration decrypt: `Encryption Key` + `IV Key`;
  - registration reply encrypt: `Encryption Key` + `IV Key`;
  - beacon decrypt: `Encryption Key` + `IV Key`;
  - `[DBUG] [agent.BuildPayloadMessage:176]: PayloadPackage:` followed by the
    12-byte NOJOB dump.
- Startup emitted neither line.

## Open questions

- The task description mentions `client.go` printing the encrypted payload.
  The code does not do that; the dump is in `agent.go`, which matches the
  commit message of `2d0a982`. Documented here as the code behaves; the
  description is not silently "fixed".
- The print uses `fmt.Println` inside a cryptographic primitive. It cannot be
  disabled without recompiling and it leaks session keys to stdout/logs. This
  is accepted for the current debugging task only.
- `docs/build-project.sh` is stored in `docs/` because the task explicitly
  requested that location; AGENTS.md §5.9 normally places reusable scripts in
  `.opencode/scripts/`. The script is listed in `.opencode/scripts/INDEX.md`
  and in `docs/FILEMAP.md` so it remains discoverable.
