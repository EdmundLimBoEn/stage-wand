# Agent notes

Edmund is the only contributor. Do not invent team process: no reviewers, no “open a PR for this,” no stacking PRs unless he asks. Isolation is the point of `main` / `dev` / worktrees, not GitHub ceremony.

## Where you are

| Directory | Branch | Role |
| --- | --- | --- |
| This clone (`stage-wand`) | `dev` | Primary Cursor workspace. Keep it here. |
| `../stage-wand-trees/main` | `main` | Locked production checkout. Do not remove. Do not commit here. |
| `../stage-wand-trees/feat-<slug>` | `feat/<slug>` | One task. Create with `./scripts/worktree add`. |

Before you edit, run `git rev-parse --abbrev-ref HEAD` and `git worktree list`.

- If this directory is on `dev` and the task is more than a one-line doc/typo: `./scripts/worktree add <slug>`, then do **all** file edits in that new directory. Do not `git switch` this directory off `dev`.
- If you are already inside a `feat/*` or `hotfix/*` worktree, stay there. Do not also dirty `dev`.
- If you are on `main`, stop. Move the work to a `hotfix/*` worktree from `main`, or a `feat/*` worktree from `dev`.

Git will refuse to check out a branch that is already in another worktree. Do not `--force` around that.

## What you may change

- Commit on `feat/*` or `hotfix/*` when Edmund asks for a commit.
- Merge a finished feature into `dev` when he asks to land it (`git merge --ff-only` from the `dev` checkout, then `./scripts/worktree rm <slug>`).
- Push `origin/dev` after landing, if he asked to push.
- Push `origin/main` only as a promotion or hotfix merge, if he asked to push.

Never: commit on `main`, force-push `main` or `dev`, delete `main`/`dev`, `git switch` the primary clone to a feature branch, or open a GitHub PR unless he asked.

## Landing (no PR)

From the **`dev` checkout**, after the feature worktree is committed:

```sh
git fetch origin
git merge --ff-only feat/<slug>
# if that fails: git merge --no-ff feat/<slug>
git push origin dev          # only if asked
./scripts/worktree rm <slug>
```

Promote `dev` → `main` only when he wants a shippable / presentable tip, not after every task:

```sh
git fetch origin
git -C ../stage-wand-trees/main merge --ff-only origin/dev
git push origin main         # only if asked
```

Hotfix: `./scripts/worktree add hotfix/<slug> main`, land on `main`, merge the same branch into `dev`.

## PRs

Use a PR only if he asks, or if he wants CI without merging yet. Base is `dev` for features. Base is `main` only for `dev` (promotion) or `hotfix/*`.

## Hooks

`.githooks` blocks ordinary commits and non-merge pushes on `main`. `./scripts/worktree setup` sets `core.hooksPath`. Do not bypass the hooks.