# AGENTS.md — Havoc Framework

This file is the single source of truth for how agents must work in this
repository. It is intentionally short. Load additional rules only when the
task requires them (see `.opencode/rules/`).

## Sources of truth

- Repository: https://github.com/HavocFramework/Havoc
- Official docs: https://havocframework.com/docs
- Wiki: https://github.com/HavocFramework/Havoc/wiki
- Contributing: `CONTRIBUTING.MD` in this repository

**The local repository is the single source of truth.** When code and the
official docs disagree, the code wins: it is what executes. Treat the docs
site as intent and syntax guidance, authoritative only where the code does
not contradict it. The teamserver evolves faster than the docs site;
divergence is expected, not exceptional.

When you find a divergence:

- Follow the code.
- Record both positions in the relevant feature document's Open questions,
  and state that the divergence is resolved in favor of the code.
- Do not silently "correct" one to match the other.
- Do not copy a doc claim into code, or a code behavior into docs, without
  an explicit task that asks for that synchronization. If the task is to
  update the docs, say so and do only that.

---

## 1. What Havoc is

Havoc is a modern, malleable post-exploitation command-and-control (C2)
framework. It is explicitly *not* designed to be evasive; it is designed to
be malleable and modular so operators can adapt it to their engagement.
Do not propose detection-evasion changes unless the task explicitly asks for
them. (README, official docs)

### Architecture at a glance

```
Operator  <──>  Teamserver  <──>  Demon (agent)
   Client         (Go)              (C / ASM)
```

- **Client** — cross-platform operator UI.
- **Teamserver** — server-side brain: listeners, payload generation,
  operator authentication, profiles, agent handling.
- **Demon** — Havoc’s flagship agent running on the target.

The client and teamserver are separate binaries; the teamserver is the
component that binds ports and accepts operator connections.

---

## 2. Modules, languages, and responsibilities

Every entry below is taken from the official README and official docs.

| Path | Language | Responsibility |
|---|---|---|
| `client/` | **C++ / Qt** | Cross-platform operator UI. Modern dark theme based on Dracula. Talks to the teamserver. |
| `teamserver/` | **Go (Golang)** | Multiplayer teamserver. Handles listeners (HTTP/HTTPS, SMB), payload generation (`exe`, `shellcode`, `dll`), customizable C2 profiles, External C2, operator sessions, and agent handling. |
| `payloads/Demon/` | **C and ASM** | Havoc’s flagship agent. Includes sleep obfuscation (Ekko, Ziliean, FOLIAGE), x64 return-address spoofing, indirect syscalls, SMB support, token vault, AMSI/ETW patching via hardware breakpoints, proxy library loading, and stack duplication during sleep. |
| `payloads/Shellcode/` | Build artifacts / loader material | Shellcode output and related loader assets. Do not edit unless the task explicitly targets shellcode packaging. |
| `payloads/DllLdr/` | Build artifacts / loader material | DLL loader assets. Do not edit unless the task explicitly targets DLL loading. |
| `profiles/` | **Yaotl** (HCL fork) | Example teamserver profiles (`havoc.yaotl`, `http_smb.yaotl`, `webhook_example.yaotl`). A Yaotl profile contains everything the teamserver needs to run: `Teamserver` block, `Operators` block, and `Listeners` block (with `Http` and `Smb` sub-blocks). |
| `data/` | Runtime data | Default `havoc.yaotl` and files created during interaction with the teamserver. Treat as runtime state, not source. |
| `assets/` | Mixed | Screenshots, icons, and Groovy packaging files. Usually out of scope for feature work. |
| Root `makefile` | Make | Build entrypoint. |

### Important language boundary

A feature is rarely one-sided. The client is C++/Qt and the teamserver is Go.
If a change alters a packet, command, or listener contract, it almost always
requires a matching change on both sides, plus a profile change if the behavior
is configurable. Do not declare a cross-boundary feature complete after
changing only one side.

---

## 3. Official build and run commands

These are the commands documented by the Havoc project. Do not substitute
your own build commands.

```bash
# Teamserver
make ts-build
./havoc server --profile ./profiles/havoc.yaotl -v --debug

# Client
make client-build
./havoc client
```

Documented prerequisites (from the official Installation page):

- Go (the docs state Go 1.18 is required to compile/run the teamserver;
  prefer a current Go toolchain that still satisfies the module).
