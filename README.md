# Multi-tenant hybrid search database

Go + PostgreSQL service for tenant-isolated document search (RLS, pgvector, BM25).
**Status: Phase 1 (foundation).** RLS, auth, documents, and search arrive in later phases;
see the project plan.

## Run

Requires Docker (Compose v2). Optional: Go 1.26 (the toolchain downloads automatically).

```sh
cp .env.example .env     # development defaults; DB_PORT defaults to 5433 to avoid clashes
make up                  # db -> migrations -> API
curl localhost:8080/healthz
curl localhost:8080/readyz
make test                # unit + integration (Testcontainers); `make test-short` skips Docker tests
make down
```

## Pinned versions

| Component | Version | Note |
|---|---|---|
| ParadeDB image | `paradedb/paradedb:0.26.6-pg17` | PostgreSQL 17, pg_search 0.26.6, pgvector 0.8.6 (both verified present) |
| golang-migrate image | `migrate/migrate:v4.18.3` | |

The image tag is duplicated in `docker-compose.yml` and `tests/foundation_test.go`; change both together.

## Role model

| Role | Purpose |
|---|---|
| `postgres` | Bootstrap superuser. Used only by the init script (creates roles, installs extensions). |
| `search_owner` | Owns schema and tables; runs migrations. Not superuser, no BYPASSRLS. |
| `search_app` | Used by the API. Not an owner, no BYPASSRLS, no DDL. RLS applies to it (Phase 2). |

Roles are created by `docker/postgres/init/01-roles.sh` on first start of an empty data volume.
Changing passwords later requires `docker compose down -v` (this deletes data).

## Schema (migrations/)

1. `extensions`: `vector`, `pg_search` (idempotent; normally installed by the init script).
2. `core_tables`: `tenants`, `users`, `documents`, `document_embeddings`. `documents` has
   `UNIQUE (tenant_id, id)` and embeddings reference `(tenant_id, document_id)`, so an embedding
   can never point at another tenant's document. `embedding` is `vector(768)` (nomic-embed-text, Phase 4).
3. `app_grants`: table privileges for `search_app`.

## Endpoints

- `GET /healthz`: process is up.
- `GET /readyz`: database reachable and `vector` + `pg_search` installed; 503 otherwise.

## Configuration (environment)

`DATABASE_URL` (required, app role), `HTTP_PORT` (8080), `LOG_LEVEL` (info), `DB_MAX_CONNS` (10),
`DB_MIN_CONNS` (1), `DB_MAX_CONN_LIFETIME` (30m), `DB_HEALTH_CHECK_PERIOD` (1m).
