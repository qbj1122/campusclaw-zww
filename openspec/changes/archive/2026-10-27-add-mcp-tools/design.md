# Design

## Context

See `proposal.md` and `specs/mcp-tools/spec.md`. The assistant already streams from a routed skill and places citations before text. This change adds a separate Go MCP process, a class-scoped registry, and a deterministic tool stage. The API's own `MCP_AUTH_TOKEN` is required configuration even when no server is registered; the MCP process fails closed without the same class of secret. Tool use must work with the offline answer path and must not give uploaded or third-party text instruction authority.

## Goals / Non-Goals

**Goals:**

- A small, testable MCP implementation with two real tools and standard JSON-RPC envelopes.
- One secret direction: the API stores a token to call out; the API never returns it.
- Cache state that reflects a completed handshake plus discovery, not a partial connect.
- Tool references that cannot be saved unless they are currently usable.
- Calls that are ordered, visible before generation, filtered, and auditable.
- Audit evidence that cannot be destroyed by deleting a server.

**Non-Goals:**

- Implementing the full MCP specification (resources, prompts, notifications, sessions).
- Encrypting tokens at rest or integrating a secret manager.
- Model-driven tool selection or retry/backoff.
- Cross-class or administrator audit views.

## Decisions

### JSON-RPC over a single POST endpoint

The MCP server uses Go's `net/http` mux with `GET /health` and `POST /mcp`. The body is decoded once into a JSON-RPC envelope; invalid JSON returns `-32700`, a missing `jsonrpc`/`method` returns `-32600`, an unknown method returns `-32601`, and invalid tool params return `-32602`. Transport authentication failures return HTTP 401; protocol and parameter failures return HTTP 200 with an error object so clients can distinguish transport from RPC errors. The request body is capped at 1 MiB.

Tool schemas are returned by `tools/list` but are not sent to the model; the runtime constructs arguments deterministically. `formula_hint` receives the question; `unit_convert` uses the fixed demo arguments `1 m -> cm`. Unsupported units return a tool result with `isError=true` rather than an RPC envelope error.

Alternative considered: a full MCP SDK. Two tools and three methods do not justify the dependency, and a small implementation makes the protocol assertions explicit.

### Constant-time bearer comparison and environment-only startup

The MCP process reads `MCP_AUTH_TOKEN` at startup, exits non-zero when empty, and compares the presented token with `crypto/subtle.ConstantTimeCompare` after requiring the exact `Bearer ` prefix. Compose passes `${MCP_AUTH_TOKEN:?...}` to both services and probes `/health` with `wget`.

### Server registry stores secrets but never serializes them

`mcp_servers` stores `auth_token` for outbound calls. API models expose only `has_auth`, computed as `auth_token <> ''`. Create accepts an optional token; update treats omitted or blank as "keep existing" and a non-empty value as replacement. Every server query includes `class_id`, so another class's id yields `ErrNoRows` and a uniform 404.

Update clears `connected` and `connected_at` because a changed URL or credential invalidates the prior handshake. The tool cache is not treated as authoritative without `connected=true`; the next successful connect replaces it.

### Connect is transactional at the application level

`Connect` calls `initialize` with protocol `2024-11-05`, then `tools/list`, and only after both succeed maps the returned schemas into `mcp_tools`. `ReplaceTools` runs in a MySQL transaction that deletes the server's rows and inserts the new set. The caller then sets `connected=true`. On any error the handler marks the server disconnected and returns 502 without calling `ReplaceTools`, so partial discovery cannot masquerade as a complete cache.

Disconnect deletes cached tools and clears the connection. `GET /tools` returns whatever cache exists, which is empty after disconnect.

### Skill validation is ordered from syntax to discovery

On create/update, the skill file parser first enforces syntax. `validateSkillTools` then skips `knowledge_search`, parses `tool@server`, loads the server by `(class_id,name)`, requires `connected`, and checks `mcp_tools` for the tool name. Validation completes before the insert/update, so a rejected save cannot partially change a skill. This reuses the same parser used at runtime, keeping save-time and run-time interpretation aligned.

### Deterministic call stage before generation

`Assistant.Chat` performs retrieval, then `runTools`, then history assembly and prompt composition. `runTools` iterates the routed skill's `Tools` in order, skips `knowledge_search`, resolves the server by class and name, and calls it with the question (or the fixed unit-conversion args). It returns both `meta.tool_calls` and text blocks. No tool definitions are sent to the model, so the model cannot choose or invent a call.

Each call still allows generation to continue: a failed call contributes an `error` entry and is not added as evidence, while the answer is produced from citations and any successful tool data.

### Tool text is data with a conservative filter

A successful result passes `sanitizeToolOutput`. Empty text becomes a placeholder. Text over 1200 runes or matching instruction patterns (ignore/override, role reassignment, `system:`, command-like requests, exfiltration phrasing) is replaced wholesale by a fixed notice. Normal text is inserted into the final user message under `[工具 <ref> 返回的数据]`, never into the system message. The unfiltered raw result may appear in the audit detail for evidence, but not in model input or `meta.tool_calls.text`.

Alternative considered: prompt-level warnings only. A deterministic filter is testable and does not rely on the model obeying a warning.

### Audit rows use a restrictive foreign key

`mcp_tool_calls` stores class id, tool ref, server id, status enum (`ok`, `denied`, `error`), detail, user id, nullable conversation id, and timestamp. `fk_call_server` uses `ON DELETE RESTRICT`, so any audit history prevents server deletion and the handler maps the foreign-key failure to 503. Renamed or deleted names that no longer resolve cannot produce a server id, so the runtime reports denied in `meta` but deliberately writes no audit row; the alternative would require a dangling id or a synthetic server.

Audit reads filter by the caller's class and order by `created_at DESC, id DESC`; the handler defaults the limit to 50 and clamps above 100.

## Risks / Trade-offs

- **Tokens are stored in plaintext in MySQL** → API never returns them, database access is the trust boundary; encryption-at-rest is out of scope and not claimed.
- **No retry means transient MCP failure is visible** → call status `error` and an audit row; the answer still streams.
- **Output filtering can reject useful text** → conservative patterns and a 1200-rune cap; the rejection is visible in meta detail and audit.
- **A stale skill reference after rename is denied without audit** → no server id exists; the fix is to reconnect/update the skill. The denial is still visible in `meta.tool_calls`.
- **Connect and cache replacement are not one database transaction with the network call** → replacement happens only after complete discovery; a crash can leave disconnected state, never a partially replaced cache.
- **Audit table grows without bound** → out of scope for this lesson; deletion is intentionally blocked while evidence exists.
- **MCP server has no rate limiting** → deployment is internal on the Compose network; public exposure is out of scope.
- **Mutable tool output is untrusted** → filter plus user-role placement, not system-role placement.

## Migration

1. `db.Migrate` creates `mcp_servers`, `mcp_tools`, and `mcp_tool_calls` with class, uniqueness, and restrictive call foreign keys.
2. Deploy `mcp-demo` from `mcp-server/Dockerfile` with `MCP_AUTH_TOKEN` and `MCP_PORT=8090`; the API must receive the same token value.
3. No server rows are seeded. A teacher creates a server, connects it, then adds `tool@server` references to a skill.
4. Rollback keeps the additive tables. Existing skills that reference tools become inert if the MCP service is removed; saves are blocked until a connected server and discovered tool exist again.
