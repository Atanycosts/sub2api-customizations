# Preserved source notice (quoted upstream text)

The block below is preserved verbatim from ranxi2001/sub2api commit
b9feadf7dd6228a4117e360821d9994b7cc54897 at
`backend/internal/handler/admin/mature_provider_NOTICE.md`.
Its references to “This fork” and `docs/mature-provider-provenance.json`
describe that source repository. They do not describe this implementation or
refer to a file distributed in this repository.

Source: https://github.com/ranxi2001/sub2api/blob/b9feadf7dd6228a4117e360821d9994b7cc54897/backend/internal/handler/admin/mature_provider_NOTICE.md

---

# Provider protocol provenance

The mature_provider_remote, mature_provider_newapi_auth, mature_provider_session,
mature_provider_key_remote, and mature_provider_key_types files adapt the
corresponding provider_*.go files from mizaawa/zayuapi v0.2.219,
a71aa9615ccb55a19973908c9f261b5ba25a48c6, under the repository LGPL v3 license.
Upstream authors retain their copyrights. Existing source notices are retained.
Source: https://github.com/mizaawa/zayuapi/tree/a71aa9615ccb55a19973908c9f261b5ba25a48c6/backend/internal/handler/admin

This fork replaces whole-setting persistence and process-local locks with SQL
revision checks and row locks. Protocol requests use public IP pinning, no
redirects or environment proxies, and separate cookie jars. Full per-file
provenance is recorded in docs/mature-provider-provenance.json.

---

This implementation only uses the read-only balance/profile field mappings:
Sub2API `/api/v1/user/profile`, and New API `/api/user/self` plus public
`/api/status`. It implements its own bounded observation storage and bearer
credential transport. It does not copy the source's login, refresh-token,
cookie/session, key creation/update, automatic account recovery, or routing code.
See `gateway-operations-NOTICE.md` and the repository's LGPL v3 `LICENSE`.
