---
feature: dns-c2
status: active
owner: Havoc Team
---

## Intent

Carry Demon's transport messages over DNS so an operator can run C2 through a
resolver path that is already allowed out of most networks. The transport
follows the AdaptixC2 DNS/DoH channel: same QNAME layout, same base32
encoding, same RC4 key handling, same fragmentation and acknowledgement
logic.

## Scope (teamserver / client / payload / cross-boundary)

Cross-boundary: teamserver, client, and Demon all participate.

- Teamserver: `LISTENER_DNS` handler (`teamserver/pkg/handlers/dns.go`),
  profile block, listener events, client-package dispatch, DB restore, payload
  builder.
- Client: listener dialog page, listener package parsing.
- Demon: `TRANSPORT_DNS` transport (`payloads/Demon/src/core/TransportDns.c`).

## Contract

### Profile

```yaotl
Listeners {
    Dns {
        Name         = "DNS C2"
        Domain       = "c2.example.com"
        BindHost     = "0.0.0.0"   # optional, default 0.0.0.0
        BindPort     = 53          # optional, default 53
        Frequency    = 30          # optional, seconds, clamped 5..300
        KillDate     = ""          # optional
        WorkingHours = ""          # optional
        PktSize      = 512         # optional, bytes of one answer
        TTL          = 10          # optional, seconds
        EncryptKey   = "000102030405060708090a0b0c0d0e0f" # optional, 16 bytes
        BurstEnabled = false       # optional
        BurstSleep   = 50          # optional, milliseconds
        BurstJitter  = 0           # optional, percent
        Resolvers    = "8.8.8.8"   # optional, comma separated
        DohResolvers = ""          # optional, comma separated urls
    }
}
```

`Domain` is stored lower-case with the trailing dot removed. `Start()` returns
an error on a bind failure, so the listener is not registered and the client
receives a listener error. Both UDP and TCP are bound on
`BindHost:BindPort`.

`EncryptKey` is the RC4 key of the transport. It is either 32 hex characters
or 16 raw characters. A listener without a key gets a random key that is
stored with the listener configuration, so the teamserver and the agent always
use the same key. `RecordType` and `Encoding` are kept as compatibility fields
for existing profiles; the wire format no longer depends on them.

### QNAME

Every query follows this label order:

```
{SID}.{OP}.{SEQ}.{IDX}.{DATA_LABELS}.{DOMAIN}
```

| Segment | Value |
| :--- | :--- |
| `SID` | 8 lower-case hex characters, the agent id |
| `OP` | `www`/`hi` → HI, `cdn`/`put` → PUT, `api`/`get` → GET, `hb` → HB |
| `SEQ` | 8 hex characters, the sequence number XORed with `seqXorMask` |
| `IDX` | reserved fragment index, the agent sends `0` |
| `DATA_LABELS` | base32 (RFC 4648, no padding), split into labels of at most 63 characters |

`seqXorMask` is `0x5A5A5A5A` on both sides. The teamserver strips the
configured domain suffix before it parses the labels.

### Payload encoding

The payload of a query is RC4 encrypted with the 16 byte key of the listener
and then base32 encoded. The teamserver decrypts and decodes it the same way.

An upload (HI and PUT) carries the frame:

```
[0:4] total length (big endian uint32)
[4:8] offset (big endian uint32)
[8:] chunk
```

The teamserver reassembles the fragments per session and hands the completed
message to the normal `parseAgentRequest` path, so DNS messages carry the
exact same `DEMON_COMMAND_GET_JOB` / task payloads as HTTP.

