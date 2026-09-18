# Stage Wand

Stage Wand turns an iPhone into a pocket presentation clicker and a trackpad for your Mac. Volume up sends Right (next slide); volume down sends Left (previous slide) to the frontmost Mac app. Draw the guided square to unlock touch controls for moving the pointer, clicking, scrolling, and switching spaces.

Requires macOS 14+, an iPhone running iOS 17+, Xcode with iOS device support, and XcodeGen. Hardware volume and locked-screen behavior must be tested on a real phone.

## Build and run

Run these commands from the repository root:

```sh
make -C mac app && open ~/Applications/StageWandMac.app
```

This runs `make app` in `mac/`, builds the release executable, and signs it with your Apple Development identity, and installs it at `~/Applications/StageWandMac.app`. A pairing window opens on launch with the code, connection status, and listening port. Reopen the app or use its menu bar item to show the window again. The default port is **8787**, with fallback ports **8788–8790**; always use the port shown there.

Generate and open the phone project:

```sh
(cd ios && xcodegen generate)
open ios/StageWand.xcodeproj
```

In Xcode, select the **StageWand** scheme and your connected iPhone, check automatic signing with team **DUU8J39BA7**, and choose **Product → Run**. Enable Developer Mode on the phone if Xcode requests it. If launch is blocked by an untrusted developer, open **Settings → General → VPN & Device Management** on the phone, trust the developer profile, then Run again.

## First connection

1. Keep Bluetooth enabled on both devices. Allow Bluetooth for StageWandMac when macOS asks on first launch. In phone Settings, choose **Bluetooth direct** (the default) and allow Bluetooth when iOS asks. No Wi‑Fi is needed for this route. **Wi‑Fi nearby** uses your network or Apple peer-to-peer Wi‑Fi instead; if that fails on the venue network, enable Personal Hotspot on the iPhone and join it from the Mac.
2. On the Mac, allow StageWandMac in **System Settings → Privacy & Security → Accessibility**. If an old ad-hoc build was granted access, remove that old entry, add `~/Applications/StageWandMac.app`, enable it, and relaunch. The development-certificate signature now stays stable across rebuilds.
3. Choose **Allow** when the macOS firewall asks about incoming connections.
4. Allow **Local Network** access on the phone. If denied, enable Stage Wand in **Settings → Privacy & Security → Local Network** and reopen the app.
5. Draw the square on the phone to unlock touch controls, then open Settings and type the four-digit pairing code shown in the Mac pairing window, including any leading zeros. Wait for the authenticated/connected state before testing controls.
6. Enable Mission Control shortcuts **Control+Left**, **Control+Right**, and **Control+Up** in **System Settings → Keyboard → Keyboard Shortcuts → Mission Control**. Create a second desktop to test switching spaces.
7. Open a Keynote deck, start the slideshow, and leave it frontmost. Set the phone's media volume near the middle, away from maximum or minimum, before the pocket test. Keep Stage Wand running; do not force-quit it.

## Controls

| Input | Action |
| --- | --- |
| Volume up / down | Next / previous slide (Right / Left), including while locked |
| NEXT / PREV, arrow keys, ESC | Send the corresponding key to the frontmost Mac app |
| One-finger drag | Move pointer |
| Tap / two-finger tap | Left / right click |
| Two-finger drag | Scroll |
| Three-finger swipe left / right | Previous / next space |
| Three-finger swipe up | Mission Control |

**Volume mode locks every touch control, including slide buttons, pairing, and Settings. Hardware volume buttons keep working.** Trace the guided square deliberately to unlock touch controls. Use Lock for pocket to relock; leaving the app or locking the screen also relocks it. After the square unlock, enable ARM for the trackpad and click controls. Slide buttons and Settings are available immediately. Adjust sensitivity in phone Settings (0.5–3) after unlocking.

The phone plays a silent audio loop to support pocket operation. Volume recentering may change media volume and may be blocked by iOS; repeated presses at a volume limit may not register. Verify locked operation on your actual device before presenting. If it fails, use the on-screen NEXT/PREV buttons while unlocked.

## Troubleshooting

