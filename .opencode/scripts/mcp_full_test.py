#!/usr/bin/env python3
"""Controlled live test of the Havoc MCP stdio server.

Non-destructive by design:
  - read-only commands for enumeration
  - benign whoami/pwd/checkin/list commands
  - benign ret-only shellcode blob
  - spawn of a short-lived cmd.exe /c echo
  - download of the Windows hosts file (read only)
  - dll/inline_execute exercised with a nonexistent path (validation error,
    never sent to the agent)
  - a throwaway HTTP listener on 127.0.0.1:18080 that is removed afterwards
Screenshot payload bytes are never written to disk or the transcript.

Every request/response is appended to a JSON transcript after each call.
The script aborts immediately if the teamserver process disappears.
"""

import argparse
import json
import os
import select
import socket
import subprocess
import sys
import time
import traceback

REPO = "/home/kali/Havoc"
HERE = os.path.dirname(os.path.abspath(__file__))
BENIGN = os.path.join(REPO, ".opencode", "scripts", "benign_x64_ret.bin")
BENIGN_X86 = os.path.join(REPO, ".opencode", "scripts", "benign_x86_ret.bin")
STAMP = time.strftime("%Y%m%d-%H%M%S")
TRANSCRIPT = os.path.join(HERE, "havoc-mcp-live-%s.json" % STAMP)
AUDIT = os.path.join(HERE, "havoc-mcp-audit-%s.log" % STAMP)
STDERR = os.path.join(HERE, "havoc-mcp-stderr-%s.log" % STAMP)


def redact(value):
    if isinstance(value, dict):
        out = {}
        for k, v in value.items():
            if k == "payload_b64" and isinstance(v, str):
                out[k] = "<omitted %d chars>" % len(v)
            elif k == "password":
                out[k] = "[REDACTED]"
            else:
                out[k] = redact(v)
        return out
    if isinstance(value, list):
        return [redact(v) for v in value]
    if isinstance(value, str) and len(value) > 4000:
        return value[:4000] + "...<truncated %d chars>" % (len(value) - 4000)
    return value


def brief(p):
    """Extract reportable scalars from a tool result before redaction."""
    if not isinstance(p, dict):
        return {"_raw": str(p)[:200]}
    out = {}
    for k, v in p.items():
        if k == "payload_b64":
            out["payload_b64_len"] = len(v)
            continue
        if k == "events" and isinstance(v, list):
            out["events_count"] = len(v)
            meta, texts = [], []
            for ev in v:
                if not isinstance(ev, dict):
                    continue
                if "payload_size" in ev or "file_extension" in ev:
                    meta.append({kk: ev.get(kk) for kk in
                                 ("command_id", "seq", "payload_size", "file_extension",
                                  "content_type")})
                if ev.get("command_id") not in (10, 128) and ev.get("text") not in ("{}", ""):
                    texts.append(str(ev.get("text"))[:220])
            if meta:
                out["event_meta"] = meta
            if texts:
                out["texts"] = texts
            continue
        if k == "output" and isinstance(v, dict):
            out["output_events_count"] = len(v.get("events", []))
            continue
        if isinstance(v, str):
            out[k] = v[:220] + ("..." if len(v) > 220 else "")
        elif isinstance(v, (int, float, bool)) or v is None:
            out[k] = v
        elif k in ("sessions", "listeners"):
            out[k] = len(v)
    return out


