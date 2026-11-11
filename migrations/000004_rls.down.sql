DROP POLICY IF EXISTS document_embeddings_delete ON document_embeddings;
DROP POLICY IF EXISTS document_embeddings_update ON document_embeddings;
DROP POLICY IF EXISTS document_embeddings_insert ON document_embeddings;
DROP POLICY IF EXISTS document_embeddings_select ON document_embeddings;
DROP POLICY IF EXISTS documents_delete ON documents;
DROP POLICY IF EXISTS documents_update ON documents;
DROP POLICY IF EXISTS documents_insert ON documents;
DROP POLICY IF EXISTS documents_select ON documents;
DROP POLICY IF EXISTS users_delete ON users;
DROP POLICY IF EXISTS users_update ON users;
DROP POLICY IF EXISTS users_insert ON users;
DROP POLICY IF EXISTS users_select ON users;
DROP POLICY IF EXISTS tenants_select ON tenants;

ALTER TABLE document_embeddings NO FORCE ROW LEVEL SECURITY;
ALTER TABLE document_embeddings DISABLE ROW LEVEL SECURITY;
ALTER TABLE documents           NO FORCE ROW LEVEL SECURITY;
ALTER TABLE documents           DISABLE ROW LEVEL SECURITY;
ALTER TABLE users               NO FORCE ROW LEVEL SECURITY;
ALTER TABLE users               DISABLE ROW LEVEL SECURITY;
ALTER TABLE tenants             DISABLE ROW LEVEL SECURITY;

GRANT INSERT, UPDATE, DELETE ON tenants TO search_app;
DROP FUNCTION IF EXISTS app_current_tenant();
