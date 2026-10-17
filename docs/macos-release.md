# macOS DMG release (Developer ID, notarized, stapled)

The Mac host ships outside the Mac App Store as a Developer ID–signed DMG.
Gatekeeper accepts it on first launch without the "unidentified developer"
warning, including offline, because both the app and the DMG carry a stapled
notarization ticket. Mac App Store guideline 2.4.5 (sandboxing) does not
apply to Developer ID distribution.

- Workflow: [`.github/workflows/mac-release.yml`](../.github/workflows/mac-release.yml)
- Script: [`scripts/mac-release.sh`](../scripts/mac-release.sh) (also runs locally)
- Bundle: `StageWandMac.app`, bundle ID from [`mac/Info.plist`](../mac/Info.plist) (`….stagewand.mac`), macOS 14+, universal (arm64 + x86_64)
- Hardened runtime: on. Entitlements: none ([`mac/StageWandMac.entitlements`](../mac/StageWandMac.entitlements) explains why). Bluetooth, Local Network and Accessibility are TCC permissions driven by the Info.plist usage strings and the Accessibility prompt, not entitlements.

## What the release job does

1. Fails immediately if any secret below is missing (it never produces an unsigned release).
2. Imports the Developer ID Application `.p12` into a throwaway keychain and checks it really is `Developer ID Application: … (DUU8J39BA7)`.
3. Builds a universal release binary, assembles `StageWandMac.app`, stamps `CFBundleShortVersionString` (from the tag) and `CFBundleVersion` (the run number).
4. `codesign --options runtime --timestamp` with the empty entitlements file, then `codesign --verify --strict --deep`.
5. Zips the app, `xcrun notarytool submit --wait` with the App Store Connect API key, `stapler staple` + `stapler validate`, `spctl -a -t execute -vv`.
6. Builds `StageWand-<version>.dmg` (app + Applications link), signs it, notarizes it, staples it, `spctl -a -t open --context context:primary-signature -vv`.
7. Re-mounts the DMG and re-checks the app inside, uploads the DMG + `.sha256` as an artifact, and attaches them to the GitHub Release `mac-v<version>`.
8. Deletes the temporary keychain and key file.

If notarization is rejected, the job prints `notarytool log` (the reasons) and fails.

## Cutting a release

```sh
git tag mac-v0.2.0 origin/main
git push origin mac-v0.2.0
```

Or run **Actions → macOS release → Run workflow** with a version (tick *publish* to attach it to a release; leave it off for a dry run that only uploads the artifact).

## One-time setup (outside the repo)

1. **Create the certificate.** On the Mac: Xcode → Settings → Accounts → team DUU8J39BA7 → Manage Certificates → **+** → **Developer ID Application**. (Only the Account Holder can create Developer ID certificates. Alternative: developer.apple.com → Certificates → **+** → Developer ID Application → G2 Sub-CA, uploading a CSR from Keychain Access.) Developer ID certificates are included in the paid membership.
2. **Export it.** Keychain Access → My Certificates → right-click *Developer ID Application: … (DUU8J39BA7)* → Export → `.p12` with a strong password. Make sure the private key is included (expand the disclosure triangle; the key must be there).
3. **Set repository secrets** (Settings → Secrets and variables → Actions), without printing them:
   ```sh
   base64 -i DeveloperID.p12 | gh secret set DEVELOPER_ID_CERTIFICATE_P12_BASE64 -R EdmundLimBoEn/stage-wand
   gh secret set DEVELOPER_ID_CERTIFICATE_PASSWORD -R EdmundLimBoEn/stage-wand   # prompts, not echoed
   gh secret set ASC_KEY_ID -R EdmundLimBoEn/stage-wand                          # e.g. the existing key's ID
   gh secret set ASC_ISSUER_ID -R EdmundLimBoEn/stage-wand
   base64 -i AuthKey_<KEYID>.p8 | gh secret set ASC_PRIVATE_KEY_B64 -R EdmundLimBoEn/stage-wand
   ```
   The ASC secrets are the same names (and can be the same key) as Lazy Man's Reminders and `ios-testflight.yml`. A team API key with Developer access or higher can notarize.
4. Delete the local `.p12` export afterwards; keep the certificate in the login keychain.
5. No bundle ID registration or App Store Connect record is needed for Developer ID distribution.

## Local check without secrets

```sh
scripts/mac-release.sh            # ad-hoc signed, not notarized: dist/StageWand-0.0.0.dmg
```

With a Developer ID identity in your login keychain and the API key on disk you can reproduce the release job:

```sh
VERSION=0.2.0 SIGN_IDENTITY="Developer ID Application: <name> (DUU8J39BA7)" NOTARIZE=1 \
ASC_KEY_PATH=~/.appstoreconnect/private_keys/AuthKey_<KEYID>.p8 ASC_KEY_ID=<KEYID> ASC_ISSUER_ID=<issuer> \
scripts/mac-release.sh
```

## Notes for users upgrading

The Developer ID signature has a different designated requirement from the Apple Development build made by `make -C mac app`, so macOS treats it as a new app for TCC: remove the old **StageWandMac** entry under Privacy & Security → Accessibility, then allow the DMG-installed copy once. Later Developer ID releases keep the grant.