class Mcp:
    def __init__(self, argv, pid):
        self.pid = pid
        self.stderr_handle = open(STDERR, "wb")
        self.proc = subprocess.Popen(
            argv, cwd=REPO, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
            stderr=self.stderr_handle, text=True, bufsize=1,
        )
        self.next_id = 0
        self.transcript = []

    def alive(self):
        return self.proc.poll() is None

    def teamserver_alive(self):
        if not os.path.exists("/proc/%d" % self.pid):
            return False
        try:
            s = socket.create_connection(("127.0.0.1", 40056), timeout=2)
            s.close()
            return True
        except OSError:
            return False

    def _write(self, payload):
        self.proc.stdin.write(json.dumps(payload) + "\n")
        self.proc.stdin.flush()

    def _read(self, request_id, timeout):
        deadline = time.time() + timeout
        while True:
            remaining = deadline - time.time()
            if remaining <= 0:
                raise TimeoutError("no response for id %s after %ss" % (request_id, timeout))
            ready, _, _ = select.select([self.proc.stdout], [], [], remaining)
            if not ready:
                continue
            line = self.proc.stdout.readline()
            if not line:
                tail = ""
                try:
                    with open(STDERR, "r", errors="replace") as h:
                        tail = h.read()[-2000:]
                except Exception:
                    pass
                raise RuntimeError("MCP server closed stdout. stderr tail:\n" + tail)
            line = line.strip()
            if not line:
                continue
            msg = json.loads(line)
            if msg.get("id") == request_id:
                return msg

    def rpc(self, method, params=None, label=None, timeout=120):
        self.next_id += 1
        rid = self.next_id
        req = {"jsonrpc": "2.0", "id": rid, "method": method}
        if params is not None:
            req["params"] = params
        label = label or method
        start = time.time()
        self._write(req)
        try:
            resp = self._read(rid, timeout)
            err = None
        except Exception as exc:
            resp = None
            err = "%s: %s" % (type(exc).__name__, exc)
        elapsed = round(time.time() - start, 2)
        summary = None
        try:
            if resp and "result" in resp and resp["result"].get("content"):
                summary = brief(json.loads(resp["result"]["content"][0]["text"]))
        except Exception:
            summary = None
        entry = {
            "label": label, "request": redact(req),
            "response": redact(resp), "summary": summary,
            "error": err, "seconds": elapsed,
            "ts": time.strftime("%H:%M:%S"),
        }
        self.transcript.append(entry)
        self.save()
        self.report(label, entry)
        if err:
            raise RuntimeError(err)
        if not self.teamserver_alive():
            raise RuntimeError("TEAMSERVER DIED during %s" % label)
        return resp

    def notify(self, method, params=None):
        req = {"jsonrpc": "2.0", "method": method}
        if params is not None:
            req["params"] = params
        self._write(req)
        self.transcript.append({"label": method, "request": redact(req),
                                "response": None, "error": None, "seconds": 0,
                                "ts": time.strftime("%H:%M:%S")})
        self.save()
        print("[NOTIFY] %s" % method, flush=True)

    def tool(self, name, args=None, timeout=120):
        return self.rpc("tools/call", {"name": name, "arguments": args or {}},
                        label="tool:" + name, timeout=timeout)

    def save(self):
        with open(TRANSCRIPT, "w") as h:
            json.dump({"teamserver_pid": self.pid, "transcript": self.transcript},
                      h, indent=2)

    def report(self, label, entry):
        resp = entry["response"]
        if entry["error"]:
            print("[ERR ] %-34s %6ss %s" % (label, entry["seconds"], entry["error"][:300]), flush=True)
            return
        if resp and "error" in resp:
            e = resp["error"]
            print("[ERR ] %-34s %6ss code=%s %s data=%s" % (
                label, entry["seconds"], e.get("code"), e.get("message"),
                str(e.get("data"))[:250]), flush=True)
            return
        summary = ""
        try:
            text = resp["result"]["content"][0]["text"]
            payload = json.loads(text)
            summary = summarize(payload)
        except Exception:
            summary = str(resp)[:250]
        print("[ OK ] %-34s %6ss %s" % (label, entry["seconds"], summary[:400]), flush=True)

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


