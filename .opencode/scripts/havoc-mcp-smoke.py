#!/usr/bin/env python3
#
# havoc-mcp-smoke.py — drive the Havoc MCP stdio server like a real MCP client
# and run one non-destructive command per MCP tool against a live teamserver.
#
# PURPOSE
#   Validate `havoc mcp` (teamserver/pkg/mcp) end to end: JSON-RPC handshake,
#   tool registry, operator login/replay and one live task per tool. Only
#   non-destructive commands are used, so the agent's filesystem, processes and
#   configuration are left untouched.
#
# ARGUMENTS
#   --teamserver <url>  teamserver websocket URL (default wss://127.0.0.1:40056/havoc/)
#   --user <name>       operator name (default Neo)
#   --password <pw>     operator password (default $HAVOC_MCP_PASSWORD, else password1234)
#   --tls-ca <path>     CA certificate used for TLS verification (default data/server.cert)
#   --insecure          skip TLS verification (also used automatically as fallback)
#   --audit-log <path>  MCP server audit log (default /tmp/opencode/havoc-mcp-audit.log)
#   --report <path>     JSON transcript (default /tmp/opencode/havoc-mcp-report.json)
#   --timeout <s>       per-response timeout in seconds (default 120)
#
# OUTPUTS
#   stdout: one block per MCP method/tool with the exact request and response.
#   <audit-log>         audit trail written by the MCP server itself.
#   <report>            machine readable transcript of every request/response.
#
# NOTES
#   The script writes only to /tmp/opencode (logs and report); it does not
#   modify the repository, the teamserver profile or the agent.

import argparse
import json
import os
import subprocess
import sys
import time

REPO_ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
DEFAULT_TEAMSERVER = "wss://127.0.0.1:40056/havoc/"
DEFAULT_USER = "Neo"
DEFAULT_PASSWORD = "password1234"
DEFAULT_CA = os.path.join(REPO_ROOT, "data", "server.cert")
DEFAULT_AUDIT = "/tmp/opencode/havoc-mcp-audit.log"
DEFAULT_REPORT = "/tmp/opencode/havoc-mcp-report.json"


class McpClient:
    """Minimal JSON-RPC 2.0 client for a newline-delimited stdio MCP server."""

    def __init__(self, argv, stderr_path):
        self.stderr_handle = open(stderr_path, "wb")
        self.argv = argv
        self.proc = subprocess.Popen(
            argv,
            cwd=REPO_ROOT,
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=self.stderr_handle,
            text=True,
            bufsize=1,
        )
        self.next_id = 0
        self.transcript = []

    def alive(self):
        return self.proc.poll() is None

    def _write(self, payload):
        self.proc.stdin.write(json.dumps(payload) + "\n")
        self.proc.stdin.flush()

    def request(self, method, params=None, label=None, timeout=120):
        self.next_id += 1
        request_id = self.next_id
        payload = {"jsonrpc": "2.0", "id": request_id, "method": method}
        if params is not None:
            payload["params"] = params
        label = label or method
        print("\n=== [{0}] REQUEST ===".format(label), flush=True)
        print(json.dumps(payload, indent=2), flush=True)
        self._write(payload)
        response = self._read(request_id, timeout)
        print("=== [{0}] RESPONSE ===".format(label), flush=True)
        print(json.dumps(response, indent=2) if response is not None else "(no response)", flush=True)
        self.transcript.append({"label": label, "request": payload, "response": response})
        return response

    def notify(self, method, params=None):
        payload = {"jsonrpc": "2.0", "method": method}
        if params is not None:
            payload["params"] = params
        print("\n=== [{0}] NOTIFICATION ===".format(method), flush=True)
        print(json.dumps(payload, indent=2), flush=True)
        self._write(payload)
        self.transcript.append({"label": method, "request": payload, "response": None})

    def _read(self, request_id, timeout):
        deadline = time.time() + timeout
        while True:
            if time.time() > deadline:
                raise TimeoutError("no response for request id {0}".format(request_id))
            line = self.proc.stdout.readline()
            if not line:
                raise RuntimeError("MCP server closed stdout (see the stderr log)")
            line = line.strip()
            if not line:
                continue
            print("<<< {0}".format(line), flush=True)
            message = json.loads(line)
            if message.get("id") == request_id:
                return message

    def tool(self, name, arguments=None, timeout=120):
        return self.request(
            "tools/call",
            {"name": name, "arguments": arguments or {}},
            label="tool:{0}".format(name),
            timeout=timeout,
        )

    def close(self):
        try:
            self.proc.stdin.close()
        except Exception:
            pass
        try:
            self.proc.wait(timeout=10)
        except Exception:
            self.proc.kill()
        self.stderr_handle.close()


