# HUMANS.md

Physical setup and acceptance checks for Stage Wand. Start with [README.md](README.md); leave checks unticked until verified on the actual devices.

- [x] Approve `docs/DESIGN.md` and `PLAN.md` before Wave 0 starts
- [x] Generate the iOS project, use the selected team `DUU8J39BA7`, build, install, and launch on the connected iPhone 17 Pro via Xcode tools.
- [ ] Allow Local Network or developer-trust prompts if shown.
- [ ] Allow Local Network access on the phone.
- [x] Build, sign, verify, and launch `StageWandMac.app`.
- [ ] Grant Accessibility to the packaged Mac app. Re-grant after rebuilding if macOS invalidates the permission.
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

- [ ] Confirm the Mac pairing window is visible on launch/reopen.
- [ ] Confirm taps, incomplete squares, diagonal swipes, and rapid scribbles do not unlock volume mode; a deliberate full square does.
- [ ] Confirm Lock for pocket and leaving the app relock all touch controls while hardware volume remains available.