def summarize(p):
    if not isinstance(p, dict):
        return str(p)[:250]
    keys = []
    for k in ("connected", "user", "profile", "sessions", "listeners", "task_id",
              "cursor", "next_cursor", "timed_out", "completed", "teamserver_path",
              "payload_size", "file_name", "status", "name", "protocol", "text"):
        if k in p:
            v = p[k]
            if k in ("sessions", "listeners") and isinstance(v, list):
                keys.append("%s=%d" % (k, len(v)))
            elif k == "text":
                keys.append("text=%r" % (str(v)[:160],))
            else:
                keys.append("%s=%r" % (k, v))
    ev = p.get("events")
    if isinstance(ev, list):
        keys.append("events=%d" % len(ev))
    task = p.get("task")
    if isinstance(task, dict) and task.get("task_id"):
        keys.append("task_id=%r" % task["task_id"])
    output = p.get("output")
    if isinstance(output, dict) and isinstance(output.get("events"), list):
        keys.append("output_events=%d" % len(output["events"]))
        for e in output["events"]:
            if e.get("command_id") not in (10, 128) and e.get("text") not in ("{}", ""):
                keys.append("first_text=%r" % (str(e.get("text"))[:120],))
                break
    if isinstance(ev, list):
        for e in ev:
            if e.get("command_id") not in (10, 128) and e.get("text") not in ("{}", ""):
                keys.append("first_text=%r" % (str(e.get("text"))[:120],))
                break
    return " ".join(keys)[:500]


