#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT
swiftc -swift-version 6 -parse-as-library -emit-module -emit-object \
  -module-name CoreBluetooth -emit-module-path "$test_dir/CoreBluetooth.swiftmodule" \
  scripts/apple-bluetooth-fakes/CoreBluetooth.swift -o "$test_dir/CoreBluetooth.o"
link_sources=()
if [[ "$(uname -s)" == "Darwin" ]]; then
  link_sources=(-D LINK_CHECK Shared/ConnectionURL.swift Shared/CommandQueue.swift
    ios/StageWand/Discovery.swift ios/StageWand/LocalSocket.swift ios/StageWand/Link.swift)
fi
swiftc -swift-version 6 "${link_sources[@]}" -I "$test_dir" "$test_dir/CoreBluetooth.o" \
  Shared/Protocol.swift Shared/Bluetooth.swift \
  mac/Sources/AuthenticationLimiter.swift mac/Sources/PeerOwnership.swift \
  ios/StageWand/BluetoothClient.swift mac/Sources/BluetoothServer.swift \
  scripts/bluetooth-check.swift -o "$test_dir/bluetooth-check"
"$test_dir/bluetooth-check"
