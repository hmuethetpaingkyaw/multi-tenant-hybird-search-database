#!/bin/sh
# Runs once, on first start of an empty data directory, as the bootstrap superuser.
#
# Role model:
#   postgres (bootstrap superuser) - only used here; never by the application.
#   search_owner - owns the schema and tables, runs migrations. NOSUPERUSER, NOBYPASSRLS.
#   search_app   - used by the API. Not an owner, so table RLS (Phase 2) applies to it.
# Extensions need superuser to install, so they are created here; migration 0001
# is then an idempotent safety net.
set -eu

: "${OWNER_PASSWORD:?OWNER_PASSWORD is required}"
: "${APP_PASSWORD:?APP_PASSWORD is required}"

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
  -v owner_pw="$OWNER_PASSWORD" -v app_pw="$APP_PASSWORD" -v db="$POSTGRES_DB" <<'SQL'
CREATE ROLE search_owner LOGIN PASSWORD :'owner_pw'
  NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;
CREATE ROLE search_app LOGIN PASSWORD :'app_pw'
  NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;

CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS pg_search;

REVOKE ALL ON DATABASE :"db" FROM PUBLIC;
GRANT CONNECT ON DATABASE :"db" TO search_owner, search_app;

ALTER SCHEMA public OWNER TO search_owner;
REVOKE ALL ON SCHEMA public FROM PUBLIC;
GRANT USAGE ON SCHEMA public TO search_app;
SQL
