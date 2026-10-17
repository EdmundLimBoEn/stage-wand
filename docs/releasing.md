# Releasing Stage Wand

```
feat/* | fix/* | chore/*  ──PR──▶  dev  ──PR──▶  main  ──▶  macOS DMG release + TestFlight upload
```

- **Work** on a `feat/<slug>`, `fix/<slug>` or `chore/<slug>` branch from `dev` (`./scripts/worktree add <slug>`).
- **Land** it with a PR into `dev`. CI must be green. Nothing is signed or uploaded.
- **Ship** by opening a PR from `dev` into `main` and merging it with a merge commit. That push to `main` *is* the release.

`branch-policy.yml` enforces this: PRs into `dev` must come from `feat/*`, `fix/*` or `chore/*`, and PRs into `main` must come from `dev`. Urgent fixes go `fix/<slug>` → `dev` → `main` like everything else.

## The version

`VERSION` (for example `0.1.0`) is the single source of truth for the marketing version of the macOS host and the iOS remote. Copies in `mac/Info.plist` (`CFBundleShortVersionString`) and `ios/project.yml` (`MARKETING_VERSION`) keep local builds honest. CI fails if they drift.

```sh
scripts/version.sh            # print it
scripts/version.sh set 0.2.0  # bump VERSION, mac/Info.plist and ios/project.yml together
scripts/version.sh check      # what CI runs
```

Bump it in a normal `feat/*` or `chore/*` PR when you want a new public macOS release.

## What runs where

| Event | Cross-platform CI | macOS release | iOS TestFlight |
| --- | --- | --- | --- |
| PR into `dev` or `main` | full | ad-hoc DMG build when `mac/`, `VERSION` or the script changes | config check only (when the iOS app changes) |
| Push to `dev` | full | – | config check only |
| **Push to `main`** | full, plus the Linux package release | **sign, notarize, staple and publish `mac-v<VERSION>`**, or skip if that release exists | **archive, sign, upload** when `ios/`, `Shared/`, `VERSION` or the workflow changed |
| Manual run (any branch) | – | sign + notarize, artifact only (tick *publish* to release) | archive + upload (untick *upload* for a dry run) |

PRs never see secrets, so forks and PRs stay green without them.

### macOS (`mac-release.yml`)
On a push to `main`, a `plan` job reads `VERSION` and checks whether the tag `mac-v<VERSION>` exists:
- **Exists:** the run succeeds with a notice ("already released, bump the version"). Merges that don't change the version are no-ops, and re-running is safe.
- **New:** it builds a universal `StageWandMac.app` and signs it with Developer ID (hardened runtime). It notarizes and staples the app, builds, signs, notarizes and staples `StageWand-<VERSION>.dmg`, checks both with `stapler validate` and `spctl`, then publishes the GitHub Release `mac-v<VERSION>`.
  - The release is not marked "latest", so the Linux `releases/latest/download/…` links keep working.
  - A missing secret fails the run. It never publishes an unsigned DMG.

### iOS (`ios-testflight.yml`)
On a push to `main` that touches the app, it runs the shared Swift checks and archives with the Apple Distribution certificate and an API-managed App Store profile. It then exports and uploads to TestFlight with the What to Test notes from `ios/TestFlight/WhatToTest.en-US.txt`.
- The build number is `<run number>.<run attempt>`, so it always increases. You never bump it by hand. App Store Connect can raise it further if needed.
- If the repository variable `TESTFLIGHT_EXTERNAL_GROUP` is set (for example `Public Testers`), the processed build is added to that external group and submitted to Beta App Review. Unset means internal testers only.
- A new `VERSION` needs a fresh Beta App Review for external testers. Later builds of the same version usually don't.

### Concurrency
Both release workflows use a per-branch concurrency group with `cancel-in-progress: false`. Two quick merges into `main` run one after the other instead of racing: the second macOS run sees the first one's tag and skips. GitHub keeps at most one *pending* run per group, so a burst of merges collapses to the newest.

## Branch protection (GitHub settings)
`main` and `dev` are both protected:
- Changes need a pull request (no approvals required; solo repo).
- The cross-platform checks and `policy` must pass.
- No force pushes and no deletion.
- Admins (Edmund) can bypass in an emergency.

## Shipping checklist
1. `dev` is green and contains what you want to ship.
2. If this should produce a new DMG, `scripts/version.sh set X.Y.Z` was merged into `dev`.
3. Open the PR `dev` → `main` and merge it with a **merge commit**.
4. Watch **Actions**:
   - *macOS release*: the new release `mac-vX.Y.Z` with the notarized DMG and `.sha256`.
   - *iOS TestFlight*: the build appears in App Store Connect → TestFlight within ~15 minutes.
   - *Cross-platform*: the moving `linux` release.
5. External testers: App Store Connect → TestFlight → the group shows the build *Waiting for Review* → *Testing*.

One-time setup (certificates, secrets, App Store Connect record, Test Information) is in [macos-release.md](macos-release.md) and [testflight.md](testflight.md).
