---
feature: dns-c2
status: active
owner: Havoc Team
---

## Intent

Carry Demon's existing transport messages over DNS TXT queries so an operator
can run C2 through a resolver path that is already allowed out of most
networks.

## Scope (teamserver / client / payload / cross-boundary)

Cross-boundary: teamserver, client, and Demon all participate.

- Teamserver: `LISTENER_DNS` handler, profile block, listener events,
  client-package dispatch, DB restore, payload builder.
- Client: listener dialog page, listener table row, listener package parsing.
- Demon: `TRANSPORT_DNS` compile-time transport using the system resolver
  (`DnsQuery_A`).

## Contract

### Profile

```yaotl
Listeners {
    Dns {
        Name         = "DNS C2"
        Domain       = "c2.example.com"
        BindHost     = "0.0.0.0"   # optional, default 0.0.0.0
        BindPort     = 53          # optional, default 53
        RecordType   = "TXT"       # optional, only TXT
        Encoding     = "base64url" # optional, only base64url
        Frequency    = 30          # optional, seconds, clamped 5..300
        KillDate     = ""          # optional
        WorkingHours = ""          # optional
    }
}
```

`Domain` is stored lower-case with the trailing dot removed. `RecordType`
accepts empty or `TXT`; any other value fails the listener start. `Encoding`
accepts empty, `base64` (legacy spelling) or `base64url`; the value is always
`base64url` after normalization. `Start()` returns an error on a bind failure,
so the listener is not registered and the client receives a listener error.

Both UDP and TCP are bound on `BindHost:BindPort`.

### Wire format

One agent message is base64url-encoded (RFC 4648 §5, no padding) and split
into chunks of at most 180 characters. Chunk `seq` of `total` is sent as a
TXT query named:

```
<seq>.<total>.<token>.<label>.<label>...<domain>
```

- `seq` is 0-based; `total` is 1-based.
- `token` is exactly 8 lower-case hex characters. The agent derives it from
  the agent id and a message counter that only advances after a message is
  answered, so a retry of the same message reuses the token.
- `label` is at most 63 characters; the server joins all labels after the
  token before decoding.
- A chunk query contains at most 180 base64url characters.
- The whole name stays under 253 bytes; the agent shrinks the chunk size for
  long domains.

The teamserver reassembles chunks per token with a 60-second inactivity
expiry, and limits a message to 512 chunks. Incomplete or expired buffers are
discarded. The reassembled bytes are handled by the normal
`parseAgentRequest` path, so DNS messages carry the exact same
`DEMON_COMMAND_GET_JOB` / task payloads as HTTP.

The response is base64url-encoded and returned as TXT strings on the query
that completed the message. Each TXT string holds at most 200 characters, so
a response larger than one string is split across several strings of the same
record. UDP replies are truncated to the request's EDNS0 buffer size (512
bytes minimum) with TC set, which makes the system resolver retry over TCP;
the TCP server runs the same handler.

The teamserver caches the encoded response per token for `Frequency` seconds.
This makes a re-query of the last chunk idempotent: the cached answer is
returned instead of executing the task again.

### Agent

- `TRANSPORT_DNS` is emitted by `GetListenerDefines`; `PatchConfig` appends
  `Domain` (wide string), `Frequency` (int32), `KillDate` (int64),
  `WorkingHours` (int32), in that order, after the common config block.
- The agent resolves TXT records through `DnsQuery_A` with
  `DNS_QUERY_BYPASS_CACHE`, so the local resolver cache cannot swallow a
  chunk or a response.
- Chunks are sent sequentially; the last chunk is re-queried up to three
  times if no answer was received.
- `DownloadChunkSize` is 32 KiB and a single request is capped at 64 KiB raw
  (below the teamserver's 512 × 180 character reassembly ceiling). Larger
  packages are dropped with a `DEMON_PACKAGE_DROPPED` notice, or stay queued
  when the aggregate request would exceed the cap.
- `CommandDispatcher` treats DNS like HTTP (request/response), but a failed
  exchange retries on the next poll instead of exiting the agent.

### Client

The listener dialog gains a `Dns` payload page with `Domain`, `Bind Host`,
`Bind Port`, `Record Type` (TXT), `Encoding` (base64url) and `Frequency`.
On edit, domain/record type/encoding/frequency are editable; bind address and
port are read-only because they are fixed at start time. The listener package
keys are exactly:

```
Name, Protocol="Dns", Domain, BindHost, BindPort, RecordType, Encoding, Frequency
```

`BindPort` and `Frequency` travel as decimal strings, matching every other
listener package.

`Protocol` is case-sensitive and must match the client's
`HavocSpace::Listener::Payload*` constants exactly: `Http`, `Https`, `Smb`,
`External`, `Dns`. The teamserver keeps these strings in
`handlers.AGENT_HTTP`/`AGENT_HTTPS`/`AGENT_PIVOT_SMB`/`AGENT_EXTERNAL`/
`AGENT_DNS`. A mismatch makes the client fall through to the service-listener
branch, drop the listener row and report "Listener protocol type not found".

## Profile impact

New optional `Dns` block under `Listeners` (`ListenerDNS` in
`teamserver/pkg/profile/config.go`). Existing profiles are unaffected.

## Verification

- `make ts-build` and `make client-build` pass.
- `go test ./pkg/handlers` covers name parsing, chunk reassembly keys and TXT
  splitting.
- `.opencode/scripts/dns-c2-smoke.sh` starts the teamserver with a DNS
  listener on a non-privileged port, checks that the UDP+TCP port is bound,
  that an incomplete chunk sequence produces an empty NOERROR answer, and that
  the port is released after shutdown.
- Agent: compile with `TRANSPORT_DNS`, check in, run `shell whoami`, run a
  small `download`, remove the listener.

## Open questions

- No DNS answer authentication: a spoofed answer from a resolver would be
  accepted if the token matches. The transport relies on the existing AES
  layer for payload integrity; the token is only a reassembly key.
- `KillDate`/`WorkingHours` are carried to the agent but not enforced by the
  teamserver handler, same as HTTP.
- The v1 profile syntax for `BindHost`/`BindPort` is new in this change; the
  docs site has not been updated.
- `SendAllPackagesToNewClient` sends `Protocol="Https"` for a secure HTTP
  listener, but the client's `Listener.Add` handler only matches
  `Protocol="Http"` and derives the HTTPS display name from the `Secure`
  field instead. A secure HTTP listener therefore still hits the "Listener
  protocol type not found" path when a client connects, and the replay
  package carries no `Secure` key. Not verified against the GUI; the DNS and
  SMB names are unaffected.
