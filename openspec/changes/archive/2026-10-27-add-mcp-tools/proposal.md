# Proposal

## Why

Lesson 6 adds real external tools without letting the model decide what to execute. The assistant needs a self-hosted MCP service, class-scoped server registration, explicit skill-to-tool bindings, deterministic invocation, untrusted-output handling, and an audit trail that survives server changes.

## What Changes

- Add a self-hosted MCP server with `GET /health` and `POST /mcp` JSON-RPC 2.0, requiring `Authorization: Bearer <MCP_AUTH_TOKEN>` and exposing `formula_hint` and `unit_convert`.
- Add teacher-only MCP server registration with name/URL validation, same-class unique names, and a persisted token that is never returned (`has_auth` only).
- Make connect a two-step handshake plus tool discovery that replaces the cached tool list; make disconnect clear the cache and connection state.
- Validate `tool@server` skill references on save: syntax, same-class server existence, connected state, and discovered tool name.
- Invoke a routed skill's tool references deterministically before generation, never by model choice.
- Put `tool_calls` in the SSE `meta` event before any text delta.
- Treat tool text as data: reject outputs containing instruction patterns or exceeding 1200 characters instead of following them.
- Record audit rows with status `ok`, `denied`, or `error`, scoped by class; a renamed or deleted server yields a denied call without an unlinkable audit row.
- Add a teacher-only, class-scoped audit query with default limit 50 and maximum 100.
- Refuse to delete a server that has audit rows, returning 503 and preserving evidence.
- Return 403 for every MCP API when the caller is a student.

## Capabilities

### New Capabilities
- `mcp-tools`: the self-hosted MCP protocol endpoint, class-scoped server lifecycle, secret handling, skill tool-reference validation, deterministic pre-generation calls, untrusted-output filtering, and audited tool use.

### Modified Capabilities
- None.

## Impact

- New tables: `mcp_servers`, `mcp_tools`, `mcp_tool_calls`.
- New API surface: `/api/mcp/servers` CRUD, connect, disconnect, tools, and `/api/mcp/tool-calls`.
- New deployment service: `mcp-demo` built from `mcp-server/Dockerfile`, with `MCP_AUTH_TOKEN` supplied only by environment.
- Runtime integration: the streaming assistant invokes tools before generation and includes `tool_calls` in `meta`.
- Affected code: `cmd/mcp-server`, `internal/service/mcp.go`, `internal/service/tools_runtime.go`, `internal/store/mcp.go`, `internal/httpapi/mcp_handlers.go`, `internal/httpapi/agent_handlers.go`, `docker-compose.yml`.
- No external model is required; tool invocation and offline answers work without a chat gateway.

## Non-goals

- No arbitrary user-supplied MCP servers outside the class-scoped registry.
- No model-selected tools, multi-step tool loops, or streaming tool results.
- No token encryption-at-rest claim; the database stores the token, but the API never returns it.
- No automatic retry, circuit breaker, or rate limiting for MCP calls.
- No cross-class audit sharing, even for administrators.
