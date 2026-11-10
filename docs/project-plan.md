# Project Plan: Multi-Tenant Hybrid Search Database

## 1. Project Overview

### Objective

Build a multi-tenant search service using PostgreSQL that allows multiple customers (tenants) to store, manage, and search their own documents securely.

The system will combine three major capabilities:

1. **Row-Level Security (RLS):** Enforce tenant-level data isolation at the database level.
2. **Vector Search (pgvector):** Perform semantic similarity searches using text embeddings.
3. **BM25 Search:** Perform traditional keyword-based document searches.

The system will combine BM25 and vector search results using Reciprocal Rank Fusion (RRF) to provide more relevant search results.

### Primary Goals

- Support multiple tenants using a shared PostgreSQL database.
- Guarantee that tenants cannot access other tenants' data.
- Support document CRUD operations.
- Support keyword and semantic search.
- Combine search results using hybrid ranking.
- Provide a REST API for integration with other applications.
- Include automated security, integration, and performance tests.

---

## 2. Technology Stack

| Component | Technology |
|---|---|
| Backend | Go |
| HTTP Framework | Gin |
| Database | PostgreSQL |
| Database Driver | pgx |
| Database Migration | golang-migrate |
| Vector Search | pgvector |
| Keyword Search | ParadeDB pg_search (BM25) |
| Ranking Algorithm | Reciprocal Rank Fusion |
| Embedding Model | OpenAI Embeddings API |
| Containerization | Docker / Docker Compose |
| Testing | Go testing + Testcontainers |

Note: Use a configurable embedding dimension based on the selected embedding model. Verify PostgreSQL and extension version compatibility before finalizing the Docker image.

---

## 3. System Architecture

### High-Level Architecture

User → Backend API → Authentication → Tenant Context → PostgreSQL

The backend will execute two search operations:

- BM25 keyword search.
- Vector similarity search.

The results will be combined through RRF before returning the final response.

### Main Components

**1. API Layer**
- Handle HTTP requests.
- Validate input.
- Authenticate users.
- Identify tenant ownership.
- Return standardized responses.

**2. Tenant Isolation Layer**
- Extract tenant identity from authenticated user context.
- Set tenant context inside database transactions.
- Ensure all database operations are protected by RLS.

**3. Document Management**
- Create documents.
- Retrieve documents.
- Update documents.
- Delete documents.
- Generate and maintain document embeddings.

**4. Search Engine**
- Generate query embeddings.
- Execute BM25 search.
- Execute vector similarity search.
- Combine search results.
- Return ranked results.

**5. Database Layer**
- PostgreSQL connection pooling.
- Transaction management.
- Database migrations.
- RLS policies.
- Search indexes.

---

## 4. Database Design

### Tables

#### A. tenants

Stores customer information.

Fields:
- id (UUID, Primary Key)
- name (VARCHAR)
- created_at (TIMESTAMPTZ)

#### B. users

Stores users belonging to tenants.

Fields:
- id (UUID, Primary Key)
- tenant_id (UUID, Foreign Key)
- email (VARCHAR)
- created_at (TIMESTAMPTZ)

#### C. documents

Stores tenant-specific documents.

Fields:
- id (UUID, Primary Key)
- tenant_id (UUID, Foreign Key)
- title (TEXT)
- content (TEXT)
- created_at (TIMESTAMPTZ)
- updated_at (TIMESTAMPTZ)

#### D. document_embeddings

Stores vector representations of documents or document chunks.

Fields:
- id (UUID, Primary Key)
- tenant_id (UUID)
- document_id (UUID)
- chunk_index (INTEGER)
- content (TEXT)
- embedding (VECTOR)
- created_at (TIMESTAMPTZ)

Important database constraints:
- Use composite foreign keys where appropriate to ensure document and embedding tenant IDs match.
- Add indexes on tenant_id and document_id.
- Add vector indexes for similarity search.
- Add BM25 indexes for keyword search.
- Enable RLS on every tenant-owned table.

---

## 5. Security Requirements

Tenant isolation is the most important requirement.

### RLS Implementation

1. Enable RLS on tenant-owned tables.
2. Create policies for SELECT, INSERT, UPDATE, and DELETE.
3. Set tenant context using PostgreSQL transaction-local settings.
4. Ensure the tenant ID comes from authenticated backend context, not directly from client input.
5. Use SET LOCAL or set_config(..., true) inside transactions.
6. Ensure connection pooling cannot leak tenant context between requests.
7. Use a restricted application database role without BYPASSRLS or superuser privileges.
8. Consider FORCE ROW LEVEL SECURITY.

