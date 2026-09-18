# Stage Wand rehearsal

Use a physical iPhone and the final Mac app build. Complete the [README setup](../README.md) first. Have a Keynote deck with enough slides, a scrollable page, and two Mac desktops ready. Input goes to the frontmost Mac app.

1. **Cold start both.** Quit and reopen StageWandMac and the phone app. Check the Mac popover's port and Accessibility state; allow firewall and Local Network prompts. Connect both devices to the same Wi-Fi. Keep ARM off and phone media volume near the middle.
2. **Type the code.** Enter the current four-digit code in phone Settings. Expect the phone to show an authenticated connection and the Mac to show a peer. Start the Keynote slideshow and confirm on-screen NEXT/PREV advances and rewinds once per press with ARM off.
3. **Pocket presses, locked.** Lock the phone and put it in your pocket. Press volume up ten times, then volume down ten times, at a deliberate pace. Expect ten next-slide and ten previous-slide actions; record observed counts and haptic behavior. If the volume reaches a limit, unlock and recenter it, then repeat. Separately try starting at maximum and minimum and record which direction still registers. Do not mark locked operation passed based on unlocked results.
4. **Unlock and ARM.** Unlock and return to Stage Wand. Wait for authentication if it reconnects. Check ARM is off after backgrounding: pointer gestures and click buttons must not act, while key buttons still work. Enable ARM.
5. **Trackpad, scroll, three-finger swipe.** Move the cursor with one finger, tap for left click, and use a two-finger tap for right click. Focus a scrollable page and drag with two fingers. Swipe with three fingers left/right to switch desktops and up for Mission Control. Return to Keynote and verify Left, Right, and ESC. Record any missing gesture separately.
6. **Lock/unlock recovery.** Lock for at least ten seconds, try another next/previous volume pair, then unlock. Expect the connection to remain authenticated or recover; target recovery within three seconds after returning to the app. Record the actual delay and any queued actions replayed. Re-enable ARM and verify movement.
7. **Kick.** Click Kick in the Mac popover. Expect the peer to disconnect and the displayed pairing code to change. Confirm the old code cannot regain control. Enter the new code on the phone and verify one NEXT/PREV pair after authentication.
8. **Hotspot fallback.** Enable Personal Hotspot on the iPhone and join it from the Mac. Clear any stale Manual Host and retry discovery. If needed, enter the Mac's new IP plus its popover port in Manual Host (for example `172.20.10.2:8787`; use the actual address). Authenticate and repeat a locked volume pair and an unlocked ARM pointer gesture.

If locked volume control fails, record the failure and rehearse with the phone unlocked using NEXT/PREV. If a gesture fails, record that limitation before presenting. Re-grant Accessibility and rerun the input checks after any Mac rebuild.

## Results

Fill in after testing; these are not preverified results.

| Check | Observed result / pass or fail |
| --- | --- |
| Device models, OS versions, build tested | |
| Cold start and pairing; selected port | |
| Locked volume: next /10, previous /10; haptics | |
| Maximum/minimum volume behavior | |
| ARM off gates pointer/click/scroll/chord; keys work | |
| Pointer, left/right click, scroll | |
| Three-finger left/right/up; keyboard buttons | |
| Lock/unlock recovery time; queued actions | |
| Kick, code rotation, new-code pairing | |
| Hotspot discovery / Manual Host fallback | |
| Remaining limitations and presentation fallback | |

Transfer completed human checks to [HUMANS.md](../HUMANS.md).

## Build-day automated evidence (2026-09-18)

- macOS debug and release builds passed; app bundle code signature verified.
- Full iOS Simulator and signed iPhone 17 Pro builds passed (Swift 6).
- Protocol round trips and nonfinite/unknown-frame rejection passed.
- Real server: auth deadline ~2.1 s, bad auth, displacement, 200 ordered moves, invalid-delta rejection, ping/pong, missing-pong close ~6.4 s, fallback ports, and Bonjour passed.
- Phone Link against real server: eight buffered keys arrived; 900/-850 movement split into 400/-400, 400/-400, 100/-50; scroll arrived; bad and empty codes entered code state.
- Transport fixture: automatic reconnect within 3 s, Bonjour resolution/auth, and delta accumulation passed.
- Mac input self-test moved 100 px right and returned to the original point; click, Esc, and scroll posted.
- Physical locked-screen volume, haptics, gesture interaction, and full Keynote rehearsal remain unverified. Simulator installation succeeded, but runtime launch stalled in this environment.
- Team updated from the plan to DUU8J39BA7 to preserve Edmund’s selection in Xcode. Build products under synced Documents acquired Finder metadata; building the phone in `/tmp/stagewand-device-build` resolved signing.
