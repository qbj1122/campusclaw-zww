# traceable-retrieval Specification

## Purpose
Turn class material text into traceable chunks and ranked evidence so search and answers can cite the exact class material and chunk they used.

## Requirements

### Requirement: Chunking supports auto, custom, and hierarchy strategies

Indexing SHALL accept `auto`, `custom`, and `hierarchy`, default an empty strategy to `auto`, and record the effective value in `knowledge_chunks.chunk_strategy`. `auto` and `hierarchy` SHALL fall back to an 800-character window and a 10% overlap when unset or invalid; `custom` SHALL clamp the window to 100–2000 characters and overlap to at most half the window. `hierarchy` SHALL keep Markdown headings with their section.

#### Scenario: Default upload uses auto chunks
- **WHEN** a teacher uploads a long `.md` file without a `strategy` field
- **THEN** every new `knowledge_chunks` row for the material has `chunk_strategy=auto`, contiguous `chunk_index` values starting at 0, and a non-empty `chunk_text` of at most 800 runes

#### Scenario: Hierarchy chunks start with their heading
- **WHEN** a teacher uploads Markdown containing `# 总标题`, `## 第一节`, and `## 第二节` using `strategy=hierarchy`
- **THEN** the resulting chunks are split at the headings and each chunk's first non-empty line is the heading that introduced that section

#### Scenario: Custom bounds are applied
- **WHEN** a teacher uploads with `strategy=custom`, `chunk_max_chars=300`, and `chunk_overlap=50`
- **THEN** every chunk row has `chunk_strategy=custom` and `chunk_text` of at most 300 runes

### Requirement: MySQL stores authoritative chunk rows with full-text search

The system SHALL persist each chunk in `knowledge_chunks` with `class_id`, `material_id`, `knowledge_entry_id`, `chunk_index`, `chunk_text`, `start_offset`, `end_offset`, `index_status`, and `chunk_strategy`. It SHALL provide a FULLTEXT index `ft_chunk_text` using the MySQL 8 ngram parser when available, and keyword search SHALL use `MATCH ... AGAINST` with a LIKE fallback when that parser is unavailable.

#### Scenario: Indexed chunks are queryable rows
- **WHEN** a material is indexed successfully
- **THEN** `knowledge_chunks` contains one row per returned chunk with the material's class id and entry id, and `information_schema.statistics` contains an index named `ft_chunk_text` on `chunk_text`

#### Scenario: Keyword mode returns the matching chunk
- **WHEN** a class-A user posts `{"q":"去括号","mode":"keyword"}` to `POST /api/search`
- **THEN** the response is `200` with `mode=keyword` and at least one hit whose `material_title` and `chunk_index` identify a class-A chunk containing `去括号`

#### Scenario: Missing ngram parser still returns keyword evidence
- **WHEN** the database cannot create the ngram FULLTEXT index and a keyword query matches chunk text
- **THEN** migration continues with a plain FULLTEXT or LIKE path, and `POST /api/search` still returns a successful keyword hit instead of a server error

### Requirement: Qdrant vectors are keyed by chunk id

For every ready chunk the system SHALL upsert one Qdrant point whose point id equals the MySQL `knowledge_chunks.id` and whose payload contains `class_id`, `material_id`, `knowledge_entry_id`, `chunk_id`, and `chunk_index`. Vector search SHALL filter by `class_id` in Qdrant.

#### Scenario: Point id matches the chunk row
- **WHEN** a material with three ready chunks is indexed
- **THEN** Qdrant contains exactly three points for the entry, each id equals the corresponding `knowledge_chunks.id`, and each payload's `class_id` equals the material's class id

#### Scenario: Vector outage fails reindex visibly
- **WHEN** a teacher calls `POST /api/materials/{id}/reindex` while Qdrant is unreachable
- **THEN** the response is `503` with `error.code=unavailable`, and the API does not report a successful new chunk set

### Requirement: Search supports keyword, vector, and hybrid modes

`POST /api/search` SHALL accept `q`, `mode` (`keyword`, `vector`, or `hybrid`, default `hybrid`), `limit`, and `offset`. It SHALL reject a blank query, an unsupported mode, or negative pagination with HTTP 400, cap `limit` at `SEARCH_MAX_RESULTS` (default 20), and apply pagination only after ranking.

#### Scenario: Hybrid is the default
- **WHEN** a class-A user posts `{"q":"一元一次方程"}` without `mode`
- **THEN** the response is `200` with `mode=hybrid`, `status` either `ok` or `no_evidence`, and `hits` always an array

#### Scenario: Invalid search input is rejected
- **WHEN** a user posts blank `q`, `mode=magic`, or `limit=-1` to `POST /api/search`
- **THEN** the response is `400` with `error.code=bad_request`, not an empty result set