- Python 3.10.x.
- A modern Qt 5 installation.
- `mingw-w64` and `nasm` for cross-compiling the C/ASM agent.
- Typical Debian/Kali build dependencies include `build-essential`, `cmake`,
  `qtbase5-dev`, `qt5-qmake`, `libqt5websockets5-dev`, `golang-go`,
  `python3-dev`, `libboost-all-dev`, `mingw-w64`, `nasm`.

The teamserver stores files created during interaction in `data/`.

---

## 4. Repository layout (top level)

```
assets/        screenshots, icons, packaging assets
client/        C++/Qt operator client
data/          runtime data and default profile
payloads/      Demon (C/ASM) and shellcode/DLL loader artifacts
profiles/      Yaotl example profiles
teamserver/    Go teamserver (entrypoint: main.go; packages under pkg/)
makefile       official build targets (ts-build, client-build)
```

Inside `teamserver/`, `main.go` is the entrypoint and `pkg/` contains the
teamserver packages. Use Serena to discover package boundaries; do not assume
package names from memory.

---

## 5. Hard rules (project conventions)

These rules are mandatory for every agent session. They exist to keep changes
reviewable, reversible, and grounded in the actual code.

### 5.1 Git workflow

- **One feature per branch.** Create a new branch from the current `main`
  before making changes.- **Commit only the feature.** Do not bundle unrelated edits.
- **No destructive git operations without explicit human approval.** This
  includes `git reset --hard`, `git clean -fd`, `git checkout -- <path>`,
  `git push --force`, and branch deletion. If a destructive operation seems
  necessary, stop and ask. State the exact command you want to run and why.
- **The diff is the review artifact.** Your job is not done until a human can
  review `git diff main...<branch>` and understand the change without asking
  follow-up questions.

### 5.2 Codebase Exploration & Editing - `serena`

- **Never** run `tree .`, recursive `grep`, or read whole files to orient
  yourself.
- **Trigger:** Navigating symbols, tracing callers/references, inspecting file structure, or editing functions/classes.
- **Action:**
  - Start with `get_symbols_overview` on target files instead of reading entire large files.
  - Use `find_symbol` and `find_referencing_symbols` to trace definitions and usages across the repository instead of relying solely on text `grep`.
  - Prefer `replace_symbol_body`, `insert_after_symbol`, and `rename_symbol` for surgical, reference-aware edits.
  - Persist important architectural discoveries, build quirks, or conventions using `write_memory`, and check `list_memories` / `read_memory` when starting work in an unfamiliar subsystem.
  - When calling execute with "serena_repl", never wrap code in backtick template literals if the payload contains backticks; use standard double-quoted strings with JSON.stringify escaping, or use the built-in edit tool directly for Markdown (.md) files.

### 5.3 Library documentation - `context7`

- **Trigger:** Writing, updating, or debugging code that uses third-party libraries, frameworks, or SDKs.
- **Action:**
  - Do not rely on memory for library signatures, CLI flags, or configuration schemas.
  - First call `resolve-library-id` with the package name, then call `get-library-docs` (or `query-docs`) to verify exact usage against current documentation before writing code.
- Do **not** use Context7 for Havoc’s own packages; Serena is the source of
  truth for project code.
- Do **not** guess method names, ownership semantics, or API signatures.

### 5.4 Reasoning & Planning - `sequential-thinking`

- **Trigger:** Any multi-file refactor, architectural change, unfamiliar bug root-cause analysis, or task with unclear scope.
- **Action:** Call `sequentialthinking` first to decompose the task into concrete steps, identify edge cases, and verify your plan before editing files or running shell commands. Revise steps explicitly if new findings contradict your initial hypothesis.

### 5.5 Feature documentation

Every feature has **one canonical front-door document** under
`docs/features/`. A folder is allowed only when the feature genuinely needs
supporting artifacts (protocol spec, threat model, test plan). Do not create
empty placeholder folders for hypothetical features.

`docs/features/INDEX.md` is the navigation index. It lists every feature with
status and a direct link to its document. Read this index first when looking
for feature context.

Each feature document uses a fixed, greppable schema:

```markdown
---
feature: <slug>
status: proposed | active | shipped
owner: <name or handle>
verified_against: <commit sha> <date>   # required only when status: shipped
---

## Intent
## Scope (teamserver / client / payload / cross-boundary)
## Contract
## Profile impact
## Verification
## Open questions
```

