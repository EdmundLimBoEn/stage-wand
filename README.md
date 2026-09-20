# Stage Wand

Stage Wand is a pocket presentation clicker and a trackpad for the computer that is running the deck. Volume up sends Right (next slide). Volume down sends Left (previous slide). Draw the guided square to unlock touch controls for moving the pointer, clicking, scrolling, and switching spaces.

The original pair is a native **iPhone** remote and a **macOS** menu-bar host. This repository also has a **Linux** host, a **Windows** host, and an **Android** remote. Remotes and hosts speak the same JSON command protocol (`Shared/Protocol.swift`) over LAN WebSocket (`_stagewand._tcp` on port **8787**, fallback **8788–8790**) and over Bluetooth LE GATT (`Shared/Bluetooth.swift`). The computer is the GATT peripheral. The phone is the GATT central.

The Apple pair needs macOS 14+, an iPhone running iOS 17+, Xcode with iOS device support, and XcodeGen. Hardware volume and locked-screen behavior must be tested on a real phone. Linux, Windows, and Android install notes are below.

## Branches and worktrees

This is a one-person repo. The split is for isolation, not review ceremony.

| Branch | Meaning |
| --- | --- |
| `main` | Last thing you would present or ship. Always buildable. No direct commits. |
| `dev` | Integration line. The checkout you keep open in Cursor. Can be slightly ahead of `main`. |
| `feat/<slug>` | One task, one directory. Cut from `dev`. |
| `hotfix/<slug>` | Production fix. Cut from `main`, then merge into `dev` too. |

GitHub pull requests are optional. Use them when you want CI on a branch you are not ready to merge, or when promoting `dev` → `main`. Day to day, merge locally.

Worktrees exist so you (and agents) can run two tasks at once without `git switch` in the directory you already have open. This clone **stays on `dev`**. A locked `main` checkout lives at `../stage-wand-trees/main`. Git will refuse to check the same branch out twice; that is intended.

```sh
./scripts/worktree setup                 # once per clone: hooks, local `dev`, locked `main`
./scripts/worktree add my-change         # ../stage-wand-trees/feat-my-change, from `dev`
./scripts/worktree add hotfix/crash main
./scripts/worktree list
./scripts/worktree rm my-change          # after the branch is merged
```

Open the new directory for that task. Do not `git switch` this folder to the feature branch.

Land a finished task on `dev` (from this `dev` checkout):

```sh
git fetch origin
git merge --ff-only feat/my-change       # or: git merge --no-ff feat/my-change
git push origin dev
./scripts/worktree rm my-change
```

Promote to production when you would actually use the build (rehearsal, a tagged point, a machine you install from):

```sh
git fetch origin
git -C ../stage-wand-trees/main merge --ff-only origin/dev
git push origin main
```

If `main` cannot fast-forward, merge `dev` into `main` as an explicit merge commit (hooks allow that) and merge `main` back into `dev` so the lines do not drift.

Hotfix: `./scripts/worktree add hotfix/crash main`, ship it to `main`, merge the same branch into `dev`, then `./scripts/worktree rm hotfix/crash`.

After a fresh clone, run `./scripts/worktree setup` so `.githooks` is installed (`core.hooksPath`). Agents should follow [AGENTS.md](AGENTS.md).

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

## Linux host

The Linux companion injects mouse and key events through `/dev/uinput`, advertises `_stagewand._tcp` on port **8787** (fallback **8788-8790**), and advertises a Bluetooth LE GATT peripheral with the same service as StageWandMac. An iPhone or Android remote can drive it over Wi-Fi or Bluetooth.

The injector is in-process uinput. That works on Wayland and X11 because the kernel presents a virtual evdev device. Do not install xdotool, ydotool, or libei for Stage Wand. extra/xdotool talks to X11 only. extra/ydotool is a second process on the same uinput node, plus a daemon. extra/libei needs a desktop portal that Hyprland and Sway do not fully expose. No AUR package is required.

### Install from GitHub Releases

Each push or merge to `main` publishes Linux packages: a Debian/Ubuntu `.deb` and an Arch `.pkg.tar.zst`. Both include the tray app (`stagewand`), the CLI host (`stagewand-host`), the uinput udev rule, and the boot module config.

**Arch Linux**

```sh
curl -fL -O https://github.com/EdmundLimBoEn/stage-wand/releases/latest/download/stagewand-host-x86_64.pkg.tar.zst
sudo pacman -U ./stagewand-host-x86_64.pkg.tar.zst
```

**Debian or Ubuntu**

