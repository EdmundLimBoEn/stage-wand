# PLAN.md — Stage Wand, 60-minute all-native build

Status: APPROVED by Edmund 2026-09-18. Execute as written.

**Hand this file to a coordinator chat (Fable 5.1 or Astra). It is self-contained.** Rationale, ADRs, and review history live in `docs/DESIGN.md`; you do not need them to build.

## What we are building
iPhone app + macOS menu-bar app, both native Swift, talking over LAN WebSocket found via Bonjour.
- **Pocket clicker:** with the iPhone locked, volume up = Right arrow, volume down = Left arrow, sent to the frontmost Mac app (Keynote). Haptic on each press. Works because background audio keeps the app process and its WebSocket alive through lock.
- **Trackpad mode (unlocked, ARM on):** 1-finger pan moves the real cursor, tap = left click, 2-finger tap = right click, 2-finger pan = scroll, 3-finger swipe left/right/up = Ctrl+←/→/↑ (switch spaces / Mission Control).
- **Keys:** ← → Esc buttons, L / R click buttons, big NEXT / PREV.
- **Pairing:** 4-digit code shown on the Mac; typed once into the phone's Settings, remembered. A valid code always displaces the current phone. Kick rotates the code.
- **Stretch only if Wave 2 is done by minute 45:** joystick mode, then gyro hold-to-aim mode.

## Coordinator rules
- You are the coordinator. You write the scaffold and the integration. You never write the hard files yourself; you dispatch, review diffs, run checks, commit.
- Dispatch Wave 1 as ONE burst of 8. If the harness throws overload errors, fall back to two bursts of 4 (rows 2,4,5,6 then 1,3,7,8). One owner per file; nobody edits another row's files. Bugs go back to the owner with the exact failing output pasted in.
- Astra = `codex exec -m gpt-6-astra`. Effort: `high` for the volume observer only, `medium` for the Mac server and trackpad, `low` for everything else. Fable subagents (model `fable`) take shell, UI, docs.
- Every agent prompt = the "Frozen contract" section below + that agent's row + this line: "Only edit the files you own. Ship your check. No comments unless the why is non-obvious. Reply with the files changed and the check output."
- Timebox is hard. At minute 45 stop adding and start rehearsing. Cut order if late: 3-finger chords → volume reset attempt (accept the at-limit dead zone) → volume clicker (NEXT/PREV buttons remain).

## Frozen contract (write in Wave 0, never change after minute 8)
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

## Wave 0 · 0–8 min · Coordinator
1. `git init`. Write `mac/Package.swift`, `mac/Makefile`, `ios/project.yml`, `Shared/Protocol.swift` (complete), the symlink, all stubs, placeholder `ContentView`, `silence.wav`.
2. `cd mac && swift build` green. `cd ios && xcodegen && xcodebuild -scheme StageWand -destination 'generic/platform=iOS Simulator' build | tail -3` green.
3. `mkdir -p prompts .out`, one prompt file per Wave 1 row. Commit `scaffold`. Dispatch all rows.
4. **Ask Edmund to open `ios/StageWand.xcodeproj` in Xcode now, plug in the iPhone, and Run the stub app once on the device** (creates the personal-team profile, triggers "Trust developer" and the Local Network prompt). This runs in parallel with Wave 1 so Wave 2 only reinstalls. Get the UDID: `xcrun devicectl list devices`.

## Wave 1 · 8–35 min · parallel, one agent per row