### Security Acceptance Criteria

- Tenant A cannot read Tenant B's documents.
- Tenant A cannot update Tenant B's documents.
- Tenant A cannot delete Tenant B's documents.
- Tenant A cannot access Tenant B's embeddings.
- Missing tenant context must not expose data.
- Cross-tenant document and embedding relationships must be rejected.
- Concurrent requests from different tenants must remain isolated.

---

## 6. Search Implementation

### A. BM25 Search

Implement keyword-based search using ParadeDB's pg_search extension.

Requirements:
- Create BM25 indexes.
- Support searching document titles and content.
- Return document IDs and relevance scores.
- Apply tenant isolation.
- Support pagination and configurable result limits.

### B. Semantic Search

Implement vector similarity search using pgvector.

Workflow:

1. Receive the search query.
2. Generate an embedding using the configured embedding provider.
3. Search document embeddings using cosine similarity or another selected distance metric.
4. Apply tenant filtering.
5. Return the top K matching documents.

Requirements:
- Store embeddings with the correct model and dimension.
- Support configurable result limits.
- Create appropriate vector indexes.
- Avoid returning duplicate documents when multiple chunks match.

### C. Hybrid Search

Combine BM25 and semantic search.

Workflow:

1. Execute BM25 search.
2. Execute semantic search.
3. Retrieve ranked result lists.
4. Merge results using Reciprocal Rank Fusion.
5. Remove duplicate document IDs.
6. Return the final ranked results.

RRF formula:

RRF(d) = Σ 1 / (k + rank(d))

Initial configuration:
- k = 60.
- BM25 top K = 20.
- Vector top K = 20.
- Final results = 10.

These values should be configurable.

---

## 7. API Design

Base URL: `/api/v1`

### Tenant / Document Management

| Method | Endpoint | Description |
|---|---|---|
| POST | /documents | Create document |
| GET | /documents | List tenant documents |
| GET | /documents/:id | Retrieve document |
| PUT | /documents/:id | Update document |
| DELETE | /documents/:id | Delete document |

### Search

| Method | Endpoint | Description |
|---|---|---|
| POST | /search | Hybrid search |
| POST | /search/keyword | BM25 search |
| POST | /search/semantic | Vector search |

### Example Hybrid Search Request

POST `/api/v1/search`

Request:

```json
{
  "query": "How can I recover my account?",
  "limit": 10
}
```

Response:

```json
{
  "results": [
    {
      "document_id": "uuid",
      "title": "Account Recovery Guide",
      "score": 0.0325
    }
  ],
  "total": 1
}
```

Tenant identity must be derived from authentication, not accepted as an unrestricted request parameter.

---

## 8. Implementation Phases

### Phase 1: Project Setup and Database Foundation

Tasks:
- Initialize Go project.
- Set up Gin and pgx.
- Configure Docker Compose.
- Start PostgreSQL.
- Install pgvector and pg_search.
- Configure database migrations.
- Create initial database schema.
- Configure connection pooling.

Deliverables:
- Running backend.
- Running PostgreSQL environment.
- Initial database migrations.

Acceptance criteria:
- Application connects to PostgreSQL.
- Database migrations execute successfully.
- Required extensions are installed and verified.

### Phase 2: Multi-Tenant Architecture and RLS

Tasks:
- Implement tenant and user models.
- Implement authentication context (using a development authentication mechanism initially).
- Implement tenant identification middleware.
- Implement transaction-level tenant context.
- Create RLS policies.
- Test all CRUD operations across tenants.

Deliverables:
- Tenant-aware database access.
- RLS policies.
- Security integration tests.

Acceptance criteria:
- Cross-tenant access is blocked at the database level.
- Connection pooling does not cause tenant context leakage.

### Phase 3: Document Management

Tasks:
- Implement document repository.
- Implement document CRUD use cases.
- Implement HTTP handlers.
- Validate document ownership.
- Add database integration tests.

Deliverables:
- Complete document management API.

Acceptance criteria:
- Tenants can manage their own documents.
- Tenants cannot modify other tenants' documents.

### Phase 4: Embedding Pipeline and Vector Search

Tasks:
- Integrate embedding provider.
- Implement embedding generation.
- Store document embeddings.
- Implement vector similarity queries.
- Create vector indexes.
- Implement semantic search API.