```sh
curl -fL -O https://github.com/EdmundLimBoEn/stage-wand/releases/latest/download/stagewand-host_amd64.deb
sudo apt install ./stagewand-host_amd64.deb
```

Launch **Stage Wand** from the application menu, or run `stagewand`. Log out of the graphical session and log in again if `/dev/uinput` is not yet writable for your seat.

### Install on Arch Linux

For the system tray app with a pairing window, build and install the Arch package from this checkout:

```sh
sudo pacman -S --needed go git gcc base-devel cmake qt6-base
cd host/arch
makepkg -si
```

Launch **Stage Wand** from the application menu, or run `stagewand`. It shows the pairing code, phone connection, input readiness, Bluetooth status, and LAN address. **Copy code** copies the current code; **Disconnect & new code** disconnects the phone and rotates it. Closing the window keeps the host in the system tray. Click its icon or launch Stage Wand again to reopen it. **Quit Stage Wand** stops the host and Bluetooth advertisement. Stop any terminal-launched host before opening the tray app.

The tray app uses Qt 6's [system tray support](https://doc.qt.io/qt-6/qsystemtrayicon.html). On desktops without a tray, the pairing window stays available and closing it quits. The CLI remains available as `stagewand-host`.

To build the desktop app without installing a package, run `make -C host desktop`, then `host/desktop/build/stagewand`. Run its lifecycle tests with `ctest --test-dir host/desktop/build --output-on-failure`.

For just the command-line host:

Build dependencies are extra/go, extra/git, and (for the PKGBUILD) core/gcc. From the repository root:

```sh
sudo pacman -S --needed go git
sudo pacman -S --needed bluez bluez-utils
sudo systemctl enable --now bluetooth
cd host
go build -o stagewand-host ./cmd/stagewand-host
sudo cp udev/70-stagewand-uinput.rules /etc/udev/rules.d/
sudo cp modules-load.d/uinput.conf /etc/modules-load.d/stagewand-uinput.conf
sudo modprobe uinput
sudo udevadm control --reload
sudo udevadm trigger --action=add --subsystem-match=misc
```

Log out of the graphical session and log in again so systemd-logind applies the `uaccess` ACL on `/dev/uinput`. Do not add your user to the `input` group.

Then run:

```sh
./stagewand-host
```

The process prints the four-digit pairing code, the LAN address, the session type, whether uinput is writable, and whether Bluetooth is advertising. Type `k` and Enter to kick the peer and rotate the code. Ctrl+C quits.

If Bluetooth is off, `bluetoothd` is not running, or D-Bus registration fails, the host still serves the LAN WebSocket. The console line `Bluetooth:` names the error. LAN pairing is unchanged.

If `/dev/uinput` is missing or not writable, the host still accepts connections and logs commands. Pointer injection stays off until the module is loaded and the seated user can open `/dev/uinput`. Run `./stagewand-host --diagnose` to print the probe.

Optional package from this checkout (needs core/gcc and the `base-devel` group for `makepkg`):

```sh
sudo pacman -S --needed go git gcc base-devel cmake qt6-base
cd host/arch
makepkg -si
```

That installs `/usr/bin/stagewand`, its application launcher and icon, `/usr/bin/stagewand-host`, the udev rule under `/usr/lib/udev/rules.d/`, and a modules-load file so `uinput` loads at boot.

### Build on Debian or Ubuntu without a package

```sh
sudo apt install golang-go git bluez
sudo systemctl enable --now bluetooth
cd host
go build -o stagewand-host ./cmd/stagewand-host
sudo cp udev/70-stagewand-uinput.rules /etc/udev/rules.d/
sudo cp modules-load.d/uinput.conf /etc/modules-load.d/stagewand-uinput.conf
sudo modprobe uinput
sudo udevadm control --reload
sudo udevadm trigger --action=add --subsystem-match=misc
```

Log out of the graphical session and log in again, then run `./stagewand-host`.

To build the Debian/Ubuntu package locally (same layout as the GitHub Release):

```sh
sudo apt install golang-go cmake g++ qt6-base-dev qt6-base-dev-tools libgl1-mesa-dev dpkg-dev
make -C host deb
sudo apt install ./host/debian/stagewand-host_amd64.deb
```

### Session types

| Session | uinput |
| --- | --- |
| Hyprland, Sway, niri, river, labwc, Wayfire | Works on Wayland |
| GNOME or KDE Plasma on Wayland | Works |
| i3, dwm, Xfce, Cinnamon, GNOME or KDE on X11 | Works |
| SSH or a tty with no seat | The `uaccess` ACL often does not apply. Run the host on the local graphical session. |

Run `./stagewand-host --selftest` after login to post a short move, click, Esc, and scroll. Use `--dry-run` to skip injection. Use `--serve` for the headless test server with code `0000`.

