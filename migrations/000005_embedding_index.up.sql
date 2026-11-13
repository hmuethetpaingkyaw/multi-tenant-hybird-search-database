-- Approximate nearest-neighbour index for cosine similarity (queries use `embedding <=> $1`).
-- One global index; per-tenant filtering is done by RLS, so searches enable hnsw.iterative_scan
-- (see search repository) to avoid short result sets for small tenants.
CREATE INDEX document_embeddings_embedding_hnsw_idx
    ON document_embeddings USING hnsw (embedding vector_cosine_ops);