Deliverables:
- Working embedding pipeline.
- Semantic search endpoint.

Acceptance criteria:
- Documents can be embedded and stored.
- Search returns semantically relevant documents.
- Search results respect RLS.

### Phase 5: BM25 and Hybrid Search

Tasks:
- Configure BM25 indexing.
- Implement keyword search.
- Implement Reciprocal Rank Fusion.
- Merge and deduplicate results.
- Implement hybrid search endpoint.
- Add ranking tests.

Deliverables:
- BM25 search endpoint.
- Hybrid search endpoint.
- RRF ranking implementation.

Acceptance criteria:
- Keyword search works.
- Semantic search works.
- Hybrid search combines both result lists.
- Results remain tenant-isolated.

### Phase 6: Testing and Production Readiness

Tasks:
- Unit tests.
- Database integration tests.
- Tenant isolation tests.
- Concurrent request tests.
- Search relevance tests.
- Query performance analysis.
- Docker deployment configuration.
- Logging and error handling.
- API documentation.

Deliverables:
- Complete test suite.
- Dockerized application.
- API documentation.
- Performance test results.

Acceptance criteria:
- All tests pass.
- No cross-tenant data leakage.
- Search queries use appropriate indexes.
- Application can run using Docker Compose.

---

## 9. Testing Strategy

Testing must cover three major areas.

### A. Functional Testing

- Create documents.
- Update documents.
- Delete documents.
- Search by exact keywords.
- Search by semantic meaning.
- Search using hybrid ranking.

### B. Security Testing

Create at least three tenants: A, B, and C.

Test:
- Tenant A searches Tenant B's document.
- Tenant B tries to retrieve Tenant C's document ID.
- Tenant A attempts to update Tenant B's document.
- Requests without tenant context.
- Concurrent requests from multiple tenants.
- Database connection reuse between tenants.

### C. Performance Testing

Measure:
- Document insertion time.
- Embedding generation latency.
- BM25 query latency.
- Vector query latency.
- Hybrid search latency.
- Database connection pool usage.

Use different dataset sizes to understand how performance changes as document volume grows.

---

## 10. Project Scope

### MVP – Must Have

- Multi-tenant database.
- PostgreSQL RLS.
- Document CRUD.
- Embedding generation.
- pgvector semantic search.
- BM25 keyword search.
- RRF hybrid ranking.
- Tenant isolation tests.
- Docker Compose.

### Future Improvements

- Document chunking strategies.
- Background embedding processing.
- Asynchronous document ingestion.
- Multiple embedding providers.
- Search relevance tuning.
- Advanced filtering.
- Rate limiting.
- Observability with Prometheus and Grafana.
- Search caching.
- Tenant-specific search configuration.

Avoid implementing these future improvements until the core MVP works correctly.

---

## 11. Collaboration and Development Rules

This project will be developed with an AI coding agent.

Follow these rules:

1. Implement one phase at a time.
2. Read and understand the architecture before generating code.
3. Do not implement the entire project in one large generation.
4. Explain important design decisions before implementation.
5. Keep business logic separate from HTTP handlers and database access.
6. Do not bypass RLS by adding application-only tenant filters.
7. Write tests alongside each feature.
8. Do not proceed to the next phase until the current phase passes its acceptance criteria.
9. Document database migrations and important architectural decisions.
10. Review generated code for security issues, race conditions, transaction handling, and error handling.

### Expected Project Structure

```
cmd/
  api/
    main.go

internal/
  domain/
    tenant/
    document/
    search/
  application/
    tenant/
    document/
    search/
  infrastructure/
    database/
    embedding/
    search/
  delivery/
    http/
    middleware/

migrations/
tests/
docker-compose.yml
Dockerfile
README.md
```

The exact structure can be adjusted as the architecture evolves.

---

## 12. Final Project Deliverables

At project completion, the repository should contain:

1. A working Go backend.
2. PostgreSQL database with RLS enabled.
3. Multi-tenant document management.
4. pgvector semantic search.
5. BM25 keyword search.
6. RRF hybrid ranking.
7. REST API documentation.
8. Automated functional and security tests.
9. Docker Compose environment.
10. README explaining architecture, setup, and execution.

**Definition of Done:**

A tenant can upload documents, search them using natural language or exact keywords, and receive ranked results without ever accessing another tenant's data. All tenant isolation must be enforced and verified at the PostgreSQL level.
