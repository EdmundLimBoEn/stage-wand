# HUMANS.md

Physical setup and acceptance checks for Stage Wand. Start with [README.md](README.md); leave checks unticked until verified on the actual devices.

- [x] Approve `docs/DESIGN.md` and `PLAN.md` before Wave 0 starts
- [ ] Generate the iOS project, connect the iPhone, select team `DUU8J39BA7`, and Run StageWand from Xcode. Enable Developer Mode and Trust developer if requested.
- [ ] Allow Local Network access on the phone.
- [ ] Build and launch `StageWandMac.app`; grant Accessibility. Re-grant after rebuilding if macOS invalidates the permission.
- [ ] Click Allow on the macOS firewall incoming-connections prompt.
- [ ] Put both devices on the same Wi-Fi, or enable the iPhone hotspot and join it from the Mac.
- [ ] Type the current four-digit Mac pairing code into phone Settings and verify authentication.
- [ ] Enable Mission Control shortcuts (Control+Left/Right/Up) and create a second desktop.
- [ ] Start a Keynote slideshow and set phone media volume away from maximum/minimum.
- [ ] Run the locked pocket test on the physical phone: volume up advances and volume down goes back with ARM off. Record counts and any missing haptics.
- [ ] Verify unlocked ARM controls: pointer, left/right click, scrolling, three-finger swipes, and keyboard buttons.
- [ ] Verify lock/unlock keeps or recovers the connection; re-enable ARM for pointer control.
- [ ] Verify Kick disconnects the phone and rotates the code; pair again with the new code.
- [ ] Verify hotspot fallback, including Manual Host with the current Mac IP and popover port if discovery fails.
- [ ] Complete [scripts/rehearse.md](scripts/rehearse.md), record failures or fallbacks, and avoid rebuilding before the presentation.
