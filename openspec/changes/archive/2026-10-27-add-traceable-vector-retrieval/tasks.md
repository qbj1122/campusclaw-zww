# Tasks

## 1. Chunking

- [x] 1.1 Implement `auto`, `custom`, and `hierarchy` normalization with the 800-character default window, 10% default overlap, and custom 100–2000 / half-window clamp; verify: index custom `300/50` and `50/0` fixtures and check the stored chunk lengths and effective strategy.
- [x] 1.2 Implement separator-aware window splitting with trimmed start offsets and section-preserving Markdown heading splits; verify: a hierarchy fixture yields one chunk per section and each chunk starts with its heading.
- [x] 1.3 Apply `strip_urls` and `collapse_whitespace` only to the custom working copy; verify: `knowledge_entries.body_text` is byte-identical after indexing and the downloadable original still contains stripped URLs.

## 2. Durable chunk storage

- [x] 2.1 Create `knowledge_chunks` with class/material/entry foreign keys, unique `(knowledge_entry_id, chunk_index)`, offsets, strategy, and status, and create `knowledge_index_state`; verify: repeated startup keeps both tables and constraints.
- [x] 2.2 Replace an entry's chunks transactionally before inserting the new pending set; verify: a reindex leaves no old `chunk_index`/id row and all new rows use the requested strategy.
- [x] 2.3 Add `ft_chunk_text` with the MySQL 8 ngram parser and a plain FULLTEXT/`LIKE` fallback; verify: `information_schema.statistics` contains the index or keyword search still returns a hit when ngram creation fails.
- [x] 2.4 Hydrate search hits only from `index_status='ready'` rows scoped by `class_id`; verify: a pending or failed chunk never appears in results and a class-B chunk id cannot be hydrated by class A.

## 3. Vector indexing

- [x] 3.1 Ensure the Qdrant collection with cosine distance and `EMBEDDING_DIM`, failing fast on a dimension mismatch; verify: a mismatched existing collection produces an indexing error instead of silent writes.
- [x] 3.2 Upsert one point per chunk with `id=chunk_id` and class/material/entry/chunk payload; verify: a Qdrant scroll for the entry returns exactly the MySQL chunk ids and class id.
- [x] 3.3 Delete the entry's points before a replacement index and mark new rows ready only after a successful upsert; verify: old point ids are absent after reindex and a Qdrant failure returns `503 unavailable`.
- [x] 3.4 Implement the deterministic hash embedding fallback when no embedding key is configured; verify: indexing and search work offline and `docs/API.md` describes vector mode without claiming a configured external model.

## 4. Search and fusion

- [x] 4.1 Implement keyword, vector, and hybrid retrieval with default `hybrid`, input validation, `limit` capped by `SEARCH_MAX_RESULTS`, and post-fusion pagination; verify: blank query/unsupported mode/negative values return `400`, and `limit=100` returns at most 20 hits.
- [x] 4.2 Implement RRF at `k=60`, summing both rank contributions per chunk and ordering ties by ascending chunk id; verify: a dual-ranked chunk scores `1/61 + 1/62` and ranks ahead of a rank-1-only chunk.
- [x] 4.3 Apply `SEARCH_VECTOR_MIN` (default `0.35`) before fusion, with the offline `0.12` + shared-bigram guard; verify: a `0.349` candidate is absent online and unrelated offline text is not returned.
- [x] 4.4 Return `status=no_evidence` and an empty array when no hit survives, while an out-of-range offset on a non-empty ranking returns `200` with the pre-pagination status and `hits=[]`; verify: both cases over HTTP.

## 5. Citations, class isolation, and reindex API

- [x] 5.1 Build 1-based citations with material title, chunk id, and chunk index, and emit `[n]` evidence text shared by ask and streaming; verify: `POST /api/ask` returns `citations[0].index=1` and all `[n]` in the answer are within range.
- [x] 5.2 Return the fixed `资料中未找到相关内容` answer and no citations when ask has no evidence, without constructing a model request; verify: a recording stub observes zero chat completions.
- [x] 5.3 Scope keyword, vector, and citation queries by session class; verify: `student_a1` querying `量子纠缠草莓` gets `200 status=no_evidence hits=[]` while `student_b1` gets the B-class note.
- [x] 5.4 Expose teacher reindex over HTTP with class-scoped material lookup and `{chunks,strategy}` response; verify: a cross-class id returns `404`, a successful strategy change removes old chunk ids/vectors, and `docs/API.md` documents all three search modes.
