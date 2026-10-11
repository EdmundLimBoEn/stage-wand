# App Store audit, 11 October 2026

Audited production `33c1fa0` and integration `acf2ac1` using three subagents. Edmund chose to keep pocket mode and audit App Store blockers only. No iOS behavior or submission assets were changed. Merging the release fixes does not make this app App Store ready.

## Rejection blockers retained by choice

- `ios/StageWand/Volume.swift` observes system volume, sends slide commands, and resets volume through a hidden `MPVolumeView`. This conflicts with Apple's rule against altering hardware volume-switch behavior, guideline 2.5.9.
- The same file loops silent audio with a playback audio session, and `ios/project.yml` declares background audio. Using silent playback to keep a presentation remote alive conflicts with background services' intended-purpose requirement, guideline 2.5.4. Genuine background Bluetooth use must be evaluated separately.

Source: [App Review Guidelines](https://developer.apple.com/app-store/review/guidelines/#software-requirements).

## Other repository gaps

- No `PrivacyInfo.xcprivacy` is bundled although Settings, Link, and StageWandApp use UserDefaults. App-owned preferences need an approved required reason, ordinarily `CA92.1` for `NSPrivacyAccessedAPICategoryUserDefaults`. Verify every used API and the final archive. [Required-reason APIs](https://developer.apple.com/documentation/bundleresources/describing-use-of-required-reason-api).
- Settings has no privacy-policy access or support contact. Supply accessible in-app policy access, a live public policy URL in App Store Connect, and an easy way to contact the developer. [Guidelines 5.1.1 and 1.5](https://developer.apple.com/app-store/review/guidelines/).
- No iOS icon asset catalog exists. Configure the app icon and validate the compiled archive. [App icon configuration](https://developer.apple.com/documentation/xcode/configuring-your-app-icon).
- `ios/project.yml` does not explicitly configure marketing/build versions. Generated defaults may provide values; inspect `CFBundleShortVersionString` and `CFBundleVersion` in the distribution archive rather than assuming they are absent.
- Pairing and settings require drawing the unlock gesture. An accessible alternative would improve usability; test VoiceOver and large text before making accessibility claims.

## Submission evidence still needed

The integration branch already has a passing Apple CI job that builds the iOS Debug simulator with signing disabled and builds/tests the macOS host. It does not produce or validate a signed iOS distribution archive. Linux tests and substituted Bluetooth frameworks cannot certify Apple device behavior.

- Build the submission with the current required SDK/toolchain. Apple has required Xcode 26 or later and iOS 26 SDK or later for iOS uploads since 28 April 2026; the iOS 17 deployment target can remain. [Apple submission requirements](https://developer.apple.com/news/?id=ueeok6yw).
- Produce and validate a signed device archive, check icon/versions/privacy manifest, then complete physical iPhone/iPad, permission denial, interruption/reconnect, and IPv6-only testing.
- Complete App Store Connect screenshots, description, age rating, privacy answers, review contact, support/privacy URLs, and encryption-export questions. Explain the required companion computer and provide working download, installation, and pairing instructions for App Review. [Required properties](https://developer.apple.com/help/app-store-connect/reference/app-information/required-localizable-and-editable-properties).
- Evaluate a separate distribution plan if the macOS companion is intended for the Mac App Store: guideline 2.4.5 requires sandboxing, and the current Accessibility/input-injection host is not certified for that channel. Do not mistake the iOS submission audit for Mac App Store approval.

No app account creation, purchases, advertising, or tracking SDKs were found. Account deletion, StoreKit, and ATT are not currently applicable based on this source audit.

Privacy labels require accurate operational evidence. Pairing/preferences are stored locally, while authentication and control frames are sent to the selected companion. Optional Cloudflare tunnels introduce provider/operator handling. Check logging and retention before claiming Data Not Collected. Apple's definition distinguishes transient request servicing from retained data. [App Privacy Details](https://developer.apple.com/app-store/app-privacy-details/).

## Release integration

At audit start, only PR #6 was open, promoting `dev` to `main`. PRs #4 and #5 were already on `dev`. PR #6's Linux and Windows jobs failed on called Go standard-library vulnerabilities fixed in Go 1.26.9. The release toolchain fix pins that patch and updates `x/net` to v0.60.0 and `x/sys` to v0.48.0. Existing uncommitted `feat/ci-green` work was preserved; its toolchain edits were applied independently here.

Written by gpt-6.1-sol in Codex on behalf of Edmund
