# Stage Wand

Stage Wand turns an iPhone into a pocket presentation clicker and a trackpad for your Mac. Volume up sends Right (next slide); volume down sends Left (previous slide) to the frontmost Mac app. Draw the guided square to unlock touch controls for moving the pointer, clicking, scrolling, and switching spaces.

Requires macOS 14+, an iPhone running iOS 17+, Xcode with iOS device support, and XcodeGen. Hardware volume and locked-screen behavior must be tested on a real phone.

## Build and run

Run these commands from the repository root:

```sh
make -C mac app && open mac/StageWandMac.app
```

This runs `make app` in `mac/`, builds the release executable, and wraps it in a menu bar app. A pairing window opens on launch with the code, connection status, and listening port. Reopen the app or use its menu bar item to show the window again. The default port is **8787**, with fallback ports **8788–8790**; always use the port shown there.

Generate and open the phone project:

```sh
(cd ios && xcodegen generate)
open ios/StageWand.xcodeproj
```

In Xcode, select the **StageWand** scheme and your connected iPhone, check automatic signing with team **DUU8J39BA7**, and choose **Product → Run**. Enable Developer Mode on the phone if Xcode requests it. If launch is blocked by an untrusted developer, open **Settings → General → VPN & Device Management** on the phone, trust the developer profile, then Run again.

## First connection

1. Put both devices on the same Wi-Fi. Alternatively, enable Personal Hotspot on the iPhone and join it from the Mac.
2. On the Mac, allow StageWandMac in **System Settings → Privacy & Security → Accessibility**. After rebuilding, re-grant access if input stops working: remove the old entry, add `mac/StageWandMac.app`, enable it, and relaunch.
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
| No Mac appears through Bonjour | Check Local Network permission and the shared network. In phone Settings, enter the Mac's Wi-Fi IP address in Manual Host, such as `192.168.1.20:8787`, substituting the actual IP and popover port. A host without a port uses 8787. Find the IP in the Mac's Wi-Fi network details. Discovery uses `_stagewand._tcp`. |
| Manual connection also fails | Check the popover port and firewall permission. Guest Wi-Fi may isolate clients; use the iPhone hotspot fallback. Update or clear Manual Host after changing networks. |
| Connected but the Mac ignores input | Re-grant Accessibility after a rebuild, draw the square to unlock touch controls, and put the intended Mac app frontmost. |
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

To exercise the phone transport against a running headless server, compile `Shared/Protocol.swift`, `ios/StageWand/Discovery.swift`, `ios/StageWand/Link.swift`, and `scripts/link-check.swift` together with `swiftc -swift-version 6`; run the result with `PORT` set to the test server port (default 8787).
