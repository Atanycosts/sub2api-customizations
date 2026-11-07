# Updating from Upstream

This repository keeps the original Sub2API history in the `upstream` remote and
stores custom changes on the private `origin` remote.

## Update procedure

Start with a clean checkout of `main`, then create an update branch:

```sh
git status --short
git fetch upstream
git switch -c "sync/upstream-$(date +%Y%m%d)" main
git merge --no-ff upstream/main
```

If following a specific upstream release instead of its latest `main`, merge
the selected upstream tag, such as `v0.2.20`, after fetching tags.

Resolve conflicts by preserving both upstream behavior and custom features.
Regenerate Go dependency injection after resolving changes in `wire.go`:

```sh
cd backend && go generate ./cmd/server
```

Run the frontend production build and test suite, plus the relevant Go tests
(and `go test ./...` when the local environment supports the full suite). Check
database migrations and the account-memory behavior when an update touches
either area. Keep deployment as a separate, deliberate step after review.

Push the reviewed update branch to the private repository and merge it into
`main` after checks pass:

```sh
git push -u origin HEAD
```

Do not force-push `upstream` or publish private customizations to the public
fork. Keep deployment secrets and runtime data out of Git.

## Current baseline

The custom branch was reconnected to upstream commit `876b96d20` (2026-11-04)
and subsequently merged with `upstream/main` at `6b4b37db4` on 2026-11-07
(v0.2.20 plus its version-file update). This adds the GPT-6.1 Terra Codex
descriptor, effort validation, compatibility cache keys, and subscription
long-context billing fixes while retaining the UI and account-memory
customizations. The original imported snapshot and pre-update worktree are
preserved locally under `backup/pre-upstream-20261107`. Production deployment
status is tracked separately in `docs/DEPLOYMENT.md`.
