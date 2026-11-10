-- Normally already installed by docker/postgres/init/01-roles.sh (needs superuser).
CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS pg_search;
