# campus-tools MCP server (Lesson 6)

Self-hosted JSON-RPC 2.0 MCP tool service. Endpoints:

- `GET /health` — no auth, returns `{"status":"ok"}`.
- `POST /mcp` — Bearer auth required; supports `initialize`, `tools/list`, `tools/call`.

Env: `MCP_PORT` (default `8090`), `MCP_AUTH_TOKEN` (required; startup fails when empty).
Tools: `formula_hint` (`query`) and `unit_convert` (`value`, `from`, `to`; m↔cm, kg↔g).

```bash
MCP_AUTH_TOKEN=secret go run ./cmd/mcp-server
curl -s http://localhost:8090/health
curl -s http://localhost:8090/mcp -H 'Authorization: Bearer secret' -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
curl -s http://localhost:8090/mcp -H 'Authorization: Bearer secret' -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"unit_convert","arguments":{"value":1,"from":"m","to":"cm"}}}'
```

Image: `docker build -f mcp-server/Dockerfile -t campus-mcp .`, then run with
`docker run -p 8090:8090 -e MCP_AUTH_TOKEN=secret campus-mcp` (token is never baked in).
