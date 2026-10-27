# Design

## Context

See `proposal.md` and `specs/auth-upload/spec.md`. The implementation is a single Go HTTP service backed by MySQL 8, with bcrypt already available through `golang.org/x/crypto`. Later lessons add retrieval and streaming, so the authentication and upload boundaries must not bake in assumptions that would let a client choose its class or lose uploaded text.

Lesson 3's earlier Cookie-based wording is superseded: every business endpoint uses `Authorization: Bearer <JWT>`, which also works from the static web client and Swagger UI.

## Goals / Non-Goals

**Goals:**

- One authentication middleware and one teacher guard reused by every current and future route.
- Class identity that cannot be supplied or overridden by request data.
- An upload transaction whose durable body text is independent of vector indexing.
- Startup that fails closed on missing security configuration.

**Non-Goals:**

- Token refresh, key rotation, or per-device sessions.
- Row-level ACLs beyond class ownership.
- Virus scanning or content moderation of uploaded text.
- Making vector indexing atomic with the MySQL upload transaction.

## Decisions

### Bearer JWT with server-side revocation

Issue HS256 tokens with `sub` (user id), random `jti`, `iat`, and `exp` (`JWT_TTL_SECONDS`, default 86400). `Authenticate` validates the algorithm and expiry, loads the user and class from MySQL on every request, and checks `token_revocations` by `jti`. Logout inserts the `jti` with its original expiry; a background hourly job deletes expired revocations.

Alternatives considered: stateless tokens cannot implement logout; opaque session ids would need a session table and cookie handling. The chosen design keeps stateless verification while supporting explicit revocation.

### Role checks after authentication, not in handlers

`RequireTeacher` wraps teacher-only handlers after `Authenticate`. This ordering makes missing/invalid credentials `401` and authenticated students `403`, consistently across current and future routes. Route registration is the single place that declares the role.

### Class comes only from the loaded user row

Every store query that touches materials includes `class_id = user.ClassID`. Cross-class and missing rows therefore produce the same SQL `ErrNoRows`, which handlers map to a uniform `404 not_found`. Body fields named `class_id` are not part of any accepted request struct; JSON decoding also rejects unknown fields.

Alternative considered: authorize in the handler after fetching by id. That risks leaking existence through different errors and is easy to omit on new endpoints.

### Bcrypt cost 12 and idempotent seed helpers

The seed hashes `SEED_PASSWORD` once per process with bcrypt cost 12 and inserts only when the natural key is absent (`classes.name`, `users.username`, `materials (class_id,title)`, `knowledge_entries.material_id`, `skills.seed_key`, `agents (class_id,name)`). Existing hashes and uploaded rows are never overwritten. A login for an unknown user still runs a dummy bcrypt comparison to reduce timing differences; five failures per username+IP in five minutes are treated like a wrong password.

### Upload commits MySQL first, then indexes

`POST /api/materials` validates extension, size, UTF-8, and title, writes the file to `UPLOAD_DIR`, then commits `materials` plus `knowledge_entries.body_text`. Only after that does it call the retrieval indexer. An indexing error is returned in `index_error` while the response remains `201`; the source of truth is never deleted, and any chunks written before the failure remain non-ready rather than being reported as a successful index.

This deliberately trades strict index consistency for data durability. Reindex is the repair path.

### Environment-only secrets and health separation

`config.Load` requires `DB_PASSWORD`, `JWT_SECRET` (>=16 chars), and `MCP_AUTH_TOKEN`, and validates chunk bounds and embedding dimension before the server binds. Compose uses `${VAR:?message}` for passwords and the MCP token and `env_file: .env` for the API. `GET /health` is a constant `{"status":"ok"}` with no database or Qdrant call so it remains a liveness probe; readiness is exercised by real API calls and the Compose dependency graph.

Alternative considered: make `/health` check MySQL. That would restart otherwise healthy containers during a transient database blip and conflate liveness with readiness.

## Risks / Trade-offs

- **Bearer tokens are bearer credentials** → short-ish TTL, revocation on logout, no token logging, and secrets only from the environment.
- **Logout revocation depends on MySQL** → if MySQL is unavailable, protected requests already fail closed with `503`; revocation cleanup is best-effort.
- **Upload can succeed while indexing fails** → expose `index_error` and `index_status`, and provide teacher reindex; source `body_text` is never lost.
- **Seed password is shared across demo accounts** → acceptable for a classroom demo; the spec requires hashes, not per-user provisioning.
- **Uniform 404 can hide operational mistakes** → logs include request id, authenticated user, and class; clients still get no cross-class existence signal.
- **No per-material ACL** → any authenticated member of a class can read that class's materials; this is the lesson's tenancy boundary.

## Migration

1. Apply `db.Migrate` at startup; every table uses `CREATE TABLE IF NOT EXISTS`, so existing deployments gain new tables without destructive changes.
2. Run `db.Seed`; helpers insert only missing natural keys, so restarting an existing database adds no duplicates.
3. Set `DB_PASSWORD`, `JWT_SECRET` (>=16 chars), `SEED_PASSWORD`, and `MCP_AUTH_TOKEN` in `.env` before starting Compose.
4. Rollback is the previous image plus its database; new tables are additive. Existing password hashes remain valid because the seed never rewrites them.