| # | Agent | Owns | Deliverable | Own check |
|---|---|---|---|---|
| 1 | Astra **high** | `ios/StageWand/Volume.swift`, `ios/StageWand/Haptics.swift` | `AVAudioSession` `.playback` + `[.mixWithOthers]`, active. Looping `AVAudioPlayer` on `silence.wav` at volume 0.01 and **actually playing** (`assert(player.isPlaying)`); restart on `AVAudioSession.interruptionNotification` and `routeChangeNotification`. KVO `outputVolume`: increase → `onUp`, decrease → `onDown`, debounce 150 ms; `suppressUntil: Date` window of 300 ms around every programmatic write so self-caused changes are ignored. At `start()`, set system volume to 0.5 once via a hidden `MPVolumeView` inside a 1×1 `UIWindow` that `Volume` creates itself; attempt the same reset after each press while the app is foreground, but treat the reset as **expected to fail** on modern iOS: correctness must not depend on it (at-limit dead zone is accepted). `Haptics.tick()` = `UIImpactFeedbackGenerator(.medium)`. `pressCount` increments per press. | `print("volume up/down #n")` lines; device test in Wave 2: lock, 10 ups + 10 downs, counts match; then repeat with volume started at max and at min and record what still registers. |
| 2 | Astra **medium** | `mac/Sources/Server.swift`, `scripts/ws-smoke.ts` | `NWListener(using: params)` where params = TCP + `NWProtocolWebSocket.Options(autoReplyPing: true)`; try 8787, then 8788-8790; `listener.service = NWListener.Service(name: Host.current().localizedName ?? "Mac", type: "_stagewand._tcp")`; receive text frames, decode `Command`; auth-within-2-s, displace-on-valid-code, ping 3 s / drop 6 s; `Task { @MainActor in session.peer = …; onCommand(cmd) }`; `kick()` sends `bye kicked` and closes. Encode `Reply` for outbound. | `swift run StageWandMac --serve` in one terminal, `bun scripts/ws-smoke.ts` (Bun's `WebSocket`, code "0000") prints PASS: no auth → closed ≈2 s; bad code → `bye badauth`; good → `status`; second good → first gets `bye displaced`; 200 moves arrive in order (server prints the count); `dns-sd -B _stagewand._tcp` shows the record. |
| 3 | Astra **medium** | `ios/StageWand/Pointer/TrackpadView.swift` | `struct TrackpadView: UIViewRepresentable { let armed: Bool; let sensitivity: Double; let onCommand: (Command) -> Void }` wrapping a `UIView` with `UIPanGestureRecognizer` (min/max 1 touch and a second one with 2), `UITapGestureRecognizer` (1 touch, and 2 touches), three `UISwipeGestureRecognizer`s (3 touches, L/R/U). 1-finger pan → accumulate `dPts * 2.0 * sensitivity`, flush `.move` on a 60 Hz `CADisplayLink`/timer; 1-tap → `.click(.left)`; 2-tap → `.click(.right)`; 2-finger pan → accumulate and flush `.scroll` (natural direction); 3-swipe → `.chord`. Emits nothing when `armed == false`. | `#Preview` with a sink appending JSON lines to an on-screen log; in Simulator (Option for two fingers), each gesture prints the right command and moves flush at ≈60 Hz. |
| 4 | Astra **low** | `mac/Sources/Input.swift` | `apply(.move)`: `CGEvent(source:nil)!.location` (CG space, top-left origin, never `NSEvent.mouseLocation`), add delta, clamp to the union of `CGDisplayBounds(id)` for every id from `CGGetActiveDisplayList` (already CG space, no conversion), post `.mouseMoved` at the absolute point with `mouseEventDeltaX/Y` set, tap `.cghidEventTap`. `.click`: down+up pair. `.scroll`: `CGEvent(scrollWheelEvent2Source:units:.pixel, wheelCount:2, wheel1: Int32(dy), wheel2: Int32(dx), wheel3: 0)`. `.key`: keyDown+keyUp. `.chord`: same with `flags = .maskControl`. `accessibilityGranted(prompt:)` → `AXIsProcessTrustedWithOptions`. `selfTest()`: 100 px right and back, left click, Esc, scroll 3 lines, print before/after coordinates, `exit(1)` if AX missing. | `swift run StageWandMac --selftest` passes (Entry.swift already routes the flag). |
| 5 | Fable | `mac/Sources/App.swift`, `mac/Sources/Session.swift` | `struct StageWandApp: App` (no `@main`); on launch `NSApp.setActivationPolicy(.accessory)`, start `Server`, `Input.accessibilityGranted(prompt: true)`. `MenuBarExtra("Stage Wand", systemImage: "wand.and.rays")` popover: pairing code large, `LAN IP:port` (via `getifaddrs`, en0 first), Accessibility red/green re-checked every 2 s, connected phone name or "waiting", Kick (rotates code, `server.kick()`), Quit. `rotateCode()` = 4 random digits. | `swift run StageWandMac` shows the icon within 2 s; `NSApp.activationPolicy() == .accessory` printed at launch; Kick changes the code; with the popover closed and Keynote in show mode, a Mac keyboard arrow still advances Keynote. |
| 6 | Fable | `ios/StageWand/Link.swift`, `ios/StageWand/Discovery.swift` | `Discovery`: `NWBrowser(for: .bonjour(type: "_stagewand._tcp", domain: nil))`; publish results as `(name, endpoint)`; pick the first, expose `macName`; `manualHost` overrides. Map browser `.failed`/permission denial to `localNetworkDenied`. `Link`: connect with `URLSession.shared.webSocketTask(with: endpoint URL ws://host:port/)` (connecting through the resolved IP is fine; IP literals are ATS-exempt); if `pairingCode` is empty → state `enterCode` and do not connect; send `auth` first; JSON-encode `Command`; buffer ≤ 8 key/click/chord while not authed and flush on `authed`; receive loop decodes `Reply`; reconnect every 1 s while `disconnected`. | Simulator + `swift run StageWandMac`: `searching → connecting → authed`; empty code → `enterCode`; kill the Mac app → `disconnected`; relaunch → `authed` within 3 s; a `key` sent while disconnected arrives after reconnect. |
| 7 | Fable | `ios/StageWand/StageWandApp.swift`, `ContentView.swift`, `SettingsView.swift` | `ContentView`: connection pill from `Link.state` + `macName` (with an "Enter code" and a "Local network denied, fix in Settings" state); ARM toggle (not persisted; off on `scenePhase == .background`); `TrackpadView(armed:sensitivity:onCommand:)` fills the middle; key row ← → ESC; L / R click; big NEXT / PREV (→ `.key(.right)` / `.key(.left)`); small debug label showing `volume.pressCount`. Wires `Volume(onUp: { link.send(.key(.right)) }, onDown: { link.send(.key(.left)) })` **outside** the ARM gate and `Haptics.tick()` on every send. `SettingsView`: sensitivity slider, pairing code field, manual host field, pointer mode picker (one option today), using exactly the frozen `@AppStorage` keys. | Builds; Simulator: every button sends the right command (Mac server prints it); ARM off blocks the trackpad but not buttons; entering the code moves the pill to `authed`. |
| 8 | Fable | `README.md`, `HUMANS.md`, `scripts/rehearse.md` | README: what it is; run (`make app && open mac/StageWandMac.app`, Xcode Run for the phone); manual steps (Accessibility + re-grant after rebuild, firewall Allow, Trust developer, Local Network permission, type the pairing code, same Wi-Fi or hotspot, Mission Control shortcuts on, phone volume not at a limit); troubleshooting (no Bonjour → manual host; volume presses not registering → check volume not at max/min and the app is running; phone locked → it reconnects). HUMANS.md checklist. `rehearse.md`: cold start both → type code → pocket presses locked → unlock + ARM → trackpad, scroll, 3-swipe → lock/unlock → Kick → hotspot fallback. | Coordinator can run everything from README alone. |

Astra dispatch (background, one call per row):
```bash
codex exec "$(cat prompts/row-1.md)" -C "$PWD" --full-auto -m gpt-6-astra -c 'model_reasoning_effort="high"' > .out/row-1.log 2>&1 &
# rows 2,3: medium · row 4: low
```
Fable dispatch: `Agent(subagent_type: "general-purpose", model: "fable", run_in_background: true, prompt: <contract + row + closing line>)`.

## Wave 2 · 35–52 min · Coordinator integrates, Simulator first, device second
1. Mac: merge rows 2, 4, 5. `cd mac && make app && open StageWandMac.app`; click Allow (firewall); grant Accessibility. `bun scripts/ws-smoke.ts` against `--serve` green. **Commit.**
2. Phone in Simulator: merge rows 3, 6, 7. Type the code; Simulator finds the Mac; ARM; trackpad moves the real cursor; tap clicks; 2-finger scroll in Safari; 3-finger swipe switches spaces; ← → Esc land in Keynote. **First complete demo. Commit.**
3. Device: merge row 1. `cd ios && xcodegen && xcodebuild -scheme StageWand -destination "id=$UDID" -allowProvisioningUpdates -derivedDataPath build build | tail -3 && xcrun devicectl device install app --device "$UDID" build/Build/Products/Debug-iphoneos/StageWand.app && xcrun devicectl device process launch --device "$UDID" systems.edmundlim.stagewand`. **Pass/fail of the pivot:** lock the phone, press volume up/down: Keynote advances with a haptic while locked. Then unlock: the same socket is still `authed` (background audio kept it alive) or it reconnects and displaces within 3 s. **Commit.**
4. Anything red → paste the failing output to the owning row's agent (Astra: fresh `codex exec` on the same files, "patch, don't rewrite"; Fable: `SendMessage`). Coordinator hand-fixes only one-line typos.

## Wave 3 · 52–60 min · Coordinator
Stop rebuilding the Mac app (each rebuild re-prompts AX and firewall). Run `scripts/rehearse.md`. `gh repo create EdmundLimBoEn/stage-wand --public --source . --push`. Tick HUMANS.md. Report DONE with the rehearsal results, or DONE_WITH_CONCERNS naming what was cut.

**Stretch (only if Wave 2 finished by minute 45):** row 9 Astra medium `Pointer/JoystickView.swift` (offset → velocity ≤ 1200 px/s, cubic curve, 60 Hz) then row 10 Astra medium `Motion.swift` + `Pointer/GyroView.swift` (`CMMotionManager` 60 Hz, `rotationRate.y → dx`, `rotationRate.x → dy`, dead zone 0.035 rad/s, EMA 0.6, clamp 60 px, only while HOLD TO AIM is held). Each adds its `PointerMode` case and a Settings picker entry.

## Human steps (Edmund; also in HUMANS.md)
- [ ] During Wave 1: open `ios/StageWand.xcodeproj` in Xcode, plug in the iPhone 13 Pro Max, Run the stub app once on the device; tap Trust for the developer profile; tap Allow on the Local Network prompt.
- [ ] Grant Accessibility to `StageWandMac.app` when prompted; expect re-prompts after rebuilds.
- [ ] Click Allow on the macOS firewall prompt at first bind.
- [ ] Type the 4-digit pairing code from the Mac menu bar into the phone's Settings once.
- [ ] Same Wi-Fi for both devices, or iPhone hotspot joined from the Mac.
- [ ] Mission Control shortcuts (Ctrl+←/→/↑) enabled in System Settings → Keyboard → Shortcuts.
- [ ] Keynote deck open for every test; phone volume not at max or min before the pocket test.

## Done means
Locked phone: volume up/down drives Keynote first press with a haptic. Unlocked + ARM: cursor, left/right click, scroll, 3-finger space switch, ← → Esc. Lock/unlock keeps or recovers control within 3 s. Kick rotates the code. Rehearsed once end to end. Pushed to GitHub.
