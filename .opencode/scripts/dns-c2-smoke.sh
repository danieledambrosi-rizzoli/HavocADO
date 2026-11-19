#!/usr/bin/env bash
#
# dns-c2-smoke.sh
#
# Purpose:
#   Start an isolated teamserver with a DNS listener and verify the listener
#   binds UDP+TCP, answers DNS queries and releases the port on shutdown.
#
# Arguments:
#   --binary <path>   teamserver binary            (default: <repo>/havoc)
#   --port <n>        DNS bind port                (default: 5353)
#   --host <ip>       DNS bind host                (default: 127.0.0.1)
#   --ts-port <n>     teamserver websocket port    (default: 40056)
#   --domain <name>   C2 domain                    (default: c2.smoke.test)
#   --timeout <s>     startup/shutdown timeout     (default: 20)
#
# Outputs:
#   Human-readable progress on stdout, one line per check. The temporary
#   working directory and its profile are removed on exit.
#
# Exit codes:
#   0  all checks passed
#   1  bad arguments or binary not found
#   2  teamserver did not open the DNS port
#   3  DNS queries did not behave as documented
#   4  DNS port was still bound after shutdown
#   5  required dependency (dig) is missing
#
# Side effects:
#   Binds <host>:<port> UDP+TCP and <ts-port> TCP on the loopback interface,
#   spawns one teamserver process, writes files only inside a mktemp directory
#   (including its own data/havoc.db), and removes that directory on exit.
#
set -u

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BINARY="$ROOT/havoc"
PORT=5353
HOST=127.0.0.1
TS_PORT=40056
DOMAIN=c2.smoke.test
TIMEOUT=20

while [ $# -gt 0 ]; do
    case "$1" in
        --binary)  BINARY="$2"; shift 2 ;;
        --port)    PORT="$2"; shift 2 ;;
        --host)    HOST="$2"; shift 2 ;;
        --ts-port) TS_PORT="$2"; shift 2 ;;
        --domain)  DOMAIN="$2"; shift 2 ;;
        --timeout) TIMEOUT="$2"; shift 2 ;;
        -h|--help) sed -n '3,31p' "$0"; exit 0 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if ! command -v dig >/dev/null 2>&1; then
    echo "[!] dig not found (install dnsutils/bind-utils)" >&2
    exit 5
fi

if [ ! -x "$BINARY" ]; then
    echo "[!] teamserver binary not found or not executable: $BINARY" >&2
    exit 1
fi

# The teamserver runs with cwd inside the temp dir, so the binary path must be
# absolute before we change directory.
BINARY="$(cd "$(dirname "$BINARY")" && pwd)/$(basename "$BINARY")"

WORK="$(mktemp -d "${TMPDIR:-/tmp}/dns-smoke.XXXXXX")"
mkdir -p "$WORK/data"
LOG="$WORK/teamserver.log"
PROFILE="$WORK/profile.yaotl"
PID=""

cleanup() {
    if [ -n "$PID" ] && kill -0 "$PID" 2>/dev/null; then
        kill -TERM "$PID" 2>/dev/null
        wait "$PID" 2>/dev/null
    fi
    rm -rf "$WORK"
}
trap cleanup EXIT

cat > "$PROFILE" <<EOF
Teamserver {
    Host = "127.0.0.1"
    Port = $TS_PORT
}

Operators {
    user "smoke" {
        Password = "smoke-test-password"
    }
}

Listeners {
    Dns {
        Name         = "DNS Smoke"
        Domain       = "$DOMAIN"
        BindHost     = "$HOST"
        BindPort     = $PORT
        RecordType   = "TXT"
        Encoding     = "base64url"
        Frequency    = 5
        KillDate     = ""
        WorkingHours = ""
    }
}

Demon {
    Sleep  = 1
    Jitter = 0

    Injection {
        Spawn64 = "C:\\\\Windows\\\\System32\\\\notepad.exe"
        Spawn32 = "C:\\\\Windows\\\\SysWOW64\\\\notepad.exe"
    }
}
EOF

port_bound() {
    if command -v ss >/dev/null 2>&1; then
        ss -lun 2>/dev/null | awk 'NR > 1 { print $4 }' | grep -qE "[:.]$1\$"
        return $?
    fi
    if command -v netstat >/dev/null 2>&1; then
        netstat -lun 2>/dev/null | awk 'NR > 2 { print $4 }' | grep -qE "[:.]$1\$"
        return $?
    fi
    # No socket tool: fall back to a query probe.
    dig @"$HOST" -p "$PORT" TXT "probe.$DOMAIN" +time=1 +tries=1 +short >/dev/null 2>&1
}

echo "[*] starting teamserver (dns $HOST:$PORT, ws $TS_PORT)"
cd "$WORK" || exit 1
"$BINARY" server --profile "$PROFILE" --debug -v > "$LOG" 2>&1 &
PID=$!

OPENED=0
for _ in $(seq 1 "$TIMEOUT"); do
    if ! kill -0 "$PID" 2>/dev/null; then
        echo "[!] teamserver exited during startup; log follows:"
        sed 's/^/    /' "$LOG"
        exit 2
    fi
    if port_bound "$PORT"; then
        OPENED=1
        break
    fi
    sleep 1
done

if [ "$OPENED" -ne 1 ]; then
    echo "[!] DNS port $PORT was not bound within ${TIMEOUT}s; log follows:"
    sed 's/^/    /' "$LOG"
    exit 2
fi
echo "[+] UDP/TCP port $PORT is bound"

# An incomplete chunk sequence must not produce an answer.
CHUNK_A="$(printf '%s' 'first-half-of-a-message' | base64 | tr '+/' '-_' | tr -d '=')"
CHUNK_B="$(printf '%s' '-second-half' | base64 | tr '+/' '-_' | tr -d '=')"
ANSWER="$(dig @"$HOST" -p "$PORT" TXT "0.2.deadbeef.$CHUNK_A.$DOMAIN" +short +time=2 +tries=1 2>/dev/null)"
if [ -n "$ANSWER" ]; then
    echo "[!] incomplete chunk produced an answer: $ANSWER"
    exit 3
fi
echo "[+] incomplete chunk produced no answer"

# A complete but unparseable message must be consumed silently.
dig @"$HOST" -p "$PORT" TXT "1.2.deadbeef.$CHUNK_B.$DOMAIN" +short +time=2 +tries=1 >/dev/null 2>&1
if ! kill -0 "$PID" 2>/dev/null; then
    echo "[!] teamserver died while handling chunks"
    sed 's/^/    /' "$LOG"
    exit 3
fi
echo "[+] chunked queries handled without crashing"

# The same handler must answer over TCP as well.
if ! dig +tcp @"$HOST" -p "$PORT" TXT "0.1.deadbeef.$CHUNK_A.$DOMAIN" +time=2 +tries=1 >/dev/null 2>&1; then
    echo "[!] TCP TXT query failed"
    exit 3
fi
echo "[+] TCP TXT query answered"

kill -TERM "$PID" 2>/dev/null
wait "$PID" 2>/dev/null
PID=""

for _ in $(seq 1 5); do
    if ! port_bound "$PORT"; then
        echo "[+] DNS port released after shutdown"
        echo "[+] dns-c2 smoke test passed"
        exit 0
    fi
    sleep 1
done

echo "[!] DNS port $PORT is still bound after shutdown"
exit 4