Chords send Ctrl+Left, Ctrl+Right, and Ctrl+Up. Bind those shortcuts in the compositor if you want space switching. Scroll uses relative wheel units, so distance will not match macOS pixel scrolling.

## Windows host

The same `host` module cross-compiles to Windows. It injects input with `SendInput` and advertises the Stage Wand GATT service with WinRT `GattServiceProvider`.

On a Windows machine with Go:

```bat
cd host
go build -o stagewand-host.exe .\cmd\stagewand-host
stagewand-host.exe
```

From Linux or macOS you can only compile, not run, the Windows binary:

```sh
cd host
GOOS=windows GOARCH=amd64 go build -o stagewand-host.exe ./cmd/stagewand-host
```

Allow the app through Windows Firewall when it binds. Turn Bluetooth on in Windows settings. The console shows the pairing code, listen address, and Bluetooth line. Type `k` and Enter to kick. `--serve`, `--dry-run`, and `--selftest` match the Linux flags. `--serve` does not start Bluetooth.

Chord mapping on Windows is a desktop analogue, not Mission Control:

| Protocol chord | Windows keys |
| --- | --- |
| `spaceLeft` | Win+Ctrl+Left (previous virtual desktop) |
| `spaceRight` | Win+Ctrl+Right (next virtual desktop) |
| `missionControl` | Win+Tab (Task View) |

Enable virtual desktops if you want the space chords. Pointer motion is integer pixels. Sub-pixel moves snap to one pixel.

## Android remote

The Android app discovers `_stagewand._tcp`, scans for the Stage Wand Bluetooth LE service, or you can type `host:port` / a tunnel URL. It authenticates with the four-digit code, sends volume up/down as next/prev, and has square unlock, ARM, trackpad, and the same on-screen keys as iOS. Bluetooth direct is the default in Settings, matching iOS.

Open the `android/` folder in Android Studio (JDK 21, Android SDK 35) and run the **app** configuration on a phone. From the command line:

```sh
cd android
./gradlew :protocol:test :app:assembleDebug
```

The debug APK is `android/app/build/outputs/apk/debug/app-debug.apk`. Install it with `adb install -r` that path, then allow Bluetooth, nearby devices (or location on Android 12 and older), and notifications if the system asks.

Pairing:

1. Run a host (Mac, Linux, or Windows) in the same room for Bluetooth, or on the same LAN for Wi-Fi.
2. Draw the square to unlock the Android UI.
3. Enter the host pairing code in Settings, including leading zeros.
4. Leave **Host address or tunnel URL** empty. Keep **Bluetooth** selected for GATT direct, or switch to **Wi-Fi** for `_stagewand._tcp`.
5. If Bluetooth stays on Connecting, confirm Bluetooth is on for both devices. If Wi-Fi discovery finds nothing, set **Host address or tunnel URL** to `192.168.1.20:8787` with the real IP and port.

Volume keys are consumed while Stage Wand is in the foreground. A media-playback foreground service tries to keep the process alive and watch music-stream volume changes after you leave the activity. That pocket path is OEM-dependent and untested on a physical phone in this change. Use NEXT/PREV if volume does not arrive.

Android has Bluetooth direct and Wi-Fi nearby. A manual `host:port` or Cloudflare tunnel URL overrides both.

## Wire protocol

`Shared/Protocol.swift` is the source of truth. `Shared/protocol.schema.json` and `Shared/protocol-fixtures.json` are the cross-language copies used by Go and Kotlin tests. Do not add a new `t` value in Swift without updating the fixtures. The fixture check greps `Protocol.swift` and fails if they drift.

First frame must be `auth` within 2 seconds on the WebSocket. A valid code displaces the current peer. The server sends WebSocket pings every 3 seconds and drops a peer after 6 seconds without a pong.

Bluetooth LE uses the same JSON objects as one ATT write-without-response (command) and one notification (reply). UUIDs live in `Shared/Bluetooth.swift` and are copied into `host/internal/ble/uuids.go` and `android/protocol/.../Bluetooth.kt`. There is no extra BLE header. The default 23-byte ATT MTU cannot hold `{"t":"auth","code":"0000"}`, so the phone exchanges a larger MTU before it sends auth. iOS does that in CoreBluetooth. Android calls `requestMtu(517)`. The host accepts the central's MTU. Kick still rotates the code. BlueZ cannot always address a notification to one central, so a failed auth from a second phone disconnects that device instead of broadcasting `badauth` to the authed phone.

## What works here vs what needs a device

Verified in this repository's CI environment:

- Go protocol parser against the golden fixtures, including BLE UUID lockstep with `Shared/Bluetooth.swift`
- Linux host WebSocket behavior (`bun scripts/ws-smoke.ts --spawn-host`): auth window, `badauth`, displace, 200 ordered moves, ping/pong
- Linux host BLE session policy without a radio (`go test ./internal/ble`): auth, `badauth`, displace, kick, move range, and serialized reconnect cleanup
- Shared Bluetooth/LAN control ownership and handover, including race-detector checks and stale-owner rejection
- Windows host tests on a Windows runner, including the native `INPUT` structure layout, plus `GOOS=windows` compilation of the WinRT GATT adapter
- Kotlin protocol, square-unlock, command-queue, connection-URL, tap/drag gesture, and Bluetooth backpressure tests
- `./gradlew :app:assembleDebug`
- Arch Linux `archlinux:latest` with `pacman -S --needed go git gcc base-devel`, `bash host/arch/check.sh`, and `makepkg -f` for `stagewand-host`
- Ubuntu `dpkg-deb` package for `stagewand-host` (tray app plus CLI), installed with `apt`

Not verified on hardware in this change:

- `/dev/uinput` injection on an Arch desktop (needs the udev rule, a loaded `uinput` module, and a graphical seat)
- `SendInput` on a Windows desktop, including firewall and virtual-desktop chords
- Android NSD discovery, volume keys, lock-screen/pocket volume, haptics, and multi-touch trackpad on a phone
- An iPhone talking to the Linux or Windows host, or Android talking to StageWandMac
- Bluetooth LE on a physical radio: BlueZ advertising, WinRT `GattServiceProvider`, Android `BluetoothGatt`, ATT MTU exchange, and pointer smoothness

## First connection

1. Keep Bluetooth enabled on both devices. Allow Bluetooth for the host when the OS asks on first launch. In phone Settings, choose **Bluetooth direct** (the default) and allow Bluetooth when the phone asks. No Wi‑Fi is needed for this route. **Wi‑Fi nearby** uses your network or, on Apple, peer-to-peer Wi‑Fi instead; if that fails on the venue network, enable Personal Hotspot on the iPhone and join it from the computer.
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
| Bluetooth direct stays on Connecting | Check Bluetooth is on for both devices. On a Mac, allow StageWandMac under **System Settings → Privacy & Security → Bluetooth**. On Linux, run `bluetoothctl show` and confirm the adapter is powered; the host talks to BlueZ over D-Bus. On Windows, confirm Bluetooth is on in Settings. On the phone, allow Bluetooth for Stage Wand. Relaunch the host after granting. Only one Stage Wand host should be advertising nearby. Fall back to Wi‑Fi nearby or the hotspot if it still fails. |
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
bun scripts/protocol-fixtures-check.ts
bun scripts/ws-smoke.ts --spawn
bun scripts/ws-smoke.ts --spawn-host
(cd host && go test ./... && go build -o stagewand-host ./cmd/stagewand-host && GOOS=windows GOARCH=amd64 go build -o stagewand-host.exe ./cmd/stagewand-host)
bash host/arch/check.sh
(cd android && ./gradlew :protocol:test :app:assembleDebug)
```

`--spawn` starts the macOS headless server with test code `0000`. `--spawn-host` does the same with the Go host. Both verify delivery order and authentication, then stop. Normal app launches generate a random pairing code.

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

Use **Settings → Bluetooth direct** for trackpad control. The phone connects as a GATT central. The host advertises service `5A3E0001-8B6C-4B1E-9F8D-2C7A1D4E6F01`, receives commands on the write-without-response characteristic, and sends `status` / `bye` as notifications. This route does not depend on Wi‑Fi, a router, or the venue network. On Apple it also avoids peer-to-peer Wi‑Fi (AWDL), which time-slices the radio and produces the stop-start cursor motion seen in Wi‑Fi nearby mode when no shared network is available. Expect roughly 30 ms update cadence on Apple radios. Linux BlueZ and Windows WinRT use the same JSON and UUIDs. The connection interval depends on those stacks.

**Wi‑Fi nearby** remains available and is smoothest when both devices share the same Wi‑Fi or the iPhone Personal Hotspot. Cloudflare remains available when neither direct route works; its latency depends on the Internet route.

The app coalesces pending movement without losing distance, applies back-pressure on the Bluetooth link so bursts merge instead of queueing, uses TCP no-delay for Wi‑Fi connections, and gives light haptics for pointer motion. The Bluetooth characteristics are unencrypted; the four-digit pairing code remains the access control, as it is for the plain-WebSocket Wi‑Fi route.
