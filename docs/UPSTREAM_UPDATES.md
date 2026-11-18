# Updating from Upstream

This repository keeps the original Sub2API history in the `upstream` remote and
stores custom changes on the `origin` remote (`Atanycosts/sub2api-customizations`).

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

Push the reviewed update branch to the customization repository and merge it into
`main` after checks pass:

```sh
git push -u origin HEAD
```

Do not force-push `upstream` or publish private customizations to the public
fork. Keep deployment secrets and runtime data out of Git.

## Current baseline

The gateway release branch includes `v0.2.23` (`5af9afb58`) and the following
version-file commit `2af3f1e15`. This adds GPT-6.2 Sol and Astra, their Codex
descriptors and pricing, and upstream stream and cache-accounting fixes. The
original same-protocol routing and request timing remain in place.

Migration history is keyed by the full SQL filename and checksum. The upstream
`252_channel_pricing_ultrafast_multiplier.sql` and the existing
`252_gateway_request_timings.sql` are distinct migrations; preserve both names
and the bytes of previously applied files. Production deployment status is
tracked separately in `docs/DEPLOYMENT.md`.
