# Stage Wand rehearsal

Use a physical iPhone and the final Mac app build. Complete the [README setup](../README.md) first. Have a Keynote deck with enough slides, a scrollable page, and two Mac desktops ready. Input goes to the frontmost Mac app.

1. **Cold start both.** Quit and reopen StageWandMac and the phone app. Verify the pairing window opens; check its port and Accessibility state; allow firewall and Local Network prompts. Connect both devices to the same Wi-Fi. Keep volume mode locked and phone media volume near the middle.
2. **Type the code.** Trace the guided square to unlock touch controls, then enter the current four-digit code in phone Settings. Expect the phone to show an authenticated connection and the Mac to show a peer. Start the Keynote slideshow and confirm on-screen NEXT/PREV advances and rewinds once per press while touch controls are unlocked.
3. **Pocket presses, locked.** Lock the phone and put it in your pocket. Press volume up ten times, then volume down ten times, at a deliberate pace. Expect ten next-slide and ten previous-slide actions; record observed counts and haptic behavior. If the volume reaches a limit, unlock and recenter it, then repeat. Separately try starting at maximum and minimum and record which direction still registers. Do not mark locked operation passed based on unlocked results.
4. **Verify the touch lock.** Unlock the phone and return to Stage Wand. Every touch control must remain locked. Taps, partial squares, diagonal swipes, and rapid scribbles must not unlock it or send commands. Trace the complete guided square deliberately to unlock; confirm Lock for pocket relocks everything. Unlock again and enable ARM for the next checks.
5. **Trackpad, scroll, three-finger swipe.** Move the cursor with one finger, tap for left click, and use a two-finger tap for right click. Focus a scrollable page and drag with two fingers. Swipe with three fingers left/right to switch desktops and up for Mission Control. Return to Keynote and verify Left, Right, and ESC. Record any missing gesture separately.
6. **Lock/unlock recovery.** Lock for at least ten seconds, try another next/previous volume pair, then unlock. Expect the connection to remain authenticated or recover; target recovery within three seconds after returning to the app. Record the actual delay and any queued actions replayed. Trace the square again and verify movement.
7. **Kick.** Click Kick in the Mac popover. Expect the peer to disconnect and the displayed pairing code to change. Confirm the old code cannot regain control. Enter the new code on the phone and verify one NEXT/PREV pair after authentication.
8. **Hotspot fallback.** Enable Personal Hotspot on the iPhone and join it from the Mac. Clear any stale Manual Host and retry discovery. If needed, enter the Mac's new IP plus its popover port in Manual Host (for example `172.20.10.2:8787`; use the actual address). Authenticate and repeat a locked volume pair and an unlocked pointer gesture.

If locked volume control fails, record the failure and rehearse with the phone unlocked using NEXT/PREV. If a gesture fails, record that limitation before presenting. Re-grant Accessibility and rerun the input checks after any Mac rebuild.

## Results

Fill in after testing; these are not preverified results.

| Check | Observed result / pass or fail |
| --- | --- |
| Device models, OS versions, build tested | |
| Cold start and pairing; selected port | |
| Locked volume: next /10, previous /10; haptics | |
| Maximum/minimum volume behavior | |
| Volume mode blocks all touch controls; square unlock and automatic relock | |
| Pointer, left/right click, scroll | |
| Three-finger left/right/up; keyboard buttons | |
| Lock/unlock recovery time; queued actions | |
| Kick, code rotation, new-code pairing | |
| Hotspot discovery / Manual Host fallback | |
| Remaining limitations and presentation fallback | |

Transfer completed human checks to [HUMANS.md](../HUMANS.md).

## Build-day automated evidence (2026-09-18)

- macOS debug and release builds passed; app bundle code signature verified.
- Full iOS Simulator and signed iPhone 17 Pro builds passed (Swift 6). Device installation and launch succeeded; the app process was verified running.
- Protocol round trips and nonfinite/unknown-frame rejection passed.
- Real server: auth deadline ~2.1 s, bad auth, displacement, 200 ordered moves, invalid-delta rejection, ping/pong, missing-pong close ~6.4 s, fallback ports, and Bonjour passed.
- Phone Link against real server: eight buffered keys arrived; 900/-850 movement split into 400/-400, 400/-400, 100/-50; scroll arrived; bad and empty codes entered code state.
- Transport fixture: automatic reconnect within 3 s, Bonjour resolution/auth, and delta accumulation passed.
- Mac input self-test moved 100 px right and returned to the original point; click, Esc, and scroll posted.
- Physical locked-screen volume, haptics, gesture interaction, and full Keynote rehearsal remain unverified. Simulator installation succeeded, but runtime launch stalled in this environment.
- Team updated from the plan to DUU8J39BA7 to preserve Edmund’s selection in Xcode. Build products under synced Documents acquired Finder metadata; building the phone in `/tmp/stagewand-device-build` resolved signing.