def tool_payload(resp):
    try:
        return json.loads(resp["result"]["content"][0]["text"])
    except Exception:
        return None


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--pid", type=int, required=True)
    ap.add_argument("--phases", default="read,task,listener,payload")
    ap.add_argument("--user", default="Neo")
    ap.add_argument("--password", default="password1234")
    ap.add_argument("--session", default="", help="override session id (NameID) when list_sessions is empty")
    ap.add_argument("--arch", default="x64")
    args = ap.parse_args()
    phases = set(args.phases.split(","))

    argv = [
        os.path.join(REPO, "havoc"), "mcp",
        "--teamserver", "wss://127.0.0.1:40056/havoc/",
        "--user", args.user, "--password", args.password,
        "--audit-log", AUDIT,
        "--loot-root", os.path.join(REPO, "data", "loot"),
        "--max-output-bytes", "262144", "--insecure",
    ]
    print("[start] %s" % " ".join(argv), flush=True)
    print("[start] transcript=%s audit=%s stderr=%s" % (TRANSCRIPT, AUDIT, STDERR), flush=True)
    m = Mcp(argv, args.pid)
    try:
        time.sleep(2)
        if not m.alive():
            raise RuntimeError("MCP server exited at startup; see " + STDERR)

        # ---- protocol handshake -----------------------------------------
        m.rpc("initialize", {"protocolVersion": "2025-06-18", "capabilities": {},
                             "clientInfo": {"name": "havoc-mcp-live", "version": "1.0"}})
        m.notify("notifications/initialized")
        m.rpc("ping", {})
        tools_resp = m.rpc("tools/list", {})
        tools = tools_resp["result"]["tools"]
        names = sorted(t["name"] for t in tools)
        print("[info] tools/list -> %d tools: %s" % (len(names), ", ".join(names)), flush=True)

        # ---- read-only ---------------------------------------------------
        sid = args.session
        arch = args.arch
        if "read" in phases:
            m.tool("teamserver_status")
            m.tool("list_listeners")
            sessions = None
            for attempt in range(3):
                resp = m.tool("list_sessions")
                sessions = tool_payload(resp) or {}
                if sessions.get("sessions"):
                    break
                time.sleep(1)
            all_sessions = (sessions or {}).get("sessions", [])
            active = [s for s in all_sessions if s.get("active")]
            chosen = (active or all_sessions or [None])[0]
            if not sid:
                if not chosen:
                    raise RuntimeError("no session available for tasking")
                sid = chosen["id"]
                arch = chosen.get("os_arch") or "x64"
            print("[info] session=%s arch=%s (list_sessions active=%d total=%d, override=%s)" % (
                sid, arch, len(active), len(all_sessions), bool(args.session)), flush=True)
            m.tool("get_session", {"session_id": sid})
            out = m.tool("get_output", {"session_id": sid, "cursor": 0, "max_bytes": 8192})
            cur = (tool_payload(out) or {}).get("next_cursor", 0)
            m.tool("wait_for_output", {"session_id": sid, "cursor": cur, "timeout_seconds": 2},
                   timeout=20)
            m.tool("get_task", {"task_id": "DEADBEEF"})
        if not sid:
            raise RuntimeError("tasking phases need --session because list_sessions returned none")

        if "probe" in phases:
            r = m.tool("send_command", {"session_id": sid, "command": "whoami", "args": {}})
            p = tool_payload(r) or {}
            m.tool("wait_for_output", {"session_id": sid, "cursor": p.get("cursor", 0),
                                       "timeout_seconds": 15}, timeout=35)
            if p.get("task_id"):
                m.tool("get_task", {"task_id": p["task_id"]})

        # ---- safe tasking -------------------------------------------------
        if "task" in phases:
            def finish(label, resp, expected=None, seconds=10):
                p = tool_payload(resp) or {}
                tid, cursor = p.get("task_id"), p.get("cursor", 0)
                if not tid:
                    return
                deadline = time.time() + seconds
                while time.time() < deadline:
                    r = m.tool("wait_for_output", {
                        "session_id": sid, "cursor": cursor, "timeout_seconds": 2,
                    }, timeout=10)
                    wp = tool_payload(r) or {}
                    evs = wp.get("events", [])
                    if not evs:
                        continue
                    cursor = wp.get("next_cursor", cursor)
                    if expected is not None:
                        hit = any(e.get("command_id") == expected for e in evs)
                    else:
                        hit = any(e.get("command_id") not in (10, 128)
                                  and e.get("text") not in ("{}", "") for e in evs)
                    if hit:
                        break
                m.tool("get_task", {"task_id": tid})

            r = m.tool("send_command", {"session_id": sid, "command": "whoami", "args": {}})
            finish("whoami", r, expected=4112, seconds=10)

            r = m.tool("send_raw_command", {
                "session_id": sid, "command_id": "15",
                "info": {"DemonID": sid, "CommandID": "15", "SubCommand": "pwd", "Arguments": ""},
                "command_line": "pwd",
            })
            finish("pwd", r, expected=15, seconds=10)

            r = m.tool("screenshot", {"session_id": sid}, timeout=60)
            finish("screenshot", r, expected=2510, seconds=20)
            r = m.tool("socks", {"session_id": sid, "action": "list"})
            finish("socks", r, expected=2540)
            r = m.tool("rportfwd", {"session_id": sid, "action": "list"})
            finish("rportfwd", r, expected=2540)
            r = m.tool("token", {"session_id": sid, "subcommand": "list"})
            finish("token", r, expected=40)
            r = m.tool("dotnet", {"session_id": sid, "action": "list-versions"})
            finish("dotnet", r, expected=None)
            r = m.tool("proc", {"session_id": sid, "action": "list"})
            finish("proc", r, expected=12)
            r = m.tool("job", {"session_id": sid, "action": "list"})
            finish("job", r, expected=21)
            r = m.tool("transfer", {"session_id": sid, "action": "list"})
            finish("transfer", r, expected=2530)
            r = m.tool("net", {"session_id": sid, "action": "domain"})
            finish("net", r, expected=2100)
            r = m.tool("pivot", {"session_id": sid, "action": "list"})
            finish("pivot", r, expected=2520)
            r = m.tool("checkin", {"session_id": sid})
            finish("checkin", r, expected=100)
            r = m.tool("spawn", {"session_id": sid, "state": "normal", "program": "cmd.exe",
                                 "args": "/c echo havoc-mcp-spawn-check"})
            finish("spawn", r, expected=4112)
            sc = BENIGN if arch == "x64" else BENIGN_X86
            r = m.tool("shellcode", {"session_id": sid, "action": "execute",
                                     "arch": arch if arch in ("x64", "x86") else "x64",
                                     "file_path": sc}, timeout=60)
            finish("shellcode", r, expected=24)
            r = m.tool("download", {"session_id": sid,
                                    "path": "C:\\Windows\\System32\\drivers\\etc\\hosts"},
                       timeout=60)
            finish("download", r, expected=15, seconds=25)
            # Validation-only: files do not exist, so the task is never sent.
            m.tool("dll", {"session_id": sid, "action": "inject", "pid": 1,
                           "file_path": os.path.join(HERE, "does-not-exist.dll")})
            m.tool("inline_execute", {"session_id": sid,
                                      "bof_path": os.path.join(HERE, "does-not-exist.o")})

        # ---- metadata re-check (payload/screenshot exact sizes) -----------
        if "meta" in phases:
            m.tool("screenshot", {"session_id": sid}, timeout=60)
            full_cfg = json.dumps({
                "Sleep": "2", "Jitter": "15",
                "Sleep Technique": "WaitForSingleObjectEx",
                "Sleep Jmp Gadget": "jmp rax", "Stack Duplication": False,
                "Proxy Loading": "None (LdrLoadDll)",
                "Amsi/Etw Patch": "Hardware breakpoints",
                "Injection": {
                    "Alloc": "Win32", "Execute": "Win32",
                    "Spawn64": "C:\\Windows\\System32\\notepad.exe",
                    "Spawn32": "C:\\Windows\\SysWOW64\\notepad.exe",
                },
            })
            m.tool("generate_payload", {"agent_type": "Demon",
                                        "listener": "teams profile - http",
                                        "arch": "x64", "format": "Windows Shellcode",
                                        "config": full_cfg}, timeout=300)

        # ---- listener lifecycle (throwaway, cleaned up) -------------------
        if "listener" in phases:
            name = "mcp-smoke-http"
            cfg = {
                "HostBind": "127.0.0.1", "PortBind": 18080,
                "Hosts": ["127.0.0.1"], "HostRotation": "round-robin",
                "PortConn": 18080, "HostHeader": "",
                "UserAgent": "Mozilla/5.0 (Havoc MCP smoke)",
                "Headers": ["X-MCP-Smoke: 1"], "Uris": ["/mcp-smoke"],
                "Secure": False, "KillDate": "",
            }
            def listener_names():
                r = m.tool("list_listeners")
                p = tool_payload(r) or {}
                return [l.get("name") for l in p.get("listeners", [])]

            def wait_present(want, seconds=10):
                deadline = time.time() + seconds
                names = []
                while time.time() < deadline:
                    names = listener_names()
                    if (name in names) == want:
                        return names
                    time.sleep(1)
                return names

            started = False
            try:
                m.tool("start_listener", {"name": name, "protocol": "http", "config": cfg})
                started = True
                print("[info] listeners after start: %s" % wait_present(True), flush=True)
                edit_cfg = dict(cfg)
                edit_cfg["Protocol"] = "Http"  # required by dispatch; see report
                edit_cfg["UserAgent"] = "Mozilla/5.0 (Havoc MCP smoke edited)"
                m.tool("edit_listener", {"name": name, "config": edit_cfg})
                print("[info] listeners after edit: %s" % wait_present(True), flush=True)
            finally:
                if started:
                    try:
                        m.tool("remove_listener", {"name": name})
                        print("[info] listeners after remove: %s" % wait_present(False, 15), flush=True)
                    except Exception as exc:
                        print("[warn] cleanup remove_listener failed: %s" % exc, flush=True)

        # ---- payload generation ------------------------------------------
        if "payload" in phases:
            full_cfg = json.dumps({
                "Sleep": "2", "Jitter": "15",
                "Sleep Technique": "WaitForSingleObjectEx",
                "Sleep Jmp Gadget": "jmp rax", "Stack Duplication": False,
                "Proxy Loading": "None (LdrLoadDll)",
                "Amsi/Etw Patch": "Hardware breakpoints",
                "Injection": {
                    "Alloc": "Win32", "Execute": "Win32",
                    "Spawn64": "C:\\Windows\\System32\\notepad.exe",
                    "Spawn32": "C:\\Windows\\SysWOW64\\notepad.exe",
                },
            })
            try:
                m.tool("generate_payload", {"agent_type": "Demon",
                                            "listener": "teams profile - http",
                                            "arch": "x64", "format": "Windows Shellcode",
                                            "config": full_cfg}, timeout=300)
            except Exception as exc:
                print("[warn] generate_payload failed: %s" % exc, flush=True)

        print("\n[done] transcript=%s" % TRANSCRIPT, flush=True)
        return 0
    except Exception:
        print("\n[ABORT] %s" % traceback.format_exc(), flush=True)
        return 1
    finally:
        m.save()
        m.close()


if __name__ == "__main__":
    sys.exit(main())
