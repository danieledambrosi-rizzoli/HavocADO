---
feature: listener-replay
status: active
owner: Havoc Team
verified_against: HEAD 2026-11-24
---

## Intent

A client that connects to a teamserver must see the listeners that are already
running, with the configuration those listeners actually run with, and it must
see each of them exactly once.

## Scope (teamserver / client / payload / cross-boundary)

Scope: **teamserver only**

- Teamserver: `SendAllPackagesToNewClient`
  (`teamserver/cmd/server/teamserver.go`), the listener handlers
  (`teamserver/pkg/handlers/*.go`), the listener events
  (`teamserver/pkg/events/listeners.go`), the listener dispatch
  (`teamserver/cmd/server/dispatch.go`) and the headless operator client
  (`teamserver/pkg/operator/client.go`).
- No client changes.
- No payload changes.
- No profile schema changes.

## Contract

### One source: `EventsList`

`EventsList` is the only replay source. When a listener starts, its handler
appends one `Listener.Add` package to `EventsList` and broadcasts it:

```go
pk := h.Teamserver.ListenerAdd("", LISTENER_HTTP, h)
h.Teamserver.EventAppend(pk)
h.Teamserver.EventBroadcast("", pk)
```

`SendAllPackagesToNewClient` replays `EventsList` to a newly authenticated
client and then sends the active agents. It does not walk `t.Listeners` and it
does not synthesize a second listener package, so a running listener arrives at
a new client exactly once.

`Listener.Remove` prunes the matching entry from `EventsList`, so a removed
listener is not replayed.

### Listener package shape

The replayed package is the full-configuration package built by
`events.Listener.ListenerAdd`. For HTTP it carries `Hosts`, `HostBind`,
`PortBind`, `PortConn`, `Uris`, `Headers`, `Secure`, `Status`, `UserAgent`,
the proxy fields and the cert fields; for DNS it carries `Domain`, `BindHost`,
`BindPort`, `RecordType`, `Encoding`, `Frequency`, `KillDate` and
`WorkingHours`. There is no shortened second copy of a listener.

`Protocol` is taken from the shared constants in
`teamserver/pkg/handlers/handlers.go` (`AGENT_HTTP`, `AGENT_HTTPS`,
`AGENT_PIVOT_SMB`, `AGENT_EXTERNAL`, `AGENT_DNS`) and is case-sensitive: the
C++ client compares it against `HavocSpace::Listener::Payload*` (`Http`,
`Https`, `Smb`, `External`, `Dns`). A mismatch makes the client fall through
to the service-listener branch, drop the listener row and report "Listener
protocol type not found".

A secure HTTP listener is replayed with `Protocol="Http"` and `Secure="true"`;
the client derives the HTTPS row from the `Secure` flag.

### Service listeners

Service listeners keep the shortened package shape (`Name`, `Protocol`, `Host`,
`Port`, `Error`, `Status`, `Info`). They are produced by
`Teamserver.ListenerStartNotify`, which the service handler calls; they are
appended to `EventsList` and replayed like any other listener.

### Headless operator client

`pkg/operator`'s `handleListener` accepts both shapes. `HostBind`/`PortBind`
are preferred, `BindHost`/`BindPort` (DNS) are accepted next, and `Host`/`Port`
(service listeners) last, so a DNS listener keeps its bind address and port
after the shortened copy of it is gone.

## Profile impact

None.

## Verification

- `./.opencode/scripts/build-project.sh --target teamserver`,
  `./.opencode/scripts/build-project.sh --target client` and
  `./test/run_parallel_tests.sh` pass (19/19 packages).
- Raw websocket capture on connect: a teamserver started with one HTTP and
  one DNS listener delivers exactly two `Listener.Add` packages, one per
  listener, each with the full configuration.
- MCP `list_listeners` reports `hosts` and `port_conn` for HTTP listeners and
  `host_bind`/`port_bind` for DNS listeners.

## Open questions

- `EventsList` grows for the lifetime of the teamserver and is never capped;
  it is pruned only by `Listener.Remove`.
- Service listeners carry the shortened shape, so they have no `Hosts`,
  `PortConn` or `Secure` field. The operator client tolerates the difference,
  but the two shapes are not symmetric.
