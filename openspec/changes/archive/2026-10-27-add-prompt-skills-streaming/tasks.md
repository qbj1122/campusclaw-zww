# Tasks

## 1. Configuration and ownership model

- [x] 1.1 Create `class_settings`, `skills`, `agents`, `agent_skills`, `agent_materials`, `conversations`, and `messages` with unique class-scoped names and foreign keys; verify: repeated startup preserves rows and a duplicate `(class_id,name)` insert fails.
- [x] 1.2 Seed class prompts, two class-A skills with agents, and a class-B skill idempotently; verify: a restart does not duplicate them and a later teacher edit is not overwritten.
- [x] 1.3 Implement class prompt read/update with the 2000-character bound and teacher-only write; verify: `student_a1` gets `403`, an overlong prompt gets `400`, and a valid update is visible to another class-A user.
- [x] 1.4 Implement agent CRUD with prompt/enabled/skill/material fields, same-class binding checks, and class-unique names; verify: cross-class bindings and duplicate names return `400`, and students list only enabled agents.
- [x] 1.5 Verify `docs/API.md` documents the agent, prompt, skill, and conversation routes and their role restrictions; verify: every documented route and role matches the registered handlers.

## 2. Skill parsing and deterministic routing

- [x] 2.1 Implement the metadata-above-blank-line parser with the five allowed fields, duplicate/unknown detection, and the 8000/2000/32/120/32 character limits; verify: each rejection returns `400` and writes no row.
- [x] 2.2 Parse `工具` entries as `knowledge_search` or `tool@server`, rejecting malformed or uppercase tool names syntactically; verify: a malformed ref returns `400` before any server lookup.
- [x] 2.3 Enforce class-unique skill names on create and update; verify: a duplicate returns `400`, the original row is unchanged, and another class can reuse the name.
- [x] 2.4 Implement lexical routing over enabled agent-bound skills with one winner and lower-id tie-breaking; verify: a 2-vs-1 match picks the higher score and a 1-vs-1 tie picks the lower id.
- [x] 2.5 Load skills on every turn; verify: disabling the matching skill removes `meta.skill` on the next message and re-enabling restores it without a restart.
- [x] 2.6 Verify `GET /api/skills` returns class-scoped `source_text`, parsed fields, and tools, while all skill/prompt/agent mutations return `403` for students; verify: class-B skill rows are absent and a student write changes no row.

## 3. Conversations and bounded context

- [x] 3.1 Implement conversation creation for an enabled same-class agent and ownership columns; verify: class-B, disabled, and unused agent ids return `404` and create no row.
- [x] 3.2 Scope conversation list/get/stream by `(user_id,class_id)`; verify: another user's conversation returns `404` before SSE headers and no message is written.
- [x] 3.3 Bound model history to the six latest prior messages and truncate each to 500 runes; verify: an eight-message conversation sends six prior messages and a 600-character message appears as 500 characters.
- [x] 3.4 Map validation failures (question shorter than 2 or longer than 500 characters) to `400` before streaming; verify: the response is JSON and contains no SSE headers.

## 4. Streaming, evidence, and citations

- [x] 4.1 Emit `meta` only after validation/retrieval/tool work and before any delta, then stream deltas and finish with `done.message_id`; verify: event order and that the persisted assistant content equals the concatenated deltas.
- [x] 4.2 Put `[n] 材料《title》切片 index` evidence in the final user message and keep chunk text out of the system message; verify: capture the outbound request and search both roles.
- [x] 4.3 Keep `meta.citations` an always-present array with 1-based index, title, and chunk index; verify: a grounded reply's markers stay within the citation count.
- [x] 4.4 Answer with empty citations and no `[n]` markers when retrieval is empty; verify: the stream still emits `meta`, at least one `delta`, and `done`.
- [x] 4.5 Emit a terminal `error` event without `done` when generation fails after `meta`, and persist no assistant row; verify: force a generation error and inspect the SSE frames and messages table.
- [x] 4.6 Confirm nginx disables proxy buffering/caching for `/api/` and that `docs/API.md` shows the `meta`/`delta`/`done` frames and citation fields; verify: both files contain the named settings and event fields.
