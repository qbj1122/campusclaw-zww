# Tasks

## 1. Self-hosted MCP service

- [x] 1.1 Implement `GET /health` without auth and `POST /mcp` with exact Bearer authentication using constant-time comparison; verify: health returns `200 {"status":"ok"}`, missing/wrong tokens return `401`, and the token is read only from `MCP_AUTH_TOKEN`.
- [x] 1.2 Implement JSON-RPC validation and `initialize`; verify: `2024-11-05`, `campus-tools`, version `1.0.0`, and `capabilities.tools` in the result, and that malformed JSON/unknown method return `-32700`/`-32601` over HTTP 200.
- [x] 1.3 Implement `tools/list` and `tools/call` for exactly `formula_hint` and `unit_convert`, including argument validation and unsupported-unit `isError=true`; verify: `1 m -> cm` yields `1 米 = 100 厘米` and missing tool name returns `-32602`.
- [x] 1.4 Fail startup when `MCP_AUTH_TOKEN` is empty and honor `MCP_PORT`; verify: the process exits non-zero before binding and `go test ./cmd/mcp-server` covers health, auth rejection, initialize, discovery, and both tools.

## 2. Server registry and secret handling

- [x] 2.1 Create `mcp_servers` with class scoping, class-unique names, URL/token columns, and connected state; verify: a duplicate `(class_id,name)` insert fails and another class can reuse the name.
- [x] 2.2 Implement create/list/tools responses that expose `has_auth` and never serialize `auth_token`; verify: the token string is absent from every response body after create, update, and list.
- [x] 2.3 Validate server name `^[a-z][a-z0-9_-]{0,63}$` and http(s) base URL up to 512 characters; verify: invalid name, invalid URL, and same-class duplicate return `400` and create no row.
- [x] 2.4 Implement update with omitted/blank token preservation and explicit replacement, clearing `connected`/`connected_at`; verify: `has_auth` remains true and the next connect uses the preserved or replaced token.
- [x] 2.5 Scope every server mutation and tool lookup by `class_id`; verify: class-A calls against a class-B id return `404` and leave class-B state unchanged.

## 3. Connect, discovery, and disconnect

- [x] 3.1 Implement connect as `initialize` followed by `tools/list` using the stored token; verify: a successful connect stores exactly `formula_hint` and `unit_convert`, sets `connected=true`, and reports `tool_count=2`.
- [x] 3.2 Replace the cache only after both discovery steps succeed, inside a transaction; verify: a repeated connect replaces prior rows and a failed initialize or malformed `tools/list` returns `502`, stays disconnected, and writes no partial rows.
- [x] 3.3 Implement disconnect as cache deletion plus disconnected state; verify: `204`, zero `mcp_tools` rows, and `connected=false` while `GET /api/mcp/servers/{id}/tools` still works with an empty array.

## 4. Skill references and deterministic invocation

- [x] 4.1 On skill create/update, validate each `tool@server` reference for syntax, same-class server existence, connected state, and discovered tool name before writing; verify: each failure returns `400` and leaves the previous skill unchanged.
- [x] 4.2 Accept `knowledge_search` without a server and preserve tool declaration order; verify: a saved mixed list returns the same order from `GET /api/skills`.
- [x] 4.3 Invoke every non-`knowledge_search` tool after retrieval and before model generation, with no tool definitions sent to the model; verify: a recording MCP stub observes the call before any chat-completion request.
- [x] 4.4 Include `tool_calls` in the first `meta` event before any `delta`, and omit the field when no MCP tool ran; verify: SSE frame order and payload over HTTP.
- [x] 4.5 Keep generation alive on tool errors; verify: a failing MCP call yields `status=error` in meta plus an `error` audit row, while the stream still ends with `done`.

## 5. Untrusted output and audit evidence

- [x] 5.1 Filter tool output by instruction patterns and a 1200-rune cap, replacing rejected output wholesale with the fixed notice; verify: the raw phrase is absent from `meta.tool_calls.text` and all outbound model messages.
- [x] 5.2 Label accepted output as `[工具 <ref> 返回的数据]` in the final user message and never in the system message; verify: capture the outbound request and search both roles.
- [x] 5.3 Insert audit rows for `ok`, `denied` (disconnected), and `error` (RPC/`isError`) with class, server, user, conversation, bounded detail, and timestamp; verify: each status in MySQL and via the query endpoint.
- [x] 5.4 Emit `denied` in meta without an audit row for malformed refs and for names that no longer resolve after rename/delete; verify: `mcp_tool_calls` row count is unchanged.
- [x] 5.5 Implement teacher-only class-scoped audit querying newest-first with default 50 and maximum 100; verify: a student gets `403`, `limit=1000` returns at most 100, and class-B rows never appear for class A.
- [x] 5.6 Make server deletion fail with `503` when audit rows reference it and succeed with `204` otherwise; verify: the audited server and rows remain queryable after refusal.

## 6. Deployment and documentation

- [x] 6.1 Add `mcp-demo` to Compose with a required `MCP_AUTH_TOKEN`, internal port 8090, and `/health` healthcheck; verify: `docker compose config` without the token fails and no token literal is committed.
- [x] 6.2 Confirm `docs/API.md` and `mcp-server/README.md` document the JSON-RPC methods, Bearer requirement, two tools, teacher-only APIs, `has_auth`, connect/disconnect, audit limits, and the audited-delete 503; verify: both files contain the named fields and limits.
- [x] 6.3 Confirm the streaming contract reports `tool_calls` before `delta` and the offline answer path exercises tools without a configured chat model; verify: inspect the SSE frames and run with no chat key.
