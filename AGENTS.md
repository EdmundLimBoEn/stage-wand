# Agent notes

Edmund is the only contributor. The flow is **feat → dev → main**, and every hop is a GitHub PR:

```
feat/<slug> | fix/<slug> | chore/<slug>  ──PR──▶  dev  ──PR──▶  main (= release)
```

Merging `dev` into `main` builds, signs, notarizes and publishes the macOS DMG, and uploads the iOS app to TestFlight. See [docs/releasing.md](docs/releasing.md). One thread of work = one branch = one PR. No reviewers are required. CI and `branch-policy.yml` are the gate.

## Where you are

| Directory | Branch | Role |
| --- | --- | --- |
| This clone (`stage-wand`) | `dev` | Primary Cursor workspace. Keep it here. |
| `../stage-wand-trees/main` | `main` | Locked production checkout. Do not remove. Do not commit here. |
| `../stage-wand-trees/feat-<slug>` | `feat/<slug>` | One task. Create with `./scripts/worktree add <slug>`. |

Before you edit, run `git rev-parse --abbrev-ref HEAD` and `git worktree list`.

- If this directory is on `dev` and the task is more than a one-line doc/typo: `./scripts/worktree add <slug>` (or `fix/<slug>`, `chore/<slug>`), then do **all** file edits in that new directory. Do not `git switch` this directory off `dev`.
- If you are already inside a `feat/*`, `fix/*` or `chore/*` worktree, stay there. Do not also dirty `dev`.
- If you are on `main`, stop. Move the work to a `fix/*` or `feat/*` worktree from `dev`.

Git will refuse to check out a branch that is already in another worktree. Do not `--force` around that.

## Landing a change

1. Commit on the `feat/*` / `fix/*` / `chore/*` branch and push it.
2. Open a PR into **`dev`**. Wait for CI to go green. Merge it when Edmund asks (merge commit or squash), then `./scripts/worktree rm <slug>`.

## Shipping (dev → main)

Only when Edmund wants a release:

1. If a new macOS DMG should come out, bump the version first in a normal PR: `scripts/version.sh set X.Y.Z`. `VERSION` is the single source of truth; `mac/Info.plist` and `ios/project.yml` follow it and CI checks they match.
2. Open a PR from **`dev`** into **`main`** and merge it with a **merge commit**.
3. The push to `main` runs:
   - the *macOS release*: publishes `mac-v<VERSION>`, or skips with a notice if that release already exists;
   - *iOS TestFlight*: when the iOS app, `Shared/` or `VERSION` changed;
   - the Linux packages.

Urgent fixes take the same path (`fix/*` → `dev` → `main`); there is no direct-to-main hotfix.

## Never

- Commit on `main`, push directly to `main` or `dev`, force-push either, or delete them. Both are protected; admin bypass is for Edmund in an emergency, not for agents.
- Open PRs into `main` from anything but `dev`, or into `dev` from anything but `feat/*`, `fix/*` or `chore/*`.
- Print, log or commit signing material (`.p12`, `.p8`, passwords, base64 blobs). Secrets live in GitHub Actions secrets only.

## Hooks

`.githooks` blocks ordinary commits and non-merge pushes on `main`. `./scripts/worktree setup` sets `core.hooksPath`. Do not bypass the hooks.
