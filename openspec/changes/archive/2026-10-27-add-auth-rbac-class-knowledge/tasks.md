# Tasks

## 1. Database foundation and seed

- [x] 1.1 Create `classes`, `users`, `token_revocations`, `materials`, `knowledge_entries`, and related tables with `IF NOT EXISTS` migrations; verify: a second startup runs `db.Migrate` without error and does not drop existing rows.
- [x] 1.2 Seed `班级A` / `班级B`, `teacher_a`, `teacher_b`, `student_a1`, `student_b1`, and sample materials through idempotent helpers; verify: a restart leaves exactly one row per natural key and does not change an existing `password_hash`.
- [x] 1.3 Hash the seed password with bcrypt cost 12 and add a dummy-hash comparison for unknown usernames; verify: `password_hash` starts with `$2` and no API response contains it.
- [x] 1.4 Verify seeded class isolation with SQL: `teacher_a` and `student_a1` resolve to `班级A`, `teacher_b` and `student_b1` to `班级B`, and class-B materials are not returned by class-A queries; verify: the four user/class joins and a class-A material query return the expected rows.

## 2. Authentication and authorization

- [x] 2.1 Implement HS256 JWT issue/parse with `sub`, random `jti`, `iat`, `exp`, and required expiry; verify: a valid login returns `token` and `expires_at`, and a token signed with another secret returns `401`.
- [x] 2.2 Implement `Authenticate` to reject missing or malformed `Authorization`, expired tokens, revoked `jti`, and deleted users; verify: each case returns `401 unauthorized` and no handler runs.
- [x] 2.3 Implement `POST /api/logout` revocation and the hourly cleanup job; verify: the same token returns `401` immediately after logout and expired revocation rows are deleted.
- [x] 2.4 Add `RequireTeacher` around material write routes; verify: `student_a1` receives `403 forbidden` on upload while `teacher_a` receives `201`.
- [x] 2.5 Include request id, authenticated user id, class id, method, path, status, and duration in structured logs; verify: a login and a rejected cross-class request emit distinguishable log records without token text.

## 3. Class-scoped material access and upload

- [x] 3.1 Scope material list, detail, and file queries by the session class; verify: class-A calls for a class-B id and an unused id both return the same `404 not_found` body.
- [x] 3.2 Implement teacher multipart upload with `.txt`/`.md` extension checks, UTF-8 validation, `MAX_UPLOAD_BYTES` enforcement, and title fallback; verify: `400 bad_request` for bad extension/empty/non-UTF-8 and `413 too_large` for oversize.
- [x] 3.3 Commit `materials` and `knowledge_entries.body_text` before indexing; verify: `body_text` equals the uploaded bytes and `materials.class_id` equals the session class.
- [x] 3.4 Return `201` with `index_error` when downstream indexing fails and keep the stored body readable; verify: with Qdrant stopped that material detail still returns the original text and either no chunks or only non-ready chunks exist.
- [x] 3.5 Verify `GET /api/materials/{id}/file` returns stored bytes for the owning class and `404` for another class; verify: the owning download matches the uploaded file, the cross-class call is `404`, and `docs/API.md` documents the route as implemented.

## 4. Runtime configuration and deployment

- [x] 4.1 Load all secrets from the environment and fail startup on missing/short `JWT_SECRET`, empty `DB_PASSWORD`, or empty `MCP_AUTH_TOKEN`; verify: the process exits non-zero before binding the port and names the missing variable.
- [x] 4.2 Add MySQL, Qdrant, MCP demo, API, and nginx services to Compose with required-variable substitutions, MySQL healthcheck, and `api.depends_on.db.condition=service_healthy`; verify: `docker compose config` fails without `DB_PASSWORD` and shows no literal secrets.
- [x] 4.3 Add `GET /health` returning `200 {"status":"ok"}` without dependency access; verify: it still answers while MySQL is stopped and that `docs/API.md` lists it as public.
