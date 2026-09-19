# HUMANS.md

Physical setup and acceptance checks for Stage Wand. Start with [README.md](README.md); leave checks unticked until verified on the actual devices.

- [x] Approve `docs/DESIGN.md` and `PLAN.md` before Wave 0 starts
- [x] Generate the iOS project, use the selected team `DUU8J39BA7`, build, install, and launch on the connected iPhone 17 Pro via Xcode tools.
- [ ] Allow Local Network or developer-trust prompts if shown.
- [ ] Allow Local Network access on the phone.
- [x] Build, sign, verify, and launch `StageWandMac.app`.
- [ ] Replace the old ad-hoc Accessibility entry with the development-signed `~/Applications/StageWandMac.app` and enable it.
- [ ] Click Allow on the macOS firewall incoming-connections prompt.
- [ ] Put both devices on the same Wi-Fi, or enable the iPhone hotspot and join it from the Mac.
- [ ] Type the current four-digit Mac pairing code into phone Settings and verify authentication.
- [ ] Enable Mission Control shortcuts (Control+Left/Right/Up) and create a second desktop.
- [ ] Start a Keynote slideshow and set phone media volume away from maximum/minimum.
- [ ] Run the locked pocket test on the physical phone: volume up advances and volume down goes back in locked volume mode. Record counts and any missing haptics.
- [ ] Verify square-unlocked touch controls: pointer, left/right click, scrolling, three-finger swipes, and keyboard buttons.
- [ ] Verify lock/unlock keeps or recovers the connection; draw the square again for touch control.
- [ ] Verify Kick disconnects the phone and rotates the code; pair again with the new code.
- [ ] Verify hotspot fallback, including Manual Host with the current Mac IP and popover port if discovery fails.
- [ ] Complete [scripts/rehearse.md](scripts/rehearse.md), record failures or fallbacks, and avoid rebuilding before the presentation.

- [x] Commit and push to https://github.com/EdmundLimBoEn/stage-wand.

- [ ] Arch Linux host: `sudo pacman -S --needed go git`, build `host/stagewand-host`, install `host/udev/70-stagewand-uinput.rules` and `host/modules-load.d/uinput.conf`, `sudo modprobe uinput`, log out of the graphical session and back in, run `./stagewand-host --diagnose`, then `./stagewand-host`. Confirm the pairing code, LAN address, and that Input shows uinput ready with wayland or x11.
- [ ] Arch Linux host: pair an iPhone or Android remote on Hyprland, Sway, or another Wayland session, then verify next/prev, pointer, click, scroll, and Ctrl+arrow chords in the frontmost app.
- [ ] Arch Linux host: repeat pointer and click on an X11 session (i3 or Xfce) if you use one.
- [ ] Debian or Ubuntu host: same uinput udev and modules-load files, then pair a remote and verify injection.
- [ ] Windows host: run `stagewand-host.exe`, allow the firewall, pair a remote, and verify `SendInput` next/prev, pointer, and Win+Ctrl+Left/Right plus Win+Tab.
- [ ] Android: install the debug APK, allow nearby devices or enter a manual `host:port`, draw the square, pair with the four-digit code, and confirm volume up/down while the app is in the foreground.
- [ ] Android: record whether volume still works after locking the screen or leaving the app. Treat failure as expected until proven on that OEM.
- [ ] Confirm Android does not offer Bluetooth direct, and that Wi-Fi, hotspot, manual host, or a Mac tunnel URL is the route you used.

- [ ] Confirm the Mac pairing window is visible on launch/reopen.
- [ ] Confirm taps, incomplete squares, diagonal swipes, and rapid scribbles do not unlock volume mode; a deliberate full square does.
- [ ] Confirm Lock for pocket and leaving the app relock all touch controls while hardware volume remains available.

- [ ] Scan the tunnel QR with iPhone Camera, open Stage Wand, unlock with the square, and enter the current Mac pairing code.
- [ ] Verify keys and locked volume presses over the tunnel; keep the tunnel runner and Mac app running.

- [ ] With Wi-Fi and Bluetooth enabled, choose Use nearby Mac (low latency), pair with the displayed code, and compare cursor response. If discovery fails, join the iPhone Personal Hotspot and retry direct mode.

- [ ] Relaunch `~/Applications/StageWandMac.app` and click Allow on the macOS Bluetooth prompt. Reinstall the phone app, open Settings, choose Bluetooth direct, and allow Bluetooth on the phone when asked.
- [ ] With Bluetooth direct selected, pair with the displayed code and confirm the status pill reads `Bluetooth · <Mac name>`. Move the cursor for 30 seconds: expect steady motion with no pause/burst. Record the result.
- [ ] If Bluetooth direct will not pair within 15 seconds, switch to Wi‑Fi nearby or join the iPhone Personal Hotspot from the Mac and retry; record which route you presented with.
- [ ] Retest sustained trackpad movement on Wi‑Fi nearby only if Bluetooth direct is unavailable; report any pause/burst behavior and the displayed route.
- [ ] For a USB comparison, enable iPhone Personal Hotspot, connect a data cable and trust the Mac. Confirm iPhone USB is active in Mac Network settings, then enter the Mac's USB-interface IP and Stage Wand port in Manual Host. Verify pairing, pointer movement, and locked volume/haptics separately.