## Touch-lock update

Mac pairing window now opens on launch/reopen. Square-unlock recognizer tests pass for valid traces and rejection of taps, partial, diagonal, fast, wrong-start, stale/reset, and scribbled traces. Signed device build passed. Physical touch-lock and pairing-window visibility still need the user’s confirmation.

## Cloudflare and signing update

- Authenticated `cf` CLI created named tunnel `stage-wand` at `stagewand.edmundlim.systems`; tunnel status healthy.
- Native URLSession secure WebSocket reached the real Mac authentication gate over Cloudflare; wrong code received `bye badauth`.
- Protected HTTP path returned 200; unknown path returned 404. Gate tests passed transparent auth/reply, ping/pong ordering, 32-connection cap, and cleanup.
- Secure URL parser and QR deep-link checks passed; signed device app installed/launched with the tunnel URL scheme.
- Mac app now signs with Apple Development certificate and installs at `~/Applications/StageWandMac.app`. Verified designated requirement uses bundle identifier and certificate, rather than an ad-hoc binary hash. Replace the old Accessibility entry once.
- End-to-end physical phone pairing, Keynote, and locked haptics remain user checks.

## Nearby latency update

- Direct transport opts into Apple peer-to-peer networking, resolves Bonjour through a scoped TCP path, then uses a native WebSocket URL on the resolved interface. Manual tunnel URLs retain TLS URLSession transport.
- Production native socket fixture passed resolution, authentication, send, >7-second ping/pong, cancellation, and bad-code rejection. Full Swift 6 iOS typecheck passed.
- Queue tests coalesced 1,000 moves with exact summed distance, legal <=400 frame splitting, and preserved key/click ordering. Pointer/scroll updates no longer trigger haptics.
- Mac listener now uses dual-stack binding, peer-to-peer opt-in, and TCP no-delay; full server smoke suite passed. Tunnel gate also disables Nagle buffering.
- Development certificate designated requirement was identical before and after a release rebuild.
- Physical nearby discovery and latency improvement remain unverified. CLI Bonjour browsing found no results, potentially due to local-network permission; the actual signed phone and Mac must be tested.

## Choppy movement follow-up

The user reports stop-start cursor movement while the Mac name is displayed, which points to the nearby path rather than the Cloudflare hostname. Physical responsiveness has not passed acceptance. The phone now uses one display-linked batching stage, keeps that display link running through a drag, and restores light motion haptics paced independently at most once per 80 ms. Its connection label distinguishes Nearby, Direct, and Secure link; Nearby does not prove a particular radio/interface. Manual ws addresses use the native no-delay socket.

USB fallback is a test via Personal Hotspot networking, not an implemented raw USB protocol. The Mac currently detects an inactive iPhone USB interface. Use its actual assigned Mac address after activation, not the Wi-Fi address displayed by default.

Mac cursor delivery now caches display bounds until display configuration changes and executes commands in the server's existing main-actor turn. Pending posted cursor locations preserve cumulative movement until the system catches up, reconcile physical-mouse changes, and expire after 250 ms idle. Focused checks cover burst accumulation, intermediate catch-up, physical handoff, timeout, clipping, and fractional deltas. Production Link delivery checks passed exact summed movement and click/scroll order. Signed phone build and Mac release packaging passed; both apps were updated. These checks do not measure physical end-to-end latency.

## Bluetooth direct transport

Root cause of the stop-start cursor in nearby mode: the Bonjour route with peer-to-peer opt-in rides Apple Wireless Direct Link when the devices have no shared network. AWDL time-slices the Wi‑Fi radio between the infrastructure channel and its own social channels and scales its availability windows with traffic, so a 60 Hz trickle of tiny frames arrives in bursts. No app-level batching fixes that.

Added a Bluetooth LE data path with the same JSON protocol. The host advertises a GATT service (`Shared/Bluetooth.swift` UUIDs) with a write-without-response command characteristic and a notify reply characteristic; the phone is the central, subscribes to replies, then authenticates with the four-digit code. That path exists on macOS (CoreBluetooth), Linux (BlueZ D-Bus), and Windows (WinRT `GattServiceProvider`). iOS and Android remotes both scan for the service UUID. Link's send loop awaits BLE readiness and write credits, so `CommandQueue` coalesces moves instead of queueing them. Settings gained a Connection picker (Bluetooth direct default, Wi‑Fi nearby); the status pill shows the route. Kick drops Bluetooth auth; the phone re-auths with the old code, receives `badauth` or a disconnect, and asks for the new code. Both ends need the Bluetooth permission on first launch.

Not verified here: physical BLE connection interval and perceived smoothness on the actual phone and Mac. Fall back to Wi‑Fi nearby on a shared network or the Personal Hotspot if Bluetooth direct will not pair.