#### Scenario: Limit is capped and offset is post-fusion
- **WHEN** a user posts a matching query with `limit=100` or with an `offset` beyond the ranked hits
- **THEN** the effective limit is 20, and the out-of-range offset returns `200` with an empty `hits` array while preserving the pre-pagination status

#### Scenario: Vector mode returns ranked similarity hits
- **WHEN** a user posts `{"q":"通分","mode":"vector"}` with a reachable vector store
- **THEN** each hit has `rank` starting at 1 in descending similarity order and retains its similarity in `score`, and no keyword contribution is added

### Requirement: Hybrid fusion uses RRF k=60 and a vector cutoff

Hybrid mode SHALL assign `1/(60+rank)` to each keyword or vector ranking position, sum both contributions for the same chunk, and order by descending fused score then ascending `chunk_id`. Vector candidates below `SEARCH_VECTOR_MIN` (default `0.35`) SHALL be discarded; the deterministic offline provider SHALL use `0.12` and additionally require a shared character bigram.

#### Scenario: A chunk found by both modes accumulates both scores
- **WHEN** the same chunk is keyword rank 1 and vector rank 2 in a hybrid query
- **THEN** its `score` equals `1/61 + 1/62` within floating-point tolerance, `keyword_rank=1`, `vector_rank=2`, and it ranks ahead of a chunk found at only one rank 1 position

#### Scenario: Low-similarity vectors are excluded
- **WHEN** a vector search returns a candidate with similarity `0.349` and `SEARCH_VECTOR_MIN=0.35`
- **THEN** that chunk is absent from `hits` and cannot contribute to a hybrid score

#### Scenario: All candidates filtered becomes no evidence
- **WHEN** every vector candidate is below the effective cutoff
- **THEN** `POST /api/search` returns `200` with `status=no_evidence` and `hits=[]`

#### Scenario: Offline embeddings require a shared bigram
- **WHEN** no embedding key is configured and a query shares no character bigram with a stored chunk
- **THEN** the offline vector path discards that chunk even if its hash similarity exceeds the lowered `0.12` cutoff

### Requirement: Ask returns citations with material title and chunk index

`POST /api/ask` SHALL answer only from class-scoped retrieved chunks. Each citation SHALL contain a 1-based `index`, `material_id`, `material_title`, `chunk_id`, and `chunk_index`; citation numbering in the answer SHALL match this array. With no evidence it SHALL return the fixed answer `资料中未找到相关内容`, an empty citation array, and SHALL NOT call the model.

#### Scenario: Evidence produces numbered traceable citations
- **WHEN** a class-A student asks about `去括号`
- **THEN** the response is `200` with `status=ok`, `citations[0].index=1`, a non-empty `citations[0].material_title`, a valid `citations[0].chunk_index`, and every `[n]` in `answer` is at most `len(citations)`

#### Scenario: Unrelated question makes no model call
- **WHEN** a user asks a question with no matching class evidence and the model gateway is a stub that records requests
- **THEN** the response is `200` with `status=no_evidence`, `answer=资料中未找到相关内容`, `citations=[]`, and the stub recorded zero chat completion requests

### Requirement: Retrieval is class-scoped and cross-class search is not an authorization error

Every keyword, vector, and citation lookup SHALL use the authenticated user's class id. A query for content that exists only in another class SHALL return a successful empty result, not HTTP 403, and SHALL expose no other-class title, excerpt, id, or score.

#### Scenario: Class-A user cannot retrieve the class-B marker
- **WHEN** `student_a1` posts `{"q":"量子纠缠草莓","mode":"hybrid"}` to `POST /api/search`
- **THEN** the response is `200` with `status=no_evidence`, `hits=[]`, and no mention of `B班物理笔记`

#### Scenario: The owning class can retrieve the marker
- **WHEN** `student_b1` posts the same query
- **THEN** the response is `200` with `status=ok` and a hit whose `material_title` is `B班物理笔记`

#### Scenario: Missing authentication is still an error
- **WHEN** a client calls `POST /api/search` without a Bearer token
- **THEN** the response is `401` with `error.code=unauthorized`, not an empty hit list

### Requirement: Reindex replaces old chunks and vectors

A successful reindex SHALL delete the entry's existing Qdrant points and MySQL chunks before inserting the newly chunked set, then mark the new rows ready. A reindex for another class or a missing material SHALL return 404.

#### Scenario: Reindex changes strategy and removes old chunk ids
- **WHEN** a teacher reindexes an `auto` material with `{"strategy":"custom","chunk_max_chars":300}` and the request succeeds
- **THEN** the response is `200` with `strategy=custom` and a chunk count, no pre-reindex chunk id remains in `knowledge_chunks` or Qdrant, and all new rows have `chunk_strategy=custom`

#### Scenario: Cross-class reindex is hidden
- **WHEN** `teacher_a` calls `POST /api/materials/{class_b_material_id}/reindex`
- **THEN** the response is `404` with `error.code=not_found` and class-B chunks and vectors are unchanged
