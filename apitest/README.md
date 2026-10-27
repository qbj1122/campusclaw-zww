# CampusClaw API test suite

Black-box API + database verification for the running CampusClaw app.  The
suite only creates files under `apitest/`; it never changes Go, web, OpenSpec or
Docker sources.

## Layout

```
apitest/
  lib.py                    HTTP, login cache, assertions, MySQL, SSE parser
  run_all.py                discovery, PASS/FAIL runner, HTML report
  cases/
    api/<url path>/<METHOD>.py    one script per endpoint
    e2e/mcp_flow.py               end-to-end MCP tool flow
  report.html               generated, gitignored, not submitted
```

Dynamic path segments use braces, for example
`cases/api/materials/{id}/GET.py` exercises `GET /api/materials/{id}`.
Every case exports:

```python
def run(ctx):
    ...
```

`ctx` is shared per run and provides:

- `ctx.get/post/put/delete(...)` with `user="teacher_a"` and automatic JWT;
- `ctx.stream(...)` returning parsed SSE events (`meta`, `delta`, `done`);
- `ctx.eq/check/contains/assert_error/...` assertions with response detail;
- `ctx.one/query/scalar/execute/insert(...)` pymysql helpers;
- `ctx.unique(...)`, `ctx.ensure_material(...)`, `ctx.connected_server()`
  fixtures and cleanup tracking.

## Run

```bash
python3 apitest/run_all.py
python3 apitest/run_all.py --filter materials
python3 apitest/run_all.py --filter api/mcp --verbose
```

Exit code is non-zero when any case fails.  The runner always writes
`apitest/report.html` (gitignored) with per-case status, duration, failure
detail and warnings.

## Environment

Values are read from the process environment.  `apitest/lib.py` also loads the
repository-root `.env` when present (without overriding real environment
variables).  The local verification defaults are:

| Variable | Default |
|---|---|
| `BASE_URL` | `http://127.0.0.1:18080` |
| `APITEST_PASSWORD` (fallback `SEED_PASSWORD`) | `Campus@2026` |
| `DB_HOST` | `127.0.0.1` |
| `DB_PORT` | `3307` |
| `DB_USER` | `campusclaw` |
| `DB_PASSWORD` | `CampusDB@2026` |
| `DB_NAME` | `campusclaw` |
| `MCP_BASE_URL` | `http://127.0.0.1:8090` |
| `MCP_AUTH_TOKEN` | `test-mcp-token-0123456789` |
| `QDRANT_URL` | `http://127.0.0.1:6333` |
| `QDRANT_COLLECTION` | `campusclaw_chunks` |

These are the local demo credentials supplied for verification, not production
secrets.  No tokens are written to reports or logs.

## Coverage

| Area | Cases |
|---|---|
| auth | `api/login/POST`, `api/me/GET`, `api/logout/POST`, `api/health/GET` |
| materials | list/detail/file/upload/reindex, teacher 201, student 403, cross-class 404, DB material + knowledge entry + chunk + uploaded file |
| search | keyword/vector/hybrid, empty query 400, negative limit/offset 400, no-evidence, class isolation, offset beyond range 200 empty |
| ask | evidence answer with citations verified against DB chunks, fixed no-evidence text and `citations: []` |
| prompt/skills/agents | prompt GET/PUT, skill create/update/delete + invalid/duplicate/tool validation, agent CRUD + bindings, student 403 |
| conversations | create with valid/invalid/disabled/other-class agent, own-only list, cross-user/cross-class 404, SSE meta/citations/skill/multiple deltas/done + persisted message rows |
| MCP | register (token never echoed), duplicate 400, connect discovery exactly `formula_hint` + `unit_convert`, idempotent connect, invalid token 502, tool@server validation, disconnect→denied call + audit, class-scoped tool-calls, student 403, audited delete 503 |
| E2E | unique server, connect, skill with `formula_hint@<server>`, agent create/update, conversation, streamed answer, `meta.tool_calls.status=ok` and matching audit row |

MCP disconnect/E2E cases necessarily cross endpoint boundaries; every other
case owns its fixture so ordering and partial failures cannot cascade.

## Isolation and cleanup

- Created names use timestamp/random suffixes, so repeated runs never collide
  with seed data or concurrent runs.
- Tracked skills, agents, conversations, materials, test-only MCP servers and
  their files are removed at the end of a run (including after failures).
- The E2E server is intentionally retained after its audit row is written;
  deleting it through the product API correctly returns 503.
- Deleting a material through the DB is followed by best-effort deletion of its
  Qdrant points.  At startup the runner also prunes Qdrant points whose MySQL
  chunk row no longer exists, so failed earlier runs cannot crowd vector search
  results.  This only touches test residue, never product source or seed data.

## Notes on findings

While building the suite, three contract divergences were observed in the
running build and were fixed outside this suite (no Go changes were made here):

1. `DELETE /api/skills/{id}` and `DELETE /api/agents/{id}` from another class
   returned 204 instead of the documented 404.
2. `PUT /api/mcp/servers/{id}` disconnected the server but kept cached tools
   (`tool_count` stayed 2) instead of clearing the cache.

The current cases assert the documented behaviour (404, `tool_count == 0`), so
they will catch a regression.
