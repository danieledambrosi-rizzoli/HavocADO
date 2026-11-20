# Script Index

Reusable scripts live here so the same intent always produces the same
command. Read this table before writing a new script; extend an existing
script instead of forking a near-duplicate.

| name | purpose | language | when to use |
|---|---|---|---|
| `dns-c2-smoke.sh` | Start an isolated teamserver with a DNS listener and verify bind, query handling, TCP fallback and port release | Bash | After changing `teamserver/pkg/handlers/dns.go`, the DNS profile block, or the DNS listener wiring |
| `docs/build-project.sh` | Build the teamserver and/or client with a memory-safe job count and a sudo-free fallback | Bash | When compiling the project or reproducing a build failure |

`docs/build-project.sh` lives in `docs/` because the task that introduced it
explicitly requested that location; it is still indexed here so the same
intent resolves to one script.
