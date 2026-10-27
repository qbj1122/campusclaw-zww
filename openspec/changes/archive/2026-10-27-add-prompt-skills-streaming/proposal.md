# Proposal

## Why

Lesson 5 turns retrieval into a usable classroom assistant. Teachers need reusable prompt/skill files and agent bindings, while students need owned conversations that stream answers with citations and predictable skill selection instead of opaque model behavior.

## What Changes

- Add class-scoped agents with a prompt, enabled flag, skill ids, and material ids; teacher-only writes, student-visible enabled agents only.
- Add a single-file skill format: metadata above the first blank line and guidance below it, with `名称`, `适用条件`, `资料`, `工具`, and `启用`.
- Validate skill files strictly: required fields, no duplicate/unknown fields, length limits, and tool references limited to `knowledge_search` or `tool@server` syntax.
- Route deterministically by lexical trigger matches over enabled skills bound to the agent; select at most one skill, with ties resolved by lower skill id.
- Add conversations owned by `(user_id, class_id)`; listing and access never cross users or classes.
- Add SSE answers with `meta`, `delta`, `done`, and `error`; citations are present in `meta` before any text.
- Limit assistant context to the last six prior messages, each truncated to 500 characters.
- Place retrieved evidence in the final user message as numbered data, never in the system prompt.
- Keep answering when retrieval is empty, with no fabricated `[n]` markers.
- Make disabled skills take effect on the next turn without restarting the process.
- Keep prompt and skill writes teacher-only.

## Capabilities

### New Capabilities
- `assistant-streaming`: agent and skill configuration, deterministic skill routing, conversation ownership, bounded history, metadata-first SSE streaming, grounded citations, no-evidence answering, and per-turn skill enablement.

### Modified Capabilities
- None.

## Impact

- New tables: `class_settings`, `skills`, `agents`, `agent_skills`, `agent_materials`, `conversations`, `messages`.
- New API surface: `/api/prompt`, `/api/skills`, `/api/agents`, `/api/conversations`, and `/api/conversations/{id}/messages/stream`.
- SSE changes: `text/event-stream`, no proxy buffering, `meta` before `delta`, terminal `done` or `error`.
- Affected code: `internal/service/skills.go`, `internal/service/assistant.go`, `internal/store/agents.go`, `internal/store/conversations.go`, `internal/httpapi/agent_handlers.go`, `internal/httpapi/conversation_handlers.go`, `nginx/default.conf`.
- Model use is optional: without a chat key the deterministic offline answer is streamed; tool-call behavior is owned by the later `mcp-tools` capability.

## Non-goals

- No model-driven tool selection or free-form agent planning.
- No streaming resumption, replay, or multi-device conversation sync.
- No semantic skill routing; matching is lexical and deterministic.
- No per-message editing or deletion API.
