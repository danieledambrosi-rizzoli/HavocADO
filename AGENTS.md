# AGENTS.md — Havoc Framework

Single source of truth for agent behavior in this repo. Load extra rules from
`.opencode/rules/` only when the task requires them.

## Project
Havoc is a malleable post-exploitation C2 framework: a C++/Qt client, a Go
teamserver, and the C/ASM Demon agent. It is not designed to be evasive; do not
propose detection-evasion changes unless explicitly asked.

## Quick start
# Teamserver
make ts-build
./havoc server --profile ./profiles/havoc.yaotl -v --debug

# Client
make client-build
./havoc client

Prereqs: Go, Python 3.10.x, Qt5, mingw-w64, nasm. Do not substitute your own
build commands.

## Layout
client/         C++/Qt operator UI
teamserver/     Go teamserver (main.go, pkg/)
payloads/       Demon (C/ASM) and shellcode/DLL loader artifacts
profiles/       Yaotl (HCL) profiles
docs/features/  feature docs (INDEX.md)
docs/FILEMAP.md per-file map

## Sources of truth
Code wins over docs. When they diverge, follow the code and record both
positions in the relevant feature doc's Open questions, resolved in favor of the
code. Do not silently sync either direction without an explicit task.
Docs: https://havocframework.com/docs | Wiki | CONTRIBUTING.MD

## Cross-boundary rule
Client and teamserver are separate binaries. Any change to a packet, command, or
listener contract requires matching changes on both sides, plus a profile update
if configurable. Do not call it done after changing one side.

## Tooling
Use dedicated tools, not shell equivalents. Detailed rules live in
`.opencode/rules/`; this file does not duplicate them.
- **Code navigation/edits:** Serena — start with `get_symbols_overview`; never
  `tree`, recursive grep, or whole-file reads to orient. Use symbol tools.
- **Library docs:** Context7 — `resolve-library-id` then `get-library-docs`.
- **Planning:** sequentialthinking for multi-file refactors or unclear scope.
- **Web:** mcp-server-fetch; request Markdown.
- **Scripts:** reuse `.opencode/scripts/` (see INDEX.md) before writing new
  ones. Every script needs a header contract, an INDEX.md entry, and a FILEMAP
  entry.

## Boundaries
✅ **Always:** write a feature doc; update FILEMAP in the same commit; run the
build gate before declaring done.
⚠️ **Ask first:** destructive git (`reset --hard`, `clean -fd`, `push --force`,
branch deletion), profile schema changes, cross-boundary contract changes.
❌ **Never:** commit secrets; push to main; bundle unrelated edits; suppress
build errors or weaken types to pass the gate.

## Feature docs
One front-door doc per feature under `docs/features/`, indexed in `INDEX.md`.
Frontmatter: `feature`, `status` (proposed|active|shipped), `owner`,
`verified_against` (required when shipped). Body: `## Intent`, `## Scope`,
`## Contract`, `## Profile impact`, `## Verification`, `## Open questions`.
Proposed docs may be ten lines; do not inflate them.

## FILEMAP
`docs/FILEMAP.md` is a per-file map. Every file you create or modify gets an
entry in the same commit:
`path | language | responsibility | contract | read-with | last-verified-against`
The contract field records what must not break.

## Build gate
make ts-build                         # teamserver changes
make client-build                     # client changes
make ts-build && make client-build    # both
If the gate fails, the task is not complete. Do not comment out failing code.

## Definition of done
- [ ] Change on its own branch, committed.
- [ ] Build gate passes.
- [ ] Serena diagnostics clean for every edited file.
- [ ] Feature doc matches behavior; `docs/features/INDEX.md` updated.
- [ ] Every touched file has a current `docs/FILEMAP.md` entry.
- [ ] Diff is understandable without the agent's transcript.

## References
- Installation: https://havocframework.com/docs/installation
- Profiles (Yaotl): https://havocframework.com/docs/profiles
- Agent (Demon): https://havocframework.com/docs/agent
- Object files / BOFs: https://havocframework.com/docs/object-files
- Wiki: https://github.com/HavocFramework/Havoc/wiki
- Contributing: CONTRIBUTING.MD
