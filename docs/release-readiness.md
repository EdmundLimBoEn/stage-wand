# Release readiness: 2026-09-22

Work is isolated on `feat/release-ready` in `../stage-wand-trees/feat-release-ready`. The original checkout is unchanged. This record covers local verification before opening the PR to `dev`; it does not certify a published release.

## Changes

- Debian/Ubuntu packages use binary-derived library dependencies and an Ubuntu 22.04 build baseline. Both Linux packages include Bluetooth, Wayland, Qt platform plugins, and icon-theme dependencies. Installation refreshes only the uinput device rule; removal and upgrade paths are tested.
- Swift, Go, and Kotlin share executable valid/invalid protocol fixtures. Decoders reject ambiguous duplicate keys, unknown fields, wrong JSON types, malformed Unicode/numbers, and trailing frames. BLE UUIDs, the 512-byte frame ceiling, and the 80-byte minimum command capacity are checked together.
- Bluetooth initialization waits for usable characteristics, sufficient MTU, and notifications. Connection attempts ignore stale callbacks, writes have deadlines, and queues are bounded. Linux handles BlueZ restart and adapter changes; Windows corrects WinRT ABI calls and supervises runtime advertisement failures.
- A single authenticated controller owns input across Bluetooth and LAN. Five failed PIN checks in 30 seconds trigger a shared cooldown; code rotation clears it. Disconnected key/click commands are discarded. Pairing-code generation fails closed when secure randomness is unavailable and never reuses the previous code during rotation.
- Input failures attempt to release every held modifier/button and preserve pending releases for recovery. Linux accumulates small scroll deltas correctly. Runtime readiness reports actual failures. The desktop bounds and validates host output and receives an initial status before potentially slow Bluetooth setup.
- Release publication depends on protocol, host, Windows, Apple, Android, Arch, and Debian matrix jobs. Releases include checksums/source provenance and cannot be overwritten by a superseded workflow. Go dependencies and the compiler were updated; vulnerability scanning is a CI gate.

## Verification

| Check | Evidence |
| --- | --- |
| Go | `go test -race ./...`, `go vet ./...`, and `go mod verify` pass. Includes shared ownership, input failure recovery, strict decoding, and isolated D-Bus integration. |
| Vulnerabilities | `govulncheck@v1.8.0` reports **No vulnerabilities found** for Linux and Windows using Go 1.26.8. |
| Linux/Windows binaries | Native Linux and Windows AMD64/ARM64 cross-builds pass. Windows-specific test binaries compile; native Windows execution is still required. |
| WebSocket process | Smoke passes auth deadline, wrong PIN, non-auth first frame, displacement, 200 ordered moves, malformed deltas, live pong, and missing-pong shutdown. |
| Desktop host process | Early JSON status, real LAN availability, truthful dry-run readiness, stdin code rotation, invalid CLI code rejection, SIGTERM exit, and port cleanup pass. |
| Qt desktop | Native Release build and lifecycle suite pass. |
| Debian compatibility | The Ubuntu 22.04-built package passes install/reinstall/remove/purge on Ubuntu 22.04/24.04 and Debian 12/13 with `--no-install-recommends`. |
| Packaged desktop | All four distributions pass offscreen, native headless Wayland (Weston), and X11 (Xvfb) launch, single-instance activation, and child-host cleanup: **12 GUI lifecycle cases**. |
| Arch | Actual `makepkg`, Qt/Go tests, package installation, and CLI smoke pass in Arch Linux. |
| Swift | Swift 6 executable protocol, queue, URL, and session-security checks pass in the official Swift Linux container. Production Apple Bluetooth sources pass deterministic CoreBluetooth-substitute tests, including stale callbacks, power loss, MTU, backpressure, and ownership. |
| Shared contract | 19 command fixtures, 6 reply fixtures, and 78 malformed frames, plus raw valid encodings; Bun checks schema/constants and Swift/Go/Kotlin execute the corpus. |
| Android | **23 protocol + 38 app tests pass**, zero failures/errors. Debug and unsigned-release APKs build. Lint completes with zero errors and seven warnings for dependency versions, redundant API guards, and attributes used on newer Android versions. |

