CREATE TABLE tenants (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name       varchar(255) NOT NULL,
    created_at timestamptz  NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid         NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    email      varchar(320) NOT NULL,
    created_at timestamptz  NOT NULL DEFAULT now(),
    -- Leading tenant_id column also serves as the tenant_id index.
    UNIQUE (tenant_id, email)
);

CREATE TABLE documents (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    title      text        NOT NULL,
    content    text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    -- Target of the composite FK below; its leading tenant_id also serves as the tenant_id index.
    UNIQUE (tenant_id, id)
);

-- vector(768) matches nomic-embed-text (Phase 4). Changing models means a new migration + re-embed.
CREATE TABLE document_embeddings (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid         NOT NULL,
    document_id uuid         NOT NULL,
    chunk_index integer      NOT NULL DEFAULT 0,
    content     text         NOT NULL,
    model       text         NOT NULL,
    embedding   vector(768)  NOT NULL,
    created_at  timestamptz  NOT NULL DEFAULT now(),
    -- Rejects an embedding whose tenant differs from its document's tenant.
    FOREIGN KEY (tenant_id, document_id) REFERENCES documents (tenant_id, id) ON DELETE CASCADE,
    -- Leading document_id column also serves as the document_id index.
    UNIQUE (document_id, chunk_index)
);

CREATE INDEX document_embeddings_tenant_id_idx ON document_embeddings (tenant_id);