A download (GET) carries the four byte offset of the chunk that the agent
wants. The answer is the frame `[0:4] total, [4:8] offset, [8:] chunk`,
RC4 encrypted and base64 encoded in the TXT records of the answer. When the
teamserver has no answer for the session, it asks the agent for its jobs
(the equivalent of AdaptixC2's `TsAgentGetHostedAll`).

### PUT acknowledgement

The acknowledgement of an upload is an `A` record:

```
ip[0]     flags (0x01 complete, 0x02 needs reset)
ip[1:4]   next expected offset, 24 bit big endian
```

The agent uses the next expected offset to continue the upload and restarts
the upload when the reset flag is set. UDP answers are limited to
`dnsSafeChunkSize` (220) raw bytes, TCP answers use `PktSize`.

### Agent

- `TRANSPORT_DNS` is a compile-time transport of the builder. The DNS block of
  the agent configuration is appended after the common config block, in this
  order: `Domain` (wide string), `Frequency` (int32), `KillDate` (int64),
  `WorkingHours` (int32), `EncryptKey` (16 bytes), `PktSize` (int32), `TTL`
  (int32), `BurstEnabled` (int32), `BurstSleep` (int32), `BurstJitter`
  (int32), `Resolvers` (wide string), `DohResolvers` (wide string).
- The first message of an agent is the registration package and is sent with
  the `www` operation, every later message with the `cdn` operation. The
  answer of the teamserver is fetched with the `api` operation.
- The agent builds the DNS query itself (`BuildDnsWireQuery`), parses the
  answer (`ParseDnsWireResponse`) and parses the upload acknowledgement
  (`ParsePutAckResponse`).
- The agent queries the plain resolvers of `Resolvers` first and falls back to
  the DNS over HTTPS resolvers of `DohResolvers` (`QueryDoH`, wininet). A
  resolver that fails three times is put on hold for 30 seconds.
- With `BurstEnabled` the agent ignores the regular sleep interval and polls
  the teamserver every `BurstSleep` milliseconds with `BurstJitter` percent of
  jitter.
- `CommandDispatcher` treats DNS like HTTP (request/response), but a failed
  exchange retries on the next poll instead of exiting the agent.

### Client

The listener dialog has a `Dns` payload page with `Domain`, `Bind Host`,
`Bind Port`, `Record Type`, `Encoding`, `Frequency`, `Encrypt Key`,
`Packet Size`, `TTL`, `Burst Mode`, `Burst Sleep`, `Burst Jitter`,
`Resolvers` and `DoH Resolvers`. On edit, the bind address and port are
read-only because they are fixed at start time.

The listener package carries:

```
Name, Protocol="Dns", Domain, BindHost, BindPort, RecordType, Encoding,
Frequency, EncryptKey, PktSize, TTL, BurstEnabled, BurstSleep, BurstJitter,
Resolvers, DohResolvers
```

plus the common `Status`, `KillDate` and `WorkingHours` fields. `BindPort`,
`Frequency` and the other numbers travel as decimal strings, matching every
other listener package.

The teamserver only applies the settings that the client actually sent, so a
client that does not know a setting does not reset it.

`Protocol` is case-sensitive and must match the client's
`HavocSpace::Listener::Payload*` constants exactly: `Http`, `Https`, `Smb`,
`External`, `Dns`. The teamserver keeps these strings in
`handlers.AGENT_HTTP`/`AGENT_HTTPS`/`AGENT_PIVOT_SMB`/`AGENT_EXTERNAL`/
`handlers.AGENT_DNS`. A mismatch makes the client fall through to the
service-listener branch, drop the listener row and report "Listener protocol
type not found".

The package a newly connected client receives is the one built by
`events.Listener.ListenerAdd`, so `Protocol` always carries the constant
spelling and the package always carries the full DNS configuration. See
[listener-replay.md](listener-replay.md).

## Profile impact

The `Dns` block under `Listeners` (`ListenerDNS` in
`teamserver/pkg/profile/config.go`) gained the optional fields `PktSize`,
`TTL`, `EncryptKey`, `BurstEnabled`, `BurstSleep`, `BurstJitter`,
`Resolvers` and `DohResolvers`. Existing profiles keep working; a listener
without an `EncryptKey` gets a random key when it starts.

## Verification

- `make ts-build` and `make client-build` pass.
- `go test ./pkg/handlers` covers the QNAME parsing, the operation mapping,
  the PUT acknowledgement layout, the response frame, the base32 alphabet and
  a round trip of the upload and the download.
- `.opencode/scripts/dns-c2-smoke.sh` starts the teamserver with a DNS
  listener on a non-privileged port, checks that the UDP+TCP port is bound,
  that an upload is acknowledged with the complete flag and the next expected
  offset, that an unknown operation is answered, that TCP works and that the
  port is released after shutdown.
- Agent: compile with `TRANSPORT_DNS`, check in, run `shell whoami`, run a
  small `download`, remove the listener.

## Open questions

- No DNS answer authentication: a spoofed answer from a resolver would be
  accepted. The transport relies on the existing AES layer for payload
  integrity.
- `KillDate`/`WorkingHours` are carried to the agent but not enforced by the
  teamserver handler, same as HTTP.
- The heartbeat operation (`hb`) is implemented by the teamserver but the
  agent does not use it yet.
- The docs site has not been updated for the new profile fields.