The local machine exposes a Bluetooth adapter but has no BlueZ service installed and no graphical input seat. The real host correctly reports BlueZ unavailable while continuing to serve LAN. BlueZ recovery is exercised against an isolated D-Bus service, not certified against this physical radio. No real input events were injected into this machine.

## Local candidate artifacts

Final package checksums were verified, and the host extracted from the final `.deb` also passed the process lifecycle check. The final `.deb` rebuild's installed-file hashes and control metadata match the artifact tested across all four distributions.

| Artifact | Location |
| --- | --- |
| Debian/Ubuntu amd64, version 0.1.0-3 | `.out/packages/stagewand-host_amd64.deb` |
| Arch x86_64, version 0.1.0-3 | `.out/packages/stagewand-host-x86_64.pkg.tar.zst` |
| Package SHA-256 hashes | `.out/packages/SHA256SUMS` |
| Android debug APK | `android/app/build/outputs/apk/debug/app-debug.apk` |
| Android unsigned release APK | `android/app/build/outputs/apk/release/app-release-unsigned.apk` |

These are local candidates from the current worktree, not published releases. Native Windows test execution and the newly configured Apple CI jobs have not run here. Workflow syntax passes `actionlint`.

## Device and distribution gates

Follow the unchecked acceptance steps in [HUMANS.md](../HUMANS.md). The prior successful Arch/iPhone test is preserved there; it does not certify the changed build.

- Run native macOS/iOS and Windows CI for this exact revision. A Linux Swift compiler and fake radio cannot replace Apple frameworks or WinRT execution.
- Exercise both phones against macOS, Windows, Arch, Debian, and Ubuntu over Bluetooth and Wi-Fi. Test lock/unlock, radio interruption, rapid handover, actual pointer/click/scroll/key injection, and locked-screen volume controls.
- Older BlueZ versions used by the supported distributions broadcast characteristic notifications. An unauthorized secondary phone cannot inject input and is dropped when it tries, but it can observe status notifications. Use one active Bluetooth remote. Fully isolating notifications on these versions requires a negotiated protocol change.
- Windows recovers advertising interruptions after successful initialization. If Bluetooth is unavailable during initial provider creation, enable it and relaunch. A lost Linux system D-Bus connection also requires relaunch; ordinary BlueZ restart/adapter power recovery is automatic.
- A Bluetooth connection needs at least 80 writable bytes. Links that cannot negotiate this fail explicitly; Wi-Fi remains the fallback. This preserves the existing one-JSON-object-per-ATT-write protocol.
- Phone release signing/provisioning and a distribution license remain owner decisions. There is no project license file; Arch correctly retains `LicenseRef-NOASSERTION`. An unsigned Android release APK is a build artifact, not an installable signed distribution.

## Reproduce the main checks

```sh
bun scripts/protocol-fixtures-check.ts
(cd host && go test -race ./... && go vet ./... && go mod verify)
make -C host desktop
ctest --test-dir host/desktop/build --output-on-failure
bun scripts/ws-smoke.ts --spawn-host
python3 scripts/host-process-check.py host/stagewand-host
bash scripts/bluetooth-check.sh                  # Swift 6 required
(cd android && ./gradlew :protocol:test :app:testDebugUnitTest :app:lintDebug :app:assembleDebug :app:assembleRelease)
make -C host deb
```

`host/debian/smoke.sh` intentionally requires a disposable root container: it installs, reinstalls, removes, and purges the candidate package. CI runs it once per supported distribution. Native Apple and Windows commands are in [.github/workflows/cross-platform.yml](../.github/workflows/cross-platform.yml).
