Implement Stage Wand now. You are file owner 2. User prioritizes speed; use medium effort. Do not delegate, commit, push or change other files. Ignore CLAUDE.md. Shared files are being edited concurrently by other owners.
FROZEN CONTRACT
 (write in Wave 0, never change after minute 8)
Port 8787 (fallback 8788-8790, shown in the Mac popover). Bonjour type `_stagewand._tcp`. Mac target macOS 14, iOS target 17. Team `U968QPQQ67`, bundle ids `systems.edmundlim.stagewand` (iOS) and `systems.edmundlim.stagewand.mac`. Repo root = this folder, flat layout:

```
Shared/Protocol.swift              # ONLY protocol source. mac/Sources/Protocol.swift is a symlink to it (SwiftPM cannot reference ../Shared); ios/project.yml lists ../Shared as a source path.
mac/Package.swift                  # executableTarget StageWandMac, macOS 14
mac/Sources/{Entry,App,Session,Server,Input}.swift
mac/Makefile                       # `make app`: swift build -c release, then wraps the binary in StageWandMac.app (Info.plist with CFBundleIdentifier, LSUIElement=true) so TCC and the firewall key on a bundle id, not a path
ios/project.yml                    # XcodeGen; target StageWand; UIBackgroundModes [audio]; NSLocalNetworkUsageDescription; NSBonjourServices [_stagewand._tcp]; NSAppTransportSecurity.NSAllowsLocalNetworking true (IP literals are exempt from ATS anyway); automatic signing team U968QPQQ67
ios/StageWand/{StageWandApp,ContentView,SettingsView,Link,Discovery,Volume,Haptics}.swift
ios/StageWand/Pointer/TrackpadView.swift
ios/StageWand/Resources/silence.wav     # 2 s of zeros, generated in Wave 0: python3 -c "import wave;w=wave.open('ios/StageWand/Resources/silence.wav','w');w.setnchannels(1);w.setsampwidth(2);w.setframerate(44100);w.writeframes(b'\0'*176400);w.close()"
scripts/ws-smoke.ts  scripts/rehearse.md  README.md  HUMANS.md
```

`Shared/Protocol.swift` (coordinator writes it complete in Wave 0, with hand-written `Codable` for both enums):
```swift
enum Command: Equatable {                     // phone → Mac
  case auth(code: String)
  case move(dx: Double, dy: Double)           // screen px, +x right, +y down, |d| ≤ 400
  case click(Button)
  case scroll(dx: Double, dy: Double)         // pixel units
  case key(Key)
  case chord(Chord)
}
enum Button: String, Codable { case left, right }
enum Key: String, Codable { case left, right, esc }                          // keycodes 123, 124, 53
enum Chord: String, Codable { case spaceLeft, spaceRight, missionControl }   // Ctrl + 123 / 124 / 126
enum Reply: Equatable {                       // Mac → phone
  case status
  case bye(reason: String)                    // kicked | displaced | badauth
}
```
Wire JSON, flat `t` discriminator: `{"t":"auth","code":"4821"}` `{"t":"move","dx":3.2,"dy":-1}` `{"t":"click","b":"left"}` `{"t":"scroll","dx":0,"dy":40}` `{"t":"key","k":"esc"}` `{"t":"chord","k":"spaceRight"}` `{"t":"status"}` `{"t":"bye","reason":"displaced"}`.
Rules: first frame must be `auth` within 2 s or the server closes. Wrong code → `bye badauth` + close. Valid code displaces the current peer (old one gets `bye displaced`). Server pings every 3 s and drops a peer after 6 s without pong. Phone accumulates `move`/`scroll` deltas and flushes the sum on a 60 Hz timer (never drops distance). Non-finite numbers → drop frame. **Volume presses send `key` unconditionally; ARM gates only move/click/scroll/chord.** `Link` buffers up to 8 `key`/`click`/`chord` commands while not `authed` and flushes them on `authed`.

iOS `@AppStorage` keys (frozen; only these): `sensitivity: Double = 1.0` (0.5-3), `pairingCode: String = ""`, `manualHost: String = ""` (host or host:port), `pointerMode: String = "trackpad"`.

Stubs (coordinator writes; bodies no-op so both builds are green):
- Mac `Entry.swift` (coordinator owns): `@main enum Entry { static func main() { if CommandLine.arguments.contains("--selftest") { Input.selfTest(); exit(0) }; if CommandLine.arguments.contains("--serve") { /* start Server headless with a fixed code "0000", print port, RunLoop.main.run() */ }; StageWandApp.main() } }`. `App.swift` declares `struct StageWandApp: App` WITHOUT `@main`.
- Mac `@MainActor final class Session: ObservableObject { @Published var code: String; @Published var peer: String?; @Published var port: UInt16; @Published var axGranted: Bool; func rotateCode(); func kick() }`
- Mac `final class Server { init(session: Session, onCommand: @escaping @Sendable (Command) -> Void); func start() throws -> UInt16; func kick() }` (Server hops to `MainActor` itself with `Task { @MainActor in }` before touching `session`; `onCommand` is called on the main actor.)
- Mac `enum Input { static func apply(_ c: Command); static func accessibilityGranted(prompt: Bool) -> Bool; static func selfTest() }`
- iOS `@MainActor final class Link: ObservableObject { enum State: Equatable { case searching, connecting(String), enterCode, authed, disconnected, localNetworkDenied }; @Published var state: State; @Published var macName: String?; func send(_ c: Command); func start() }`
- iOS `enum PointerMode: String, CaseIterable { case trackpad }` (joystick / gyro cases only if a stretch lands)
- iOS `final class Volume { init(onUp: @escaping () -> Void, onDown: @escaping () -> Void); func start(); func stop(); var pressCount: Int }`
Concurrency: Network callbacks arrive on dispatch queues; every `Session` / `Input` / `Link` touch hops to `MainActor` via `Task { @MainActor in }`. Swift 6 strict concurrency is on; do not fight it with `@unchecked Sendable`.


YOUR ROW
| 2 | Astra **medium** | `mac/Sources/Server.swift`, `scripts/ws-smoke.ts` | `NWListener(using: params)` where params = TCP + `NWProtocolWebSocket.Options(autoReplyPing: true)`; try 8787, then 8788-8790; `listener.service = NWListener.Service(name: Host.current().localizedName ?? "Mac", type: "_stagewand._tcp")`; receive text frames, decode `Command`; auth-within-2-s, displace-on-valid-code, ping 3 s / drop 6 s; `Task { @MainActor in session.peer = …; onCommand(cmd) }`; `kick()` sends `bye kicked` and closes. Encode `Reply` for outbound. | `swift run StageWandMac --serve` in one terminal, `bun scripts/ws-smoke.ts` (Bun's `WebSocket`, code "0000") prints PASS: no auth → closed ≈2 s; bad code → `bye badauth`; good → `status`; second good → first gets `bye displaced`; 200 moves arrive in order (server prints the count); `dns-sd -B _stagewand._tcp` shows the record. |
Only edit the files you own. Ship your check. No comments unless the why is non-obvious. Reply with the files changed and the check output.
Use Swift 6 strict concurrency without @unchecked Sendable. MainActor isolation is acceptable. Coordinate interfaces by reading existing files; do not wait for humans. Build checks may reveal peer-owned errors; report them without editing those files. For row 6, Link owns required 60 Hz accumulation; avoid double coalescing loss with trackpad. Row 7 owns PointerMode in SettingsView.swift. Session should expose a server property so Session.kick can rotate and invoke server.kick; App sets it.