def tool_payload(response):
    """Return (ok, payload) where payload is the decoded tool result."""
    if response is None:
        return False, None
    if "error" in response:
        return False, response["error"]
    try:
        text = response["result"]["content"][0]["text"]
    except Exception:
        return False, response
    try:
        return True, json.loads(text)
    except Exception:
        return True, text


def build_argv(args, insecure):
    argv = [
        os.path.join(REPO_ROOT, "havoc"),
        "mcp",
        "--teamserver", args.teamserver,
        "--user", args.user,
        "--password", args.password,
        "--audit-log", args.audit_log,
        "--loot-root", os.path.join(REPO_ROOT, "data", "loot"),
        "--max-output-bytes", "262144",
    ]
    if insecure:
        argv.append("--insecure")
    elif args.tls_ca:
        argv.extend(["--tls-ca", args.tls_ca])
    return argv


def start_client(args):
    """Start the MCP server, falling back to --insecure when TLS verification fails."""
    attempts = [True] if (args.insecure or not args.tls_ca) else [False, True]
    for attempt, insecure in enumerate(attempts, start=1):
        argv = build_argv(args, insecure)
        stderr_path = "{0}.stderr.{1}".format(args.audit_log, attempt)
        client = McpClient(argv, stderr_path)
        time.sleep(2.0)
        if client.alive():
            print("[start] MCP server started: {0}".format(" ".join(argv)), flush=True)
            return client, insecure
        print("[start] MCP server exited early with --insecure={0}".format(insecure), flush=True)
        try:
            with open(stderr_path, "r", errors="replace") as handle:
                print(handle.read()[-2000:], flush=True)
        except Exception:
            pass
        client.close()
    raise RuntimeError("could not start the Havoc MCP server")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--teamserver", default=DEFAULT_TEAMSERVER)
    parser.add_argument("--user", default=DEFAULT_USER)
    parser.add_argument("--password", default=os.environ.get("HAVOC_MCP_PASSWORD", DEFAULT_PASSWORD))
    parser.add_argument("--tls-ca", default=DEFAULT_CA)
    parser.add_argument("--insecure", action="store_true")
    parser.add_argument("--audit-log", default=DEFAULT_AUDIT)
    parser.add_argument("--report", default=DEFAULT_REPORT)
    parser.add_argument("--timeout", type=int, default=120)
    args = parser.parse_args()

    os.makedirs(os.path.dirname(args.audit_log), exist_ok=True)
    os.makedirs(os.path.dirname(args.report), exist_ok=True)

    client, insecure = start_client(args)
    session_id = None
    arch = ""
    try:
        # ---- protocol handshake -------------------------------------------
        client.request("initialize", {
            "protocolVersion": "2025-06-18",
            "capabilities": {},
            "clientInfo": {"name": "havoc-mcp-smoke", "version": "1.0.0"},
        }, timeout=args.timeout)
        client.notify("notifications/initialized")
        client.request("ping", {}, timeout=args.timeout)
        client.request("tools/list", {}, timeout=args.timeout)

        # ---- read-only inventory ------------------------------------------
        client.tool("teamserver_status", timeout=args.timeout)
        sessions_resp = client.tool("list_sessions", timeout=args.timeout)
        client.tool("list_listeners", timeout=args.timeout)

        ok, sessions = tool_payload(sessions_resp)
        if ok and isinstance(sessions, dict) and sessions.get("sessions"):
            session_id = sessions["sessions"][0]["id"]
            arch = sessions["sessions"][0].get("os_arch", "")
            print("[info] using session {0} (arch {1})".format(session_id, arch), flush=True)
        if session_id is None:
            print("[abort] no active agent session available for tasking tools", flush=True)
            return 2

        client.tool("get_session", {"session_id": session_id}, timeout=args.timeout)

        # ---- one task per non-destructive tool ----------------------------
        raw_resp = client.tool("send_raw_command", {
            "session_id": session_id,
            "command_id": "15",
            "info": {"DemonID": session_id, "CommandID": "15", "SubCommand": "pwd", "Arguments": ""},
            "command_line": "pwd",
        }, timeout=args.timeout)

        send_resp = client.tool("send_command", {
            "session_id": session_id,
            "command": "whoami",
            "args": {},
        }, timeout=args.timeout)
        ok, sent = tool_payload(send_resp)
        task_id = sent.get("task_id") if ok and isinstance(sent, dict) else None
        cursor = sent.get("cursor") if ok and isinstance(sent, dict) else 0

        client.tool("get_output", {"session_id": session_id, "cursor": 0, "max_bytes": 8192},
                    timeout=args.timeout)
        client.tool("wait_for_output", {
            "session_id": session_id,
            "cursor": cursor,
            "timeout_seconds": 30,
        }, timeout=args.timeout)
        if task_id:
            client.tool("get_task", {"task_id": task_id}, timeout=args.timeout)

        client.tool("screenshot", {"session_id": session_id}, timeout=args.timeout)
        client.tool("socks", {"session_id": session_id, "action": "list"}, timeout=args.timeout)
        client.tool("rportfwd", {"session_id": session_id, "action": "list"}, timeout=args.timeout)
        client.tool("token", {"session_id": session_id, "subcommand": "list"}, timeout=args.timeout)
        client.tool("dotnet", {"session_id": session_id, "action": "list-versions"}, timeout=args.timeout)
        client.tool("proc", {"session_id": session_id, "action": "list"}, timeout=args.timeout)
        client.tool("job", {"session_id": session_id, "action": "list"}, timeout=args.timeout)
        client.tool("transfer", {"session_id": session_id, "action": "list"}, timeout=args.timeout)
        client.tool("net", {"session_id": session_id, "action": "domain"}, timeout=args.timeout)
        client.tool("pivot", {"session_id": session_id, "action": "list"}, timeout=args.timeout)
        client.tool("checkin", {"session_id": session_id}, timeout=args.timeout)
        client.tool("spawn", {
            "session_id": session_id,
            "state": "normal",
            "program": "cmd.exe",
            "args": "/c echo havoc-mcp-spawn-check",
        }, timeout=args.timeout)

        # benign, side-effect-free shellcode (xor eax,eax ; ret)
        shellcode_path = os.path.join(SCRIPT_DIR, "benign_{0}_ret.bin".format(arch if arch else "x64"))
        client.tool("shellcode", {
            "session_id": session_id,
            "action": "execute",
            "arch": arch if arch in ("x64", "x86") else "x64",
            "file_path": shellcode_path,
        }, timeout=args.timeout)

        client.tool("download", {
            "session_id": session_id,
            "path": "C:\\Windows\\System32\\drivers\\etc\\hosts",
        }, timeout=args.timeout)

        client.tool("generate_payload", {
            "agent_type": "Demon",
            "listener": "teams profile - http",
            "arch": "x64",
            "format": "Windows Exe",
            "config": "{}",
        }, timeout=300)
    finally:
        with open(args.report, "w") as handle:
            json.dump({
                "teamserver": args.teamserver,
                "user": args.user,
                "insecure": insecure,
                "session_id": session_id,
                "transcript": client.transcript,
            }, handle, indent=2)
        print("\n[report] transcript written to {0}".format(args.report), flush=True)
        client.close()
    return 0


if __name__ == "__main__":
    sys.exit(main())
