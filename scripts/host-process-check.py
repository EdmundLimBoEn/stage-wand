#!/usr/bin/env python3
"""Check the built Linux host's desktop status, code rotation, and shutdown."""

import json
from pathlib import Path
import queue
import signal
import socket
import subprocess
import sys
import threading
import time

binary = str(Path(sys.argv[1] if len(sys.argv) > 1 else "host/stagewand-host").resolve())
bad = subprocess.run([binary, "--code=bad"], capture_output=True, text=True, timeout=5)
assert bad.returncode == 2 and "four ASCII digits" in bad.stderr
process = subprocess.Popen(
    [binary, "--dry-run", "--json-status", "--code=0012", "--name=StageWandReleaseCheck"],
    stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, bufsize=1,
)
lines = queue.Queue(maxsize=64)


def read_lines():
    for line in process.stdout:
        lines.put(line)
    lines.put(None)


threading.Thread(target=read_lines, daemon=True).start()


def next_status(check, seconds=35):
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        try:
            line = lines.get(timeout=0.2)
        except queue.Empty:
            assert process.poll() is None, "host exited unexpectedly"
            continue
        assert line is not None, "host closed its status stream"
        try:
            value = json.loads(line)
        except json.JSONDecodeError:
            continue
        if value.get("type") == "status" and check(value):
            return value
    raise AssertionError("desktop status deadline exceeded")


try:
    first = next_status(lambda value: True, seconds=10)
    assert first["code"] == "0012" and first["bluetooth"] == "starting"
    assert first["inputReady"] is False
    live = next_status(lambda value: value["bluetooth"] != "starting")
    assert live["inputReady"] is False
    port = int(live["address"].rsplit(":", 1)[1])
    with socket.create_connection(("127.0.0.1", port), timeout=2):
        pass
    process.stdin.write("k\n")
    process.stdin.flush()
    rotated = next_status(lambda value: value["code"] != "0012")
    assert len(rotated["code"]) == 4 and rotated["code"].isascii() and rotated["code"].isdigit()
    process.send_signal(signal.SIGTERM)
    assert process.wait(timeout=20) == 0
    try:
        with socket.create_connection(("127.0.0.1", port), timeout=0.2):
            pass
    except OSError:
        pass
    else:
        raise AssertionError("host port remained open after shutdown")
    print("PASS: early desktop status, dry-run readiness, live LAN, kick/code rotation, SIGTERM cleanup, CLI validation")
    print("Bluetooth status:", live["bluetooth"])
finally:
    if process.poll() is None:
        process.kill()
        process.wait(timeout=5)
