-- RLS policies (Phase 2) are what actually restrict rows; these are table-level privileges.
GRANT SELECT, INSERT, UPDATE, DELETE
    ON tenants, users, documents, document_embeddings TO search_app;
