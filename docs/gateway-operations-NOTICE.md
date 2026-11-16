# Gateway operations provenance

`frontend/src/utils/magpieImport.ts` adapts the corresponding file from
ranxi2001/sub2api commit b9feadf7dd6228a4117e360821d9994b7cc54897, licensed
under GNU LGPL v3. Existing author copyrights remain with their authors.
Source: https://github.com/ranxi2001/sub2api/blob/b9feadf7dd6228a4117e360821d9994b7cc54897/frontend/src/utils/magpieImport.ts

The read-only Sub2API/New API wallet field mappings in
`backend/internal/service/gateway_operations_wallet.go` were informed by that
commit's `backend/internal/handler/admin/mature_provider_remote.go`.
Those provider protocols adapt mizaawa/zayuapi v0.2.219 commit
a71aa9615ccb55a19973908c9f261b5ba25a48c6 under LGPL v3. See the preserved
`gateway-operations-provider-NOTICE.md` for upstream provenance.

This implementation includes no remote login, token creation, key mutations,
automatic account disable/recovery, or provider scheduler. Authentication uses
an administrator-supplied read-only bearer token. Wallet refresh only performs
GET requests, checks remote identity, pins a public DNS result, and rejects redirects.

Corresponding LGPL v3 license text is preserved in the repository's `LICENSE`.
The import scheme is also registered by the locally installed `/Applications/magpie.app`;
no client import was executed during development.
