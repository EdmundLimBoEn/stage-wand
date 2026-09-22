#!/usr/bin/env python3
"""Exercise the installed desktop/host pair on a private session bus."""

import os
from pathlib import Path
import socket
import subprocess
import sys
import tempfile
import time


def children(pid):
    path = Path(f"/proc/{pid}/task/{pid}/children")
    return [int(child) for child in path.read_text().split()] if path.exists() else []


def wait_for(predicate, message):
    deadline = time.monotonic() + 15
    while time.monotonic() < deadline:
        if predicate():
            return
        time.sleep(0.05)
    raise RuntimeError(message)


def host_child(pid):
    for child in children(pid):
        if Path(f"/proc/{child}/exe").resolve() == Path("/usr/bin/stagewand-host"):
            return child
    return None


def host_listens():
    try:
        with socket.create_connection(("127.0.0.1", 8787), timeout=0.25):
            return True
    except OSError:
        return False


platform = sys.argv[1] if len(sys.argv) > 1 else "offscreen"
with tempfile.TemporaryDirectory(prefix="stagewand-runtime-") as runtime:
    env = {**os.environ, "XDG_RUNTIME_DIR": runtime, "QT_QPA_PLATFORM": platform}
    compositor = None
    desktop = None
    try:
        if platform == "wayland":
            env["WAYLAND_DISPLAY"] = "stagewand-smoke"
            compositor = subprocess.Popen([
                "weston", "--backend=headless-backend.so", "--socket=stagewand-smoke",
                "--idle-time=0", "--no-config",
            ], env=env)
            wait_for(lambda: Path(runtime, "stagewand-smoke").exists(), "Wayland compositor did not start")
        desktop = subprocess.Popen(["/usr/bin/stagewand"], env=env)
        wait_for(lambda: host_child(desktop.pid), "desktop did not start its host")
        host_pid = host_child(desktop.pid)
        wait_for(host_listens, "installed host did not start its LAN listener")
        second = subprocess.run(["/usr/bin/stagewand"], env=env, timeout=15, check=False)
        assert second.returncode == 0, "second launch could not activate the desktop"
        assert desktop.poll() is None, "desktop exited unexpectedly"
        assert children(desktop.pid) == [host_pid], "second launch duplicated the host"
        desktop.terminate()
        assert desktop.wait(timeout=10) == 0, "desktop did not shut down cleanly"
        wait_for(lambda: not Path(f"/proc/{host_pid}").exists(), "desktop left its host running")
    finally:
        if desktop is not None and desktop.poll() is None:
            desktop.kill()
            desktop.wait(timeout=10)
        if compositor is not None:
            compositor.terminate()
            compositor.wait(timeout=10)

print(f"PASS: {platform} desktop runs as a regular user, activates once, and stops its host")
