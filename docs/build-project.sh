#!/usr/bin/env bash
#
# build-project.sh — compile the Havoc teamserver and/or client.
#
# PURPOSE
#   Reproducible build entrypoint for this checkout. It wraps the repository's
#   official make targets so a fresh session does not have to rediscover the
#   required flags, toolchain paths, or the low-memory workaround.
#
# ARGUMENTS
#   --target <all|teamserver|client>  What to build. Default: all.
#   --jobs <N>                        Parallel compile jobs. Default: auto
#                                     (1 when MemTotal < 4 GiB, otherwise 4).
#   --no-setcap                       Never run `sudo setcap` after building
#                                     the teamserver.
#   -h, --help                        Print this header and exit.
#
# OUTPUTS
#   stdout: timestamped progress lines and the final build summary.
#   ./havoc                teamserver binary (teamserver target).
#   client/Havoc           client binary (client target).
#   client/Build/...       CMake build tree.
#   No log file is written; redirect stdout yourself if you need one.
#
# EXIT CODES
#   0   requested targets built and their binaries exist.
#   1   invalid arguments.
#   2   missing build dependency (checked before compiling).
#   10  teamserver build failed.
#   20  client build failed.
#   30  build reported success but an expected binary is missing.
#
# SIDE EFFECTS
#   - Runs `go build` via the makefile; writes ./havoc.
#   - Runs CMake and the compiler; writes client/Build and client/Havoc.
#   - Runs `git submodule update --init --recursive`.
#   - If client/Modules is absent, clones it from GitHub (network access).
#   - If sudo is passwordless and --no-setcap was not given, runs
#     `sudo setcap 'cap_net_bind_service=+ep' havoc`.
#   - If the teamserver is built without setcap, low ports (<1024) cannot be
#     bound by an unprivileged user.
#
# NOTES
#   `make ts-build` runs teamserver/Install.sh (sudo apt + cross-toolchain
#   download) and then setcap. When sudo is unavailable this script falls back
#   to `make dev-ts-compile`, which builds the same binary without those steps.
#   The client steps mirror `make client-build`, except that the job count is
#   configurable: the makefile hardcodes `-j 4`, which OOM-kills cc1plus on
#   hosts with ~1.5 GiB RAM.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

TARGET="all"
JOBS=""
USE_SETCAP=1

usage() { sed -n '4,50p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; }

log()  { printf '[build] %s\n' "$*"; }
warn() { printf '[build] WARNING: %s\n' "$*" >&2; }
die()  { printf '[build] ERROR: %s\n' "$*" >&2; exit "${2:-1}"; }

while [ $# -gt 0 ]; do
    case "$1" in
        --target)
            [ $# -ge 2 ] || die "--target needs a value" 1
            TARGET="$2"; shift 2 ;;
        --jobs)
            [ $# -ge 2 ] || die "--jobs needs a value" 1
            JOBS="$2"; shift 2 ;;
        --no-setcap)
            USE_SETCAP=0; shift ;;
        -h|--help)
            usage; exit 0 ;;
        *)
            die "unknown argument: $1" 1 ;;
    esac
done

case "$TARGET" in
    all|teamserver|client) ;;
    *) die "--target must be all, teamserver or client" 1 ;;
esac

if [ -z "$JOBS" ]; then
    mem_kb="$(awk '/^MemTotal:/{print $2}' /proc/meminfo 2>/dev/null || echo 0)"
    if [ "${mem_kb:-0}" -lt 4194304 ]; then
        JOBS=1
    else
        JOBS=4
    fi
fi
case "$JOBS" in
    ''|*[!0-9]*) die "--jobs must be a positive integer" 1 ;;
esac
[ "$JOBS" -ge 1 ] || die "--jobs must be a positive integer" 1

require() {
    command -v "$1" >/dev/null 2>&1 || die "missing dependency: $1" 2
}

build_teamserver() {
    require go
    log "building teamserver (jobs=$JOBS)"

    if [ "$USE_SETCAP" -eq 1 ] && sudo -n true >/dev/null 2>&1; then
        if ! make ts-build; then
            die "make ts-build failed" 10
        fi
    else
        if [ "$USE_SETCAP" -eq 1 ]; then
            warn "passwordless sudo unavailable; building with make dev-ts-compile"
            warn "setcap NOT applied: binding ports <1024 will require root"
        fi
        if ! make dev-ts-compile; then
            die "make dev-ts-compile failed" 10
        fi
    fi

    [ -x ./havoc ] || die "teamserver build produced no ./havoc binary" 30
    log "teamserver: ./havoc ($(stat -c '%s' ./havoc) bytes)"
}

build_client() {
    require cmake
    require c++
    log "building client (jobs=$JOBS)"

    git submodule update --init --recursive || die "git submodule update failed" 20

    mkdir -p client/Build
    if ! ( cd client/Build && cmake .. ); then
        die "cmake configure failed" 20
    fi

    if [ ! -d client/Modules ]; then
        log "client/Modules missing; cloning it (network access)"
        git clone https://github.com/HavocFramework/Modules client/Modules \
            --single-branch --branch "$(git rev-parse --abbrev-ref HEAD)" \
            || die "could not clone client/Modules" 20
    fi

    if ! cmake --build client/Build -- -j "$JOBS"; then
        die "client compilation failed (retry with --jobs 1 if the OOM killer fired)" 20
    fi

    [ -x client/Havoc ] || die "client build produced no client/Havoc binary" 30
    log "client: client/Havoc ($(stat -c '%s' client/Havoc) bytes)"
}

log "repository: $ROOT_DIR"
log "commit:     $(git rev-parse --short HEAD) ($(git rev-parse --abbrev-ref HEAD))"

case "$TARGET" in
    all)       build_teamserver; build_client ;;
    teamserver) build_teamserver ;;
    client)    build_client ;;
esac

log "build completed successfully (target=$TARGET)"
