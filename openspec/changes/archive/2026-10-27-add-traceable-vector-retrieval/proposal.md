# Proposal

## Why

Uploaded text is only useful if a student receives an answer that can be traced back to a specific class material and chunk. Lesson 4 replaces ad-hoc search with a repeatable chunk → embed → store → fuse → cite pipeline, while keeping all class boundaries server-side.

## What Changes

- Add three chunking strategies: `auto`, `custom`, and `hierarchy`, with normalized window/overlap bounds and Markdown-heading preservation.
- Store chunks in MySQL `knowledge_chunks` with a MySQL 8 FULLTEXT ngram index; keep `body_text` untouched by preprocessing.
- Store one Qdrant point per chunk, keyed by the MySQL chunk id and carrying `class_id`, `material_id`, and `knowledge_entry_id`.
- Add `keyword`, `vector`, and `hybrid` search; hybrid uses reciprocal rank fusion with `k=60`, and vector candidates below `SEARCH_VECTOR_MIN` (default `0.35`) are discarded.
- Add `POST /api/search` hits with material title, chunk index, offsets, excerpt, score, and ranks.
- Add `POST /api/ask` with citations carrying a 1-based index, material title, and chunk index; a no-evidence result uses a fixed answer and makes no model call.
- Scope all retrieval by the session class; cross-class queries return an empty successful result rather than `403`.
- Reindex deletes the entry's prior chunks and vectors before writing the replacement set.
- Keep a deterministic offline embedding fallback so the pipeline runs without a configured gateway.

## Capabilities

### New Capabilities
- `traceable-retrieval`: chunking, MySQL and Qdrant indexing, keyword/vector/hybrid search with RRF, class-scoped retrieval, citations, no-evidence behavior, and reindex replacement.

### Modified Capabilities
- None.

## Impact

- New tables/indexes: `knowledge_chunks`, `knowledge_index_state`, FULLTEXT index `ft_chunk_text`.
- New Qdrant collection usage: `campusclaw_chunks` with cosine distance and `EMBEDDING_DIM` dimensions.
- New API surface: `POST /api/search`, `POST /api/ask`, and `POST /api/materials/{id}/reindex`.
- New configuration: `CHUNK_MAX_CHARS`, `CHUNK_OVERLAP_CHARS`, `SEARCH_VECTOR_MIN`, `SEARCH_MAX_RESULTS`, `RRF_K`, `QDRANT_URL`, `QDRANT_COLLECTION`, `EMBEDDING_*`.
- Affected code: `internal/chunk`, `internal/service/retrieval.go`, `internal/service/assistant.go`, `internal/store/retrieval.go`, `internal/vector`, `internal/httpapi`.
- External model use is optional: with no chat/embedding key the deterministic hash embedding and offline answer path are used.

## Non-goals

- No cross-encoder reranking, learned sparse retrieval, or query rewriting.
- No multi-vector or late-interaction models.
- No claim that the offline hash embedding is semantically equivalent to a hosted embedding model.
- No per-chunk ACL; all filtering remains class- and (for assistants) material-binding scoped.