- `proposed` documents may be ten lines. Do not inflate them.
- `active` documents add the symbol map and the plan.
- `shipped` documents add `verified_against` so staleness is detectable.

Write documents as a human-readable database: short headings, short lines,
consistent field names. The goal is that a human and an agent can both grep
them.

### 5.6 Filemap — same-commit freshness

`docs/FILEMAP.md` is a maintained, per-file map. It exists so an agent can
locate the right file without filesystem exploration.

Each entry is one line (or one small row) with these fields:

```
path | language | responsibility | contract (what callers depend on) | read-with (files that must be read together) | last-verified-against
```

- Every file you create or modify must have an updated entry in the same
  commit. The map is part of the change, not a separate maintenance task.
- The **contract** field is the most important field. It records what must not
  break: packet symmetry, Qt ownership rules, calling conventions, profile
  invariants.
- The **read-with** field prevents partial-context edits: if a file is
  meaningless without another file, say so.
- Keep the map bounded. One line per file. If an entry needs a paragraph, the
  paragraph belongs in a feature document, not in the filemap.

### 5.7 Cross-boundary completeness

A cross-boundary feature is complete only when:

1. The teamserver side compiles and the relevant package builds.
2. The client side compiles.
3. The contract between them is symmetric: every field written is read, every
   error case is handled on both sides.
4. The profile documentation is updated if a new key was introduced.
5. The feature document’s `Contract` section reflects the final behavior.

### 5.8 Build gate

Do not declare work complete until the appropriate build gate passes:

```bash
# Teamserver changes
make ts-build

# Client changes
make client-build

# Both sides changed
make ts-build && make client-build
```

If the build gate fails, the task is not complete. Do not suppress errors,
comment out failing code, or weaken types to make the gate pass.

---

### 5.9 Script reuse rule

Scripts are code. They live under `.opencode/scripts/`.
The rule exists for one reason: **the same intent must produce the same
command every time.** Regenerating a known script is both a token waste and
a consistency risk, because two sessions that "do the same thing" will
differ in flags, order, or error handling.

**Reuse before writing.**
- Before writing any script, read `.opencode/scripts/INDEX.md`. If an
  existing script does the job, use it.
- If an existing script does almost the job, extend it with a new flag or
  subcommand. Do not fork it into a near-duplicate.
- Write a new script only when no existing one fits, and state why in the
  commit message.
- Do not save one-off commands to the repository. A command that runs once
  in a shell is not a script; a command that will run again in a future
  session is.

**Every script carries a header contract.** The first lines of the file
must state, in this order:
- purpose (one line),
- arguments (name, type, meaning, default),
- outputs (files written, stdout format),
- exit codes (what each nonzero value means),
- side effects (network, filesystem, git, subprocesses).

A script without a header contract is not ready to be committed.

**Every script has an index entry.** `.opencode/scripts/INDEX.md` is a
greppable table: `name | purpose | language | when to use`. Update it in
the same commit as the script, exactly as `docs/FILEMAP.md` is updated.

**Every script has a filemap entry.** Follow section 5.6. The `contract`
field states what the script guarantees to its callers — the invariant a
future session can rely on without reading the script body.

**Scripts are reviewed like code.** No script is exempt from the build
gate, the diff review, or the branch rule. A script that bypasses review
is worse than no script, because future sessions will trust it without
checking.

## 6. Definition of done

A task is done when all of the following are true:

- [ ] The change is on its own branch and committed.
- [ ] `make ts-build` and/or `make client-build` passes, as applicable.
- [ ] Serena diagnostics are clean for every edited file.
- [ ] The feature document exists and matches the implemented behavior.
- [ ] `docs/features/INDEX.md` is updated.
- [ ] Every touched file has a current `docs/FILEMAP.md` entry.
- [ ] The diff is understandable without reading the agent’s transcript.

---

## 7. Official references

- Installation: https://havocframework.com/docs/installation
- Profiles (Yaotl): https://havocframework.com/docs/profiles
- Agent (Demon): https://havocframework.com/docs/agent
- Object files / BOFs: https://havocframework.com/docs/object-files
- Wiki: https://github.com/HavocFramework/Havoc/wiki
- Contributing: `CONTRIBUTING.MD`
