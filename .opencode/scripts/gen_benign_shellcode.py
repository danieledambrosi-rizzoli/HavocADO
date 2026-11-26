#!/usr/bin/env python3
#
# gen_benign_shellcode.py — write harmless "do nothing" shellcode blobs.
#
# PURPOSE
#   Produce a minimal, side-effect-free shellcode file that can be handed to
#   the Havoc MCP `shellcode` tool (action "execute") without changing the
#   target: the payload only zeroes the return register and returns.
#
# ARGUMENTS
#   --out-dir <path>   Directory that receives the blobs.
#                      Default: the directory holding this script.
#
# OUTPUTS
#   <out-dir>/benign_x64_ret.bin   3 bytes: xor eax,eax ; ret
#   <out-dir>/benign_x86_ret.bin   3 bytes: xor eax,eax ; ret
#   stdout: one line per generated file with its size and sha256.
#
# NOTES
#   The blobs are architecture neutral (the same three bytes are valid x86 and
#   x64); separate names only document which arch the MCP call declares.
#   Nothing outside --out-dir is written.

import argparse
import hashlib
import os

# xor eax,eax (31 C0) ; ret (C3)
BYTES = bytes([0x31, 0xC0, 0xC3])


def main() -> int:
    parser = argparse.ArgumentParser()
    default_out = os.path.dirname(os.path.abspath(__file__))
    parser.add_argument("--out-dir", default=default_out)
    args = parser.parse_args()

    os.makedirs(args.out_dir, exist_ok=True)
    for name in ("benign_x64_ret.bin", "benign_x86_ret.bin"):
        path = os.path.join(args.out_dir, name)
        with open(path, "wb") as handle:
            handle.write(BYTES)
        digest = hashlib.sha256(BYTES).hexdigest()
        print(f"{path} {len(BYTES)} bytes sha256={digest}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
