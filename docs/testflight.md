# iOS TestFlight (internal and external testers)

Stage Wand is not going to the App Store: the locked-screen volume-button control very likely fails review under guideline 2.5.9 (and the silent background audio that keeps it working under 2.5.4). TestFlight is the distribution channel. Internal testers (up to 100 App Store Connect users) need no review. **External testers** (public link, up to 10,000) need the first build of each version approved by **Beta App Review**, which applies a lighter subset of the guidelines. The volume behaviour may still be questioned, so the review notes below disclose it up front.

- Workflow: [`.github/workflows/ios-testflight.yml`](../.github/workflows/ios-testflight.yml) (runs on push to `main` touching `ios/` or `Shared/`, or manually)
- What to Test text (uploaded with every build): [`ios/TestFlight/WhatToTest.en-US.txt`](../ios/TestFlight/WhatToTest.en-US.txt)
- Privacy policy: [`docs/privacy.md`](privacy.md), public at <https://github.com/EdmundLimBoEn/stage-wand/blob/main/docs/privacy.md> once merged to `main`

## What the workflow does

1. A `gate` job checks the secrets and the `ASC_APP_ID` variable. If any are missing, it posts a notice and the upload job is **skipped**, not failed.
2. On `macos-26` with stable Xcode 26.6, it runs `scripts/apple-swift-checks.sh` (the same Swift protocol, queue, URL, unlock, session and Bluetooth checks as cross-platform CI). It then generates the project with XcodeGen and imports the Apple Distribution certificate.
3. `scripts/asc-appstore-profile.py` finds or creates an App Store provisioning profile (`Stage Wand App Store CI <cert id>`) for the imported Apple Distribution certificate through the API, then `xcodebuild archive` signs manually with it. (Automatic signing on a fresh runner would mint a new Apple Development certificate every run.) `CURRENT_PROJECT_VERSION` = `<run number>.<run attempt>`, so the build number always increases. Export also sets `manageAppVersionAndBuildNumber`, so App Store Connect can raise it further if a number was already used.
4. It checks that the archive contains the privacy manifest and the compiled app icon, then exports an App Store Connect IPA.
5. `asc builds upload --wait --test-notes …` uploads the build, waits for processing and sets *What to Test*.
6. **Optional external step:** if the repository variable `TESTFLIGHT_EXTERNAL_GROUP` is set (and the manual run's *external* box is ticked), it runs `asc builds add-groups --group "$TESTFLIGHT_EXTERNAL_GROUP" --submit --confirm`. That adds the build to the external group and submits it to Beta App Review. When the variable is unset this step does nothing.

## One-time setup

### 1. Bundle ID (Apple Developer portal)
Certificates, Identifiers & Profiles → Identifiers → **+** → App IDs → App:
- Description: `Stage Wand`
- Bundle ID (Explicit): exactly `PRODUCT_BUNDLE_IDENTIFIER` in [`ios/project.yml`](../ios/project.yml) (`systems.<prefix>.stagewand`, the same prefix as the Mac host's `….stagewand.mac`)
- Capabilities: none needed (Bluetooth, background audio and Bonjour need no App ID capability)

There are no app extensions, so this is the only iOS bundle ID. Automatic signing would also register it, but only with an Admin-role key, so registering it by hand is more predictable.

### 2. App Store Connect app record
App Store Connect → Apps → **+** → New App:
- Platform: **iOS**
- Name: **Stage Wand** (if taken: *Stage Wand Remote*). You can change it later.
- Primary language: English (U.S.)
- Bundle ID: the one from step 1
- SKU: `stagewand-ios-2026` (private, any unique string)
- User access: Full Access

Creating the record does not submit anything to the App Store. Copy the record's **Apple ID** (App Information → General) into the repository **variable** `ASC_APP_ID`.

### 3. Secrets (same names and values as Lazy Man's Reminders)
```sh
R=EdmundLimBoEn/stage-wand
gh secret set ASC_KEY_ID -R $R
gh secret set ASC_ISSUER_ID -R $R
base64 -i AuthKey_<KEYID>.p8 | gh secret set ASC_PRIVATE_KEY_B64 -R $R
base64 -i AppleDistribution.p12 | gh secret set APPSTORE_CERTIFICATES_FILE_BASE64 -R $R
gh secret set APPSTORE_CERTIFICATES_PASSWORD -R $R
gh variable set ASC_APP_ID -R $R --body <numeric Apple ID>
```
The Apple Distribution certificate is per team, so the `.p12` already used for LMR (team DUU8J39BA7) works here. The API key needs the **Admin** role (or App Manager with access to certificates/profiles) so CI can create the App Store profile. The key LMR uses already works for this.

### 4. Internal testing
TestFlight → Internal Testing → **+** → group `Team`, tick *Enable automatic distribution*, add yourself. Every processed build then reaches your phone with no review.

### 5. Test Information (required before any external review)
TestFlight → **Test Information** (left sidebar), English (U.S.):

| Field | Value |
| --- | --- |
| Beta App Description | see below |
| Feedback Email | `FEEDBACK_EMAIL_PLACEHOLDER` (replace with a real inbox you read) |
| Marketing URL | `https://github.com/EdmundLimBoEn/stage-wand` |
| Privacy Policy URL | `https://github.com/EdmundLimBoEn/stage-wand/blob/main/docs/privacy.md` |
| Beta App Review: contact first/last name, phone, email | your details (Apple uses these only for review questions) |
| Sign-in required | **No** (no accounts; leave demo account empty) |
| Review notes | see below |

Export compliance is pre-answered by `ITSAppUsesNonExemptEncryption = NO` in the Info.plist (the app uses no encryption beyond standard OS TLS).

### 6. External group and public link
1. TestFlight → External Testing → **+** → group name `Public Testers`.
2. In the group: Builds → **+** → pick the processed build → *What to Test* is prefilled → **Submit for Review**. (Or let CI do this: `gh variable set TESTFLIGHT_EXTERNAL_GROUP -R EdmundLimBoEn/stage-wand --body "Public Testers"`.)
3. Once approved (usually within a day), open the group → **Public Link** → *Enable Public Link*, optionally limit the number of testers (e.g. 100) and add tester criteria (iOS 17+). Share the `https://testflight.apple.com/join/…` URL.
4. Later builds of the same version usually go to testers without a full re-review. A new `MARKETING_VERSION` needs review again.

Builds expire after 90 days; push to `main` or run the workflow to refresh.

## Copy for App Store Connect

### Beta App Description
> Stage Wand turns your iPhone into a presentation remote and trackpad for your own computer. Advance slides with on-screen buttons or the iPhone's volume buttons (even with the phone locked), move the pointer with a trackpad, click and scroll. It connects directly over Bluetooth or your local Wi-Fi to the free, open-source Stage Wand host running on a Mac, Windows or Linux computer. There is no account and no server: your phone talks only to your computer, protected by a four-digit pairing code.
>
> Get the computer host at https://github.com/EdmundLimBoEn/stage-wand.

### What to Test
Uploaded automatically from [`ios/TestFlight/WhatToTest.en-US.txt`](../ios/TestFlight/WhatToTest.en-US.txt). Edit that file to change it.

### Beta App Review notes
> Stage Wand is a presentation remote. The iPhone controls slides and the mouse pointer on the tester's own computer, which runs our free companion host for macOS, Windows or Linux (open source: https://github.com/EdmundLimBoEn/stage-wand; macOS DMG and Linux packages under Releases). There is no account, no sign-in and no developer server. The phone talks only to the paired computer over Bluetooth LE or the local network, authenticated with a 4-digit code shown on the computer.
>
> HOW TO TRY IT: Main screen controls are locked against pocket touches. To unlock, trace a square clockwise with one finger, starting at the top-left corner (an on-screen guide shows the path). Settings is where you enter the computer's pairing code. Without a computer running the host, the app launches, unlocks and shows "Disconnected"; that is expected.
>
> PLEASE NOTE: HARDWARE VOLUME BUTTONS (guideline 2.5.9). We want to be upfront about this. While Stage Wand is running, pressing volume up sends "next slide" and volume down sends "previous slide" to the paired computer, so a presenter can change slides with the phone in their pocket or locked. To do this, the app observes changes to the system output volume and then restores the previous level through MPVolumeView, so the volume does not drift during a talk. To keep receiving presses while the screen is locked, it plays a silent audio loop using the "audio" background mode (relevant to 2.5.4). This lasts until the user force-quits the app. We understand this repurposes the volume buttons and is why the app is not submitted to the App Store. We are using TestFlight to evaluate the interaction with real presenters. The on-screen Next/Previous buttons give the same control without the volume buttons. If this behaviour is not acceptable for external TestFlight testing, please tell us and we will provide a build with the volume control turned off by default.
>
> Bluetooth and Local Network permissions are used only to find and connect to the user's own computer. No data is collected; see the privacy policy.

## Checks before the first external submission
- [ ] `FEEDBACK_EMAIL_PLACEHOLDER` replaced here, in App Store Connect and in `docs/privacy.md`
- [ ] Privacy policy URL loads (requires this branch merged to `main`)
- [ ] An internal build installed and paired with a host at least once
- [ ] Mac host DMG release published (see `docs/macos-release.md`) so external testers have something to pair with
