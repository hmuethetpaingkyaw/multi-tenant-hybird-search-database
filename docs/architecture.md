# Architecture notes

The layout follows the conventions of `sfa_api` (flat layers, one file per entity per layer, Port interfaces next to
their implementations, request/response packages, one central error handler). These are the places where this project
deliberately differs, and why.

| Difference | Why |
|---|---|
| Tenant isolation is enforced by PostgreSQL RLS; every repository call runs inside `db.WithTenantTx` and there is no global DB handle | A global handle plus optional `WHERE group_id = ...` filters makes tenant leaks easy; here a query that skips the tenant context returns nothing |
| Explicit constructor injection in `cmd/api/main.go` (usecases do not build their own repositories) | Tests inject fakes; the app injects the tenant-scoped pool |
| Usecases never import `http/*`; controllers turn request structs into domain types | Keeps business logic independent of the transport; checked by `make check-layers` |
| Package `apperror` instead of `error` + an `appErr` alias | Unique package names mean no import aliases anywhere |
| Gin with `c.Error(err)` + one error middleware instead of Echo's returned errors | Same central-handler effect without changing the framework |
| Hand-written validation in `http/request`, no validator library and no struct tags | Messages are an API contract; no extra dependency |
| Error body is `{"error": "..."}`, not `{"errors": [...]}` | Existing API contract is unchanged |
| Hand-written fakes in `mock/` folders instead of mockgen | No extra tooling for four small ports |

Behavior is locked by `tests/api_contract_test.go`, which drives the whole stack through every documented success and
failure response. When packages move, only its `buildContractRouter` wiring may change.
