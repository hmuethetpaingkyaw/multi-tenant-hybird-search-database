-- Tenant isolation. The API sets app.tenant_id per transaction (set_config(..., true));
-- an unset/empty value yields NULL, so every predicate below matches nothing (fail closed).
CREATE FUNCTION app_current_tenant() RETURNS uuid
    LANGUAGE sql STABLE
    AS $$ SELECT NULLIF(current_setting('app.tenant_id', true), '')::uuid $$;

-- tenants: read-only (own row) for the app. Not FORCE'd: the owner role creates tenant rows (seed).
REVOKE INSERT, UPDATE, DELETE ON tenants FROM search_app;
ALTER TABLE tenants ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenants_select ON tenants FOR SELECT USING (id = app_current_tenant());

-- Tenant-owned tables: ENABLE + FORCE so even the table owner is subject to policies.
-- Policies have no TO clause, so they apply to every role.
ALTER TABLE users               ENABLE ROW LEVEL SECURITY;
ALTER TABLE users               FORCE  ROW LEVEL SECURITY;
ALTER TABLE documents           ENABLE ROW LEVEL SECURITY;
ALTER TABLE documents           FORCE  ROW LEVEL SECURITY;
ALTER TABLE document_embeddings ENABLE ROW LEVEL SECURITY;
ALTER TABLE document_embeddings FORCE  ROW LEVEL SECURITY;

CREATE POLICY users_select ON users FOR SELECT USING (tenant_id = app_current_tenant());
CREATE POLICY users_insert ON users FOR INSERT WITH CHECK (tenant_id = app_current_tenant());
CREATE POLICY users_update ON users FOR UPDATE USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());
CREATE POLICY users_delete ON users FOR DELETE USING (tenant_id = app_current_tenant());

CREATE POLICY documents_select ON documents FOR SELECT USING (tenant_id = app_current_tenant());
CREATE POLICY documents_insert ON documents FOR INSERT WITH CHECK (tenant_id = app_current_tenant());
CREATE POLICY documents_update ON documents FOR UPDATE USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());
CREATE POLICY documents_delete ON documents FOR DELETE USING (tenant_id = app_current_tenant());

CREATE POLICY document_embeddings_select ON document_embeddings FOR SELECT USING (tenant_id = app_current_tenant());
CREATE POLICY document_embeddings_insert ON document_embeddings FOR INSERT WITH CHECK (tenant_id = app_current_tenant());
CREATE POLICY document_embeddings_update ON document_embeddings FOR UPDATE USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());
CREATE POLICY document_embeddings_delete ON document_embeddings FOR DELETE USING (tenant_id = app_current_tenant());
