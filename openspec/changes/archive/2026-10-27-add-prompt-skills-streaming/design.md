# Design

## Context

See `proposal.md` and `specs/assistant-streaming/spec.md`. Retrieval already returns class-scoped hits and citations. This change adds configuration and a per-turn orchestration layer. The chat gateway is optional: with no chat key, `generate` streams a deterministic offline answer, so the SSE contract cannot depend on a hosted model being present.

## Goals / Non-Goals

**Goals:**

- Configuration that a teacher can change without a deploy and that students cannot mutate.
- Reproducible routing so the same question and skill set select the same skill.
- Conversation privacy enforced in SQL, not in client filters.
- A stream whose metadata is usable before the first token.
- Evidence as data, not as executable instructions.

**Non-Goals:**

- LLM-based skill selection or multi-skill composition.
- Token-by-token parity across providers; deltas may be arbitrary chunks.
- Server-side resumable streams or event replay.
- Rich message editing, branching, or sharing.

## Decisions

### Skill files are parsed, not executed

`ParseSkillFile` splits on the first `\n\n`, parses `key：value` (also accepting ASCII `:`), rejects unknown or duplicate keys, and requires `名称`, `适用条件`, and `启用`. The guidance becomes `description`; the raw normalized text becomes `source_text`. Limits are checked in runes so Chinese input is measured consistently. Tool entries accept `knowledge_search` or the syntactic form `tool@server`; server connectivity and discovery validation are layered on at save time by the `mcp-tools` capability.

Alternative considered: free-form Markdown with front matter. The lesson's single-file format is easier to paste into a textarea and gives deterministic validation errors.

### Routing is a lexical score over already-scoped candidates

`AgentSkills` returns only enabled skills bound to the agent and filters by class, ordered by `skills.id`. `RouteSkill` lowercases the question, splits `trigger_text` on comma/Chinese punctuation, counts matching terms, and replaces the winner only on a strictly greater score. That yields at most one skill and makes ties resolve to the lower id without a separate sort. Zero matches means no skill rather than an error.

Trade-off: keyword matching misses paraphrase. It is predictable, testable, and keeps the model from inventing which skill to use.

### Conversation ownership is a composite predicate

Every read uses `WHERE id=? AND user_id=? AND class_id=?`; listing uses `WHERE user_id=? AND class_id=?`. Creation first loads the agent by the caller's class and requires `enabled`, then stores both ownership columns. A non-owner therefore sees the same `ErrNoRows` as a nonexistent conversation, mapped to 404 before SSE headers.

### Context is a bounded window, not a transcript

After persisting the current user message, the service loads the newest seven messages, removes the current message, and keeps at most six prior messages in ascending order. Each is truncated to 500 runes when composed for the model. The current question is appended once, with evidence, so it is never duplicated from history.

### Metadata-first SSE

The HTTP handler supplies two callbacks. `Chat` performs validation, retrieval, tool calls, and history assembly before invoking `metaFn`; only at that point does the handler set `text/event-stream`, `no-cache`, `X-Accel-Buffering: no`, and status 200, then emit `meta`. Deltas follow. `done` is emitted only after the assistant message is persisted. If an error occurs before `metaFn`, the handler can still return a normal 4xx/5xx JSON error; if it occurs after, it emits a terminal `error` event and suppresses `done`.

This is why the handler does not write SSE headers at function entry. It also lets nginx disable buffering end-to-end without changing the event contract.

### Evidence in user role, citations in meta

`BuildEvidence` creates numbered citations and `[n] 材料《title》切片 index` blocks. `composeMessages` puts agent prompt and skill guidance in the system message, then a single user message containing evidence, optional tool data, and the question. Models treat user content as data more reliably than a system message containing arbitrary uploaded text, and tests can assert the separation. `meta.citations` is always a non-nil array so clients do not branch on null.

### No-evidence behavior is a control-flow branch

Streaming still generates an answer with no evidence, then strips any `[n]` markers; `meta.citations` is empty. The non-streaming ask path returns the fixed string before constructing a model request. This prevents both fabricated citations and accidental model calls on an empty retrieval.

## Risks / Trade-offs

- **Lexical routing false negatives** → keep trigger terms teacher-editable; routing remains deterministic and inspectable.
- **History truncation can cut mid-sentence** → 500 runes is a hard budget; recent turns are prioritized and the current question is always complete.
- **SSE through proxies** → nginx sets `proxy_buffering off`, the handler sets `X-Accel-Buffering: no`, and write timeouts are disabled for the API.
- **Error after headers cannot change status** → terminal `error` event; clients must treat a missing `done` as failure.
- **Offline answer is templated** → documented fallback; it still honors citations and the no-`[n]` rule.
- **Raw skill text is stored** → only teachers can write it and it is returned only to the class; it is treated as guidance, not executable code.
- **Tool output is untrusted** → later capability filters instruction patterns and oversized output before composing the user message.

## Migration

1. `db.Migrate` adds `class_settings`, `skills`, `agents`, `agent_skills`, `agent_materials`, `conversations`, and `messages` with foreign keys and `IF NOT EXISTS`.
2. `db.Seed` inserts the default class prompts, two class-A skills/agents, and a class-B skill only when their natural keys are absent, so existing teacher edits survive.
3. nginx must pass SSE without buffering; the existing `/api/` location already disables buffering and caching.
4. Rollback keeps the new tables; older binaries ignore them. No destructive migration is required.
