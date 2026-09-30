# Proposal

## Why

CampusClaw needs a trustworthy multi-tenant foundation before retrieval and assistants can be added. Lessons 2–3 require real accounts, role separation, server-side class isolation, and a materials ingestion path whose uploaded text is stored for later indexing.

## What Changes

- Add username/password login backed by bcrypt hashes and HS256 Bearer JWTs, with logout revocation.
- Add `teacher` and `student` roles; teacher-only material writes return `403` for students.
- Derive the active class exclusively from the authenticated user and return a uniform `404` for both cross-class and absent resources.
- Add teacher multipart upload of non-empty UTF-8 `.txt`/`.md` files into `materials` plus `knowledge_entries.body_text`, returned with `201` and a chunk count.
- Keep the material row after a later vector-index failure; report the failure through `index_error` without discarding the uploaded body.
- Seed classes `班级A` / `班级B`, two teachers and two students with bcrypt hashes, idempotently.
- Require database password, JWT secret, seed password, and MCP token from the environment; refuse to start when required values are absent.
- Provide Docker Compose topology with MySQL, Qdrant, the MCP demo service, the API, and nginx, plus a liveness `GET /health` that does not query dependencies.

## Capabilities

### New Capabilities
- `auth-upload`: account authentication, teacher/student authorization, server-side class isolation, seeded demo data, environment-only secrets, and teacher text-material ingestion with a liveness probe.

### Modified Capabilities
- None.

## Impact

- New API surface: `POST /api/login`, `POST /api/logout`, `GET /api/me`, `GET /health`, and teacher material upload/detail/file endpoints.
- New tables and seed rows: `classes`, `users`, `token_revocations`, `materials`, `knowledge_entries`.
- New configuration requirements: `DB_PASSWORD`, `JWT_SECRET`, `SEED_PASSWORD`, and `MCP_AUTH_TOKEN`.
- Affected code: `internal/auth`, `internal/httpapi`, `internal/db`, `internal/config`, `cmd/server`, `docker-compose.yml`, `Dockerfile`.
- Existing later-lesson behavior (retrieval, streaming, MCP tools) is out of scope except where upload must leave `body_text` ready for indexing.

## Non-goals

- No self-service registration, password reset, OAuth, or refresh tokens.
- No fine-grained per-material ACLs; isolation is by class.
- No binary formats, OCR, or PDF ingestion.
- No claim that a model gateway is configured; the service must run with the offline fallback.
