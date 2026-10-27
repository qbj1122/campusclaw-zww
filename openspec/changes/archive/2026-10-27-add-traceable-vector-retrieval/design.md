# Design

## Context

See `proposal.md` and `specs/traceable-retrieval/spec.md`. Lesson 2–3 already stores each material's text in `knowledge_entries.body_text`; this change derives chunks from that durable copy. MySQL 8 is the system of record, Qdrant is a derived index, and the chat/embedding gateway may be absent. The existing offline provider returns a deterministic character-bigram hash embedding and a deterministic answer, so retrieval must remain functional without claiming semantic quality.

## Goals / Non-Goals

**Goals:**

- Reproducible chunking with stored offsets and strategy.
- One authoritative chunk identity shared by MySQL and Qdrant.
- Explainable ranking: keyword score, vector similarity, or RRF contributions.
- Citations precise enough to reopen the source material at a chunk index.
- Deterministic no-evidence behavior.

**Non-Goals:**

- Cross-encoder reranking, semantic query expansion, or hybrid weights tuned per class.
- Real-time synchronous consistency between MySQL and Qdrant during a failure.
- Solving Chinese word segmentation beyond MySQL ngram and the LIKE fallback.
- Evaluating the offline hash embedding as a production semantic model.

## Decisions

### Chunking is a pure function with normalized options

`chunk.Split` receives the immutable `body_text` plus options. `Normalize` supplies the default window/overlap and clamps custom values. `auto` uses a separator-aware sliding window (`\n\n`, line breaks, Chinese/English sentence ends) and trims each segment while adjusting the start offset. `hierarchy` first groups lines by `#`–`###` headings and only window-splits a section that exceeds the limit. URL stripping and whitespace collapsing apply only to the working copy for `custom`; `body_text` and the original file remain unchanged.

Alternative considered: store cleaned text and offsets into it. That would make citations disagree with the downloadable original.

### MySQL holds chunk truth; Qdrant holds only vectors

`ReplaceChunks` runs in a MySQL transaction, deletes the entry's old rows, inserts every new chunk as `pending`, and returns their ids. The indexer embeds each chunk, upserts points with `id = chunk_id`, marks rows `ready`, and records a body/settings hash in `knowledge_index_state`. Search always hydrates hits from MySQL with `class_id` and `index_status='ready'`, so a stale or forged Qdrant payload cannot expose another class's text.

The hash short-circuit avoids re-embedding unchanged entries at startup. Reindex clears the hash first, which forces a full replacement.

### RRF over raw scores in hybrid mode

Keyword and vector scores are not comparable, so hybrid uses unweighted reciprocal rank fusion with `k=60`: each contributing rank position adds `1/(60+rank)`, then results sort by score and finally chunk id. Keyword and vector-only modes keep their native scores and assign `rank` after ordering. Vector candidates are filtered by `SEARCH_VECTOR_MIN` before fusion; offline mode lowers the cutoff to `0.12` and requires a shared character bigram because hash collisions otherwise create false positives.

Alternatives considered: weighted score normalization needs per-corpus calibration and can let one retriever dominate; concatenating ranked lists loses the "found by both" signal.

### Evidence construction is shared by ask and streaming

`BuildEvidence` converts hydrated hits into 1-based citations and `[n] 材料《title》切片 index` evidence strings, truncating each evidence body to 800 runes. `POST /api/ask` retrieves hybrid top-4, and if no citation survives, returns before constructing any model request. This makes the no-evidence path a control-flow guarantee rather than a prompt instruction.

### Reindex is a replace, not an append

A request first verifies class-scoped material access, clears the index-state row, deletes Qdrant points filtered by `knowledge_entry_id`, replaces MySQL chunks, and upserts the new vectors. Failure is reported as 503; the API does not claim a successful replacement. Because the stores are not transactionally coupled, a mid-flight failure can leave the entry requiring another reindex, but it cannot make old chunk text appear under a new chunk id.

### Cross-class queries return empty, not forbidden

Class filtering happens inside both MySQL (`kc.class_id=?`) and Qdrant (`payload.class_id`). Search is a normal read for any authenticated class member, so asking about another class's content is simply no evidence. HTTP 403 is reserved for role violations, not data absence.

## Risks / Trade-offs

- **Dual-write inconsistency between MySQL and Qdrant** → MySQL is authoritative; search hydrates by id and ignores points without ready rows; reindex repairs.
- **RRF ignores score magnitude** → stable and explainable, but a very strong single-retriever hit does not outrank a weak dual-retriever hit; acceptable for lesson-scale corpora.
- **Cutoff is corpus-dependent** → expose `SEARCH_VECTOR_MIN`; use a lower, bigram-guarded value only for the offline demo.
- **Offline hash embeddings are not semantic** → document the fallback, keep the deterministic path testable, and do not claim model availability.
- **ngram parser may be unavailable** → migrate to plain FULLTEXT, then LIKE; results are less precise but available.
- **Chunk offsets can drift after trimming** → store the adjusted start offset and test boundaries; excerpts are still traceable to the chunk index.
- **Reindex temporarily leaves an entry without vectors** → requests during the window return no evidence rather than stale evidence; retry or reindex.

## Migration

1. Start the API; `db.Migrate` creates `knowledge_chunks` and `knowledge_index_state`, then attempts `ft_chunk_text` with the ngram parser.
2. `EnsureAllIndexed` runs after startup, ensures the Qdrant collection dimension matches `EMBEDDING_DIM`, and indexes entries whose body/settings hash is absent or changed.
3. Existing materials need no schema rewrite because their `knowledge_entries.body_text` is already populated.
4. Rollback keeps MySQL data; the Qdrant collection can be dropped and rebuilt. Changing `EMBEDDING_DIM` against an existing collection fails fast and requires recreating the collection.
