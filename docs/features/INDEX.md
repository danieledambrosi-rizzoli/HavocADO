# Feature Index

Read this table before searching the code for feature context. Each feature
has exactly one front-door document in this folder.

| feature | status | document | summary |
|---|---|---|---|
| `dns-c2` | active | [dns-c2.md](dns-c2.md) | DNS transport for Demon with AdaptixC2 parity: base32/RC4 QNAME layout, PUT acknowledgement, framed downloads, DoH fallback and burst mode; teamserver listener, profile block, client dialog |
| `listener-replay` | active | [listener-replay.md](listener-replay.md) | Replay of running listeners to a newly connected client: `EventsList` is the only source, one full package per listener |
| `print-instrumentation` | active | [print-instrumentation.md](print-instrumentation.md) | Teamserver prints AES key/IV per crypto call and hex-dumps outbound payload packages |
| `havoc-mcp-server` | active | [havoc-mcp-server.md](havoc-mcp-server.md) | MCP stdio server exposing Havoc teamserver as MCP tools (sessions, listeners, tasking) |