| Symptom | What to check |
| --- | --- |
| Bluetooth direct stays on Connecting | Check Bluetooth is on for both devices, that StageWandMac is allowed under **System Settings → Privacy & Security → Bluetooth**, and that Stage Wand is allowed under phone **Settings → Privacy & Security → Bluetooth**. Relaunch the Mac app after granting. Only one Stage Wand Mac should be advertising nearby. Fall back to Wi‑Fi nearby or the hotspot if it still fails. |
| No Mac appears through Bonjour | Check Local Network permission and the shared network. In phone Settings, enter the Mac's Wi-Fi IP address in Manual Host, such as `192.168.1.20:8787`, substituting the actual IP and popover port. A host without a port uses 8787. Find the IP in the Mac's Wi-Fi network details. Discovery uses `_stagewand._tcp`. |
| Manual connection also fails | Check the popover port and firewall permission. Guest Wi-Fi may isolate clients; use the iPhone hotspot fallback. Update or clear Manual Host after changing networks. |
| Connected but the Mac ignores input | Ensure the signed app in `~/Applications` is the enabled Accessibility entry, draw the square to unlock touch controls, and put the intended Mac app frontmost. |
| Volume presses do not register | Check that media volume is not at maximum/minimum and Stage Wand is running. Reopen it after an audio interruption. Test on the physical phone; use NEXT/PREV if necessary. |
| Phone was locked or connection dropped | Unlock and reopen Stage Wand; it reconnects. Wait for authentication, then draw the square again. Up to eight queued key/click/chord commands may arrive after reconnecting. |
| Pairing code rejected or Mac used Kick | Read the current code from the Mac popover and replace the saved phone code. Kick rotates the code and disconnects the peer. |
| Three-finger swipes do nothing | Unlock touch controls and enable the Mission Control shortcuts, and create another desktop for left/right switching. |
| Code signing reports resource forks or Finder information | Keep DerivedData outside synced Documents folders. Use Xcode’s default DerivedData or pass `-derivedDataPath /tmp/stagewand-device-build` to `xcodebuild`. |
| Phone app will no longer launch | Reinstall with Xcode if the development provisioning profile has expired. |

Only one phone controls the Mac at a time; another phone authenticating with the current code displaces the existing connection.

## Rehearse

Follow [the complete rehearsal](scripts/rehearse.md) and record real-device results before presenting. [HUMANS.md](HUMANS.md) tracks the permissions and physical checks that require a person. Do not rebuild between the final permission check and the presentation.

## Automated checks

```sh
swiftc Shared/Protocol.swift scripts/protocol-check.swift -o /tmp/stagewand-protocol-check
/tmp/stagewand-protocol-check
swiftc Shared/SquareUnlock.swift scripts/square-unlock-check.swift -o /tmp/stagewand-square-check
/tmp/stagewand-square-check
bun scripts/ws-smoke.ts --spawn
```

`--spawn` starts a headless server with test code `0000`, verifies delivery order and authentication, then stops it. Normal app launches generate a random pairing code.

To exercise the phone transport against a running headless server, compile `Shared/Protocol.swift`, `Shared/ConnectionURL.swift`, `Shared/CommandQueue.swift`, `ios/StageWand/Discovery.swift`, `ios/StageWand/LocalSocket.swift`, `ios/StageWand/Link.swift`, and `scripts/link-check.swift` together with `swiftc -swift-version 6`; run the result with `PORT` set to the test server port (default 8787).

## Cloudflare Tunnel for isolated Wi-Fi

When the network blocks phone-to-Mac traffic, run the Mac app and then:

```sh
brew install cloudflared
bun scripts/tunnel.ts
```

Keep the tunnel process running. The Mac pairing window updates with a QR code. Scan it using the iPhone Camera and open Stage Wand; draw the square to unlock Settings and enter the Mac’s current four-digit code. The phone and Mac only need Internet access, not direct LAN connectivity. You can also use **Copy tunnel URL** on the Mac and paste it into the phone’s **Mac address or tunnel URL** field.

This Mac uses the named tunnel **stage-wand** at **stagewand.edmundlim.systems**, created using the authenticated `cf` CLI. Its token and hostname live in `~/.config/stage-wand/tunnel-token` and `named-host`; the secret URL path persists in `tunnel-path`, so restarts keep the same phone address. Without a named-tunnel token, the runner falls back to a temporary Quick Tunnel. See [Cloudflare’s setup documentation](https://developers.cloudflare.com/tunnel/get-started/).

The tunnel URL contains a random 256-bit secret path, checked before traffic reaches the Mac WebSocket server. The four-digit pairing code remains required. Treat the QR and full URL as private; neither is committed. The local URL file is `~/.config/stage-wand/tunnel-url`. Stop the runner with Ctrl+C to remove the URL and close the tunnel. If the Mac chooses a fallback port, start the runner with `STAGEWAND_PORT=8788 bun scripts/tunnel.ts` (substitute the port shown).

## Cursor latency

Use **Settings → Bluetooth direct** for trackpad control. The phone connects to the Mac over Bluetooth LE as a GATT central; the Mac advertises a Stage Wand service and receives commands as write-without-response packets. This route does not depend on Wi‑Fi, a router, or the venue network, and it avoids Apple peer-to-peer Wi‑Fi (AWDL), which time-slices the radio and produces the stop-start cursor motion seen in Wi‑Fi nearby mode when no shared network is available. Expect roughly 30 ms update cadence, which is what the BLE connection interval allows between two Apple devices.

**Wi‑Fi nearby** remains available and is smoothest when both devices share the same Wi‑Fi or the iPhone Personal Hotspot. Cloudflare remains available when neither direct route works; its latency depends on the Internet route.

The app coalesces pending movement without losing distance, applies back-pressure on the Bluetooth link so bursts merge instead of queueing, uses TCP no-delay for Wi‑Fi connections, and gives light haptics for pointer motion. The Bluetooth characteristics are unencrypted; the four-digit pairing code remains the access control, as it is for the plain-WebSocket Wi‑Fi route.
