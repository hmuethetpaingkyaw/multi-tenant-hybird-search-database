# Multi-tenant hybrid search database

Go + PostgreSQL service for tenant-isolated document search (RLS, pgvector, BM25).
**Status: Phase 2 (multi-tenant architecture and RLS).** Document CRUD and search arrive in later
phases; see [docs/project-plan.md](docs/project-plan.md).

## Run

Requires Docker (Compose v2). Optional: Go 1.26 (the toolchain downloads automatically).

```sh
cp .env.example .env     # development defaults; DB_PORT defaults to 5433 to avoid clashes
make up                  # db -> migrations -> API
make seed                # tenants A/B/C + one user each; prints TOKEN_A/B/C (valid 24h)
curl localhost:8080/healthz
curl localhost:8080/readyz
curl -H "Authorization: Bearer $TOKEN_A" localhost:8080/api/v1/me   # tenant A's tenant + user
make test                # unit + integration (Testcontainers); `make test-short` skips Docker tests
make down
```

`make seed` runs on the host against the compose database (owner role, `localhost:$DB_PORT`),
is idempotent, and prints fresh tokens each time. Copy a `TOKEN_x=...` line into your shell
(`export TOKEN_A=...`).

## Pinned versions

| Component | Version | Note |
|---|---|---|
| ParadeDB image | `paradedb/paradedb:0.26.6-pg17` | PostgreSQL 17, pg_search 0.26.6, pgvector 0.8.6 (both verified present) |
| golang-migrate image | `migrate/migrate:v4.18.3` | |

The image tag is duplicated in `docker-compose.yml` and `tests/helpers_test.go`; change both together.

## Role model

| Role | Purpose |
|---|---|
| `postgres` | Bootstrap superuser. Used only by the init script (creates roles, installs extensions). |
| `search_owner` | Owns schema and tables; runs migrations and `cmd/seed`. Not superuser, no BYPASSRLS. |
| `search_app` | Used by the API. Not an owner, no BYPASSRLS, no DDL. Read-only on `tenants`. |

Roles are created by `docker/postgres/init/01-roles.sh` on first start of an empty data volume.
Changing passwords later requires `docker compose down -v` (this deletes data).

## Tenant isolation (RLS)

Isolation is enforced by PostgreSQL, not by application filters.

1. **Identity.** Clients send a dev JWT (HS256, claims `sub` = user id, `tenant_id`, `exp`), verified with
   `JWT_SECRET`. The tenant is taken only from the verified token, never from the request body, query or headers.
2. **Tenant context.** All tenant-scoped queries go through `database.WithTenantTx`, which opens a transaction and
   runs `set_config('app.tenant_id', <uuid>, true)`. The `true` makes it transaction-local, so it vanishes at
   commit/rollback and cannot leak to the next user of a pooled connection (covered by tests).
3. **Policies** (migration `000004_rls`). `app_current_tenant()` reads the setting; policies compare `tenant_id`
   to it for SELECT/INSERT/UPDATE/DELETE, with `WITH CHECK` so a row cannot be written into, or moved to, another tenant.
4. **Fail closed.** With no (or empty) tenant context, `app_current_tenant()` is NULL, so reads return no rows and
   writes are rejected.
5. **FORCE RLS** is on `users`, `documents` and `document_embeddings`, so even the table owner is subject to policies.
   `tenants` is ENABLE-only (not FORCE): the owner role creates tenant rows (seed), and the app role may only
   SELECT its own row (INSERT/UPDATE/DELETE are revoked).
6. **Foreign keys bypass RLS** by design. Cross-tenant links are prevented by the composite FK
   `(tenant_id, document_id) -> documents (tenant_id, id)`, not by policies.

Rule for contributors: never add `WHERE tenant_id = ...` in application SQL to "help" RLS, and never use
session-level `SET` for the tenant.

## Schema (migrations/)

1. `extensions`: `vector`, `pg_search` (idempotent; normally installed by the init script).
2. `core_tables`: `tenants`, `users`, `documents`, `document_embeddings`. `embedding` is `vector(768)`
   (nomic-embed-text, Phase 4).
3. `app_grants`: table privileges for `search_app`.
4. `rls`: tenant helper function, RLS enabled/forced, per-command policies, `tenants` made read-only for the app.

## Endpoints

- `GET /healthz`: process is up (no auth).
- `GET /readyz`: database reachable and `vector` + `pg_search` installed; 503 otherwise (no auth).
- `GET /api/v1/me`: the caller's tenant and user. 401 for a missing/invalid token, 404 if the user or tenant is not
  visible under RLS.

## Configuration (environment)

`DATABASE_URL` (required, app role), `JWT_SECRET` (required, at least 32 bytes), `HTTP_PORT` (8080),
`LOG_LEVEL` (info), `DB_MAX_CONNS` (10), `DB_MIN_CONNS` (1), `DB_MAX_CONN_LIFETIME` (30m),
`DB_HEALTH_CHECK_PERIOD` (1m). `cmd/seed` additionally needs `SEED_DATABASE_URL` (owner role; set by `make seed`).

## Security tests

`go test ./tests/...` starts a ParadeDB container and checks, with tenants A, B, C: cross-tenant read/update/delete
are blocked, embeddings are isolated, missing context returns nothing, `WITH CHECK` blocks writes into other tenants,
the composite FK rejects cross-tenant embeddings, FORCE RLS binds the owner, and pooled connections never carry
tenant context (including 16 goroutines x 50 iterations over a 4-connection pool; run with `-race` for the race check).
