# Script Index

Reusable scripts live here so the same intent always produces the same
command. Read this table before writing a new script; extend an existing
script instead of forking a near-duplicate.

| name | purpose | language | when to use |
|---|---|---|---|
| `dns-c2-smoke.sh` | Start an isolated teamserver with a DNS listener and verify the bind of UDP+TCP, the PUT acknowledgement, the fallback answer of an unknown operation, TCP and the release of the port | Bash | After changing `teamserver/pkg/handlers/dns.go`, the DNS profile block, or the DNS listener wiring |
| `build-project.sh` | Build the teamserver and/or client with a memory-safe job count and a sudo-free fallback | Bash | When compiling the project or reproducing a build failure |
| `generate_payloads.py` | Connect to a running teamserver via MCP and generate Demon shellcode payloads for multiple listeners | Python | When you need to programmatically generate identical payloads across different listeners for comparison/testing |
