# Multi-tenant hybrid search database

Go + PostgreSQL service for tenant-isolated document search (RLS, pgvector, BM25).
**Status: Phase 4 (embeddings and semantic search).** Keyword (BM25) and hybrid search arrive in Phase 5; see [docs/project-plan.md](docs/project-plan.md).

## Run

Requires Docker (Compose v2). Optional: Go 1.26 (the toolchain downloads automatically).

```sh
cp .env.example .env     # development defaults; DB_PORT defaults to 5433 to avoid clashes
make up                  # db -> migrations -> Ollama (pulls the embedding model) -> API
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

The first `make up` downloads the Ollama image (about 3 GB) and the embedding model into the `ollama_data` volume;
later runs reuse them. Ollama runs CPU-only inside Docker on macOS, which is fine for an embedding model.

## Pinned versions

| Component | Version | Note |
|---|---|---|
| ParadeDB image | `paradedb/paradedb:0.26.6-pg17` | PostgreSQL 17, pg_search 0.26.6, pgvector 0.8.6 (both verified present) |
| golang-migrate image | `migrate/migrate:v4.18.3` | |
| Ollama image | `ollama/ollama:0.40.16` | serves `nomic-embed-text` (768 dimensions, fixed by the schema) |

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
5. `embedding_index`: HNSW cosine index on `document_embeddings.embedding`.

## Embeddings and semantic search (Phase 4)

- Creating or updating a document **synchronously** embeds it (Ollama, `nomic-embed-text`) before writing, then writes the
  document and its embedding in **one transaction**. The embedding call happens *outside* the transaction so a slow model
  never holds a database connection.
- If the embedding service fails (down, timeout of `EMBEDDING_TIMEOUT` default 30s, model not pulled, wrong dimension),
  the request returns `503 {"error":"embedding service unavailable"}`, nothing is written (updates leave the document and
  its embedding unchanged), and the cause is logged at ERROR. Reads and deletes keep working; `/readyz` is database-only.
- Measured against `ollama/ollama:0.40.16` + `nomic-embed-text`: `POST /api/embed` with `{"model","input":[...]}` returns
  `{"embeddings":[[...768 floats...]], ...}`; a missing model is HTTP 404; input beyond the context window is **silently
  truncated by Ollama to 2048 tokens** (observed `prompt_eval_count: 2048` for a 30k-character input), so dense text
  (code, non-English) can lose its tail before our own 8000-character cut.
- One chunk per document: the text is `title + "\n\n" + content`, cut to the first **8000 characters**. Text beyond that is
  saved but does not influence semantic search. Real chunking and background embedding are planned for a later phase.
- Documents created before Phase 4 have no embedding and are not returned by semantic search until they are re-saved with `PUT`.
- `POST /api/v1/search/semantic` embeds the query and returns the nearest documents by cosine similarity, under RLS.
  There is no similarity cutoff, so an unrelated query still returns the closest documents with low scores.
- Index: one global HNSW index (`vector_cosine_ops`); searches also set `hnsw.iterative_scan` per transaction.
  **Known limitation:** with a single global ANN index and RLS filtering after the scan, a very small tenant surrounded by a
  much larger tenant's nearer vectors can get short or empty results if the planner is forced onto the HNSW graph (reproduced
  in development; iterative scan does not fix that extreme case). For selective tenant filters the planner normally chooses
  the `tenant_id` index and an exact sort, which is correct, and the test suite covers that. Per-tenant partitioning or an
  exact-search fallback are the options if this matters at scale (Phase 6 performance analysis).

## Endpoints

- `GET /healthz`: process is up (no auth).
- `GET /readyz`: database reachable and `vector` + `pg_search` installed; 503 otherwise (no auth).
- `GET /api/v1/me`: the caller's tenant and user. 401 for a missing/invalid token, 404 if the user or tenant is not
  visible under RLS.

### Documents (`/api/v1/documents`, bearer token required)

| Method | Path | Success | Notes |
|---|---|---|---|
| POST | `/documents` | 201 + document | body `{"title","content"}` |
| GET | `/documents` | 200 `{documents, limit, offset}` | newest first; items omit `content`; `?limit=` (1-100, default 20, larger values are clamped) and `?offset=` (default 0) |
| GET | `/documents/:id` | 200 + document | |
| PUT | `/documents/:id` | 200 + document | full replace of title and content |
| DELETE | `/documents/:id` | 204 | also removes the document's embeddings |

Rules: `title` is required, trimmed, at most 500 characters; `content` is required, at most 1 MiB; the body must be JSON
containing only `title` and `content` (anything else, e.g. `tenant_id`, is a 400; the tenant always comes from the token).
The request body is capped at about 1 MiB plus 64 KiB of overhead; note that JSON escaping of the content counts toward it.

Errors are `{"error": "<message>"}`: `401 unauthorized` (bad/missing token), `400` (invalid body, id or paging),
`404 not found` (missing **or another tenant's** document; the two are indistinguishable), `413 request body too large`,
`500 internal error` (details are in the server log, not the response). Concurrent edits are last-write-wins.

```sh
curl -s -X POST localhost:8080/api/v1/documents -H "Authorization: Bearer $TOKEN_A" \
  -H 'Content-Type: application/json' -d '{"title":"Account Recovery Guide","content":"How to recover your account"}'
curl -s localhost:8080/api/v1/documents -H "Authorization: Bearer $TOKEN_A"
curl -s localhost:8080/api/v1/documents/<id> -H "Authorization: Bearer $TOKEN_B"   # 404: not tenant B's document
```

### Semantic search (`POST /api/v1/search/semantic`, bearer token required)

Body `{"query": "...", "limit": 10}` (`limit` optional: default 10, above 100 clamped to 100, below 1 is a 400). `query` is
required, trimmed, at most 2000 characters; unknown fields such as `tenant_id` are a 400; bodies over 16 KiB are a 413.
Response `200 {"results":[{"document_id","title","score"}],"total":N}`, most similar first (`score` is cosine similarity),
`{"results":[],"total":0}` when nothing is embedded. `503 embedding service unavailable` if Ollama fails.

```sh
curl -s -X POST localhost:8080/api/v1/search/semantic -H "Authorization: Bearer $TOKEN_A" \
  -H 'Content-Type: application/json' -d '{"query":"How can I recover my account?","limit":5}'
```

## Configuration (environment)

`DATABASE_URL` (required, app role), `JWT_SECRET` (required, at least 32 bytes), `HTTP_PORT` (8080),
`LOG_LEVEL` (info), `DB_MAX_CONNS` (10), `DB_MIN_CONNS` (1), `DB_MAX_CONN_LIFETIME` (30m),
`DB_HEALTH_CHECK_PERIOD` (1m), `OLLAMA_URL` (`http://localhost:11434`; Compose sets `http://ollama:11434`),
`EMBEDDING_MODEL` (`nomic-embed-text`), `EMBEDDING_TIMEOUT` (30s). `cmd/seed` additionally needs `SEED_DATABASE_URL` (owner role; set by `make seed`).

## Security tests

`go test ./tests/...` starts a ParadeDB container and checks, with tenants A, B, C: cross-tenant read/update/delete
are blocked, embeddings are isolated, missing context returns nothing, `WITH CHECK` blocks writes into other tenants,
the composite FK rejects cross-tenant embeddings, FORCE RLS binds the owner, and pooled connections never carry
tenant context (including 16 goroutines x 50 iterations over a 4-connection pool; run with `-race` for the race check).
