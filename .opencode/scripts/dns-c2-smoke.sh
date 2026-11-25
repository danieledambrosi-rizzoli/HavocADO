#!/usr/bin/env bash
#
# dns-c2-smoke.sh
#
# Purpose:
#   Start an isolated teamserver with a DNS listener and verify the listener
#   binds UDP+TCP, acknowledges an upload with the AdaptixC2 compatible PUT
#   acknowledgement and releases the port on shutdown.
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
#   5  required dependency (dig, python3) is missing
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

# The RC4 key of the listener. The profile below uses the same key so that
# the script can build the queries of the agent.
KEY="000102030405060708090a0b0c0d0e0f"

# The sequence number is XORed with the mask before it is written into the
# QNAME. The teamserver unmasks it again.
SEQ_MASKED="5a5a5a5b"

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

for dependency in dig python3; do
    if ! command -v "$dependency" >/dev/null 2>&1; then
        echo "[!] $dependency not found" >&2
        exit 5
    fi
done

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
        Frequency    = 5
        KillDate     = ""
        WorkingHours = ""
        EncryptKey   = "$KEY"
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

# payload_label builds the DATA_LABELS of a query. The payload is framed as
# [total][offset][chunk], encrypted with RC4 and encoded with base32.
payload_label() {
    python3 - "$KEY" "$1" <<'PY'
import base64
import sys

key = bytes.fromhex(sys.argv[1])
payload = sys.argv[2].encode()

frame = len(payload).to_bytes(4, "big") + (0).to_bytes(4, "big") + payload

state = list(range(256))
j = 0
for i in range(256):
    j = (j + state[i] + key[i % len(key)]) & 0xFF
    state[i], state[j] = state[j], state[i]

i = j = 0
encrypted = bytearray()
for byte in frame:
    i = (i + 1) & 0xFF
    j = (j + state[i]) & 0xFF
    state[i], state[j] = state[j], state[i]
    encrypted.append(byte ^ state[(state[i] + state[j]) & 0xFF])

sys.stdout.write(base64.b32encode(bytes(encrypted)).decode().rstrip("="))
PY
}

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

# An upload of a single fragment is acknowledged with an A record. The first
# byte carries the flags (0x01 complete) and the last three bytes carry the
# next expected offset.
PAYLOAD="hello world"
LABEL="$(payload_label "$PAYLOAD")"
ANSWER="$(dig @"$HOST" -p "$PORT" A "deadbeef.cdn.$SEQ_MASKED.0.$LABEL.$DOMAIN" +short +time=2 +tries=1 2>/dev/null)"
EXPECTED="1.0.0.$(printf '%d' "${#PAYLOAD}")"

if [ "$ANSWER" != "$EXPECTED" ]; then
    echo "[!] PUT acknowledgement is '$ANSWER', expected '$EXPECTED'"
    exit 3
fi
echo "[+] PUT acknowledgement carries the complete flag and the next offset"

# A query with an unknown operation is answered with a plain TXT record.
ANSWER="$(dig @"$HOST" -p "$PORT" TXT "deadbeef.unknown.$SEQ_MASKED.0.$LABEL.$DOMAIN" +short +time=2 +tries=1 2>/dev/null | tr -d '"')"
if [ "$ANSWER" != "OK" ]; then
    echo "[!] unknown operation answered with '$ANSWER', expected 'OK'"
    exit 3
fi
echo "[+] unknown operation answered with OK"

# The same handler must answer over TCP as well.
if ! dig +tcp @"$HOST" -p "$PORT" A "deadbeef.cdn.$SEQ_MASKED.0.$LABEL.$DOMAIN" +time=2 +tries=1 >/dev/null 2>&1; then
    echo "[!] TCP query failed"
    exit 3
fi
echo "[+] TCP query answered"

if ! kill -0 "$PID" 2>/dev/null; then
    echo "[!] teamserver died while handling queries"
    sed 's/^/    /' "$LOG"
    exit 3
fi

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
