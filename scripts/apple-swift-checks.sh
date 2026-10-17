#!/usr/bin/env bash
# Swift 6 protocol, command-queue, URL, square-unlock, session-security and
# Bluetooth failure-path checks shared by the iOS remote and the macOS host.
# Needs only a Swift 6 toolchain (no signing, no simulator). Used by
# cross-platform.yml and by ios-testflight.yml before archiving.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "${root}"
out="${RUNNER_TEMP:-$(mktemp -d)}"

check() {
  local name="$1"
  shift
  echo "::group::${name}"
  swiftc -swift-version 6 "$@" -o "${out}/${name}"
  "${out}/${name}"
  echo "::endgroup::"
}

check protocol-check Shared/Protocol.swift Shared/Bluetooth.swift scripts/protocol-check.swift
check command-queue-check Shared/Protocol.swift Shared/CommandQueue.swift scripts/command-queue-check.swift
check connection-url-check Shared/ConnectionURL.swift scripts/connection-url-check.swift
check square-unlock-check Shared/SquareUnlock.swift scripts/square-unlock-check.swift
check session-security-check mac/Sources/AuthenticationLimiter.swift mac/Sources/PeerOwnership.swift scripts/session-security-check.swift

if [ "${SKIP_BLUETOOTH_CHECK:-0}" != "1" ]; then
  bash scripts/bluetooth-check.sh
fi
