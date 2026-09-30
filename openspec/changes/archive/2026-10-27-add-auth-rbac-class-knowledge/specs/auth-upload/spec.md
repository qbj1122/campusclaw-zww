# Spec Delta

## Purpose

Provide authenticated, class-isolated access to CampusClaw and let teachers ingest plain-text knowledge so each class can later retrieve only its own materials.

## ADDED Requirements

### Requirement: Password login issues and revokes bearer tokens

The system SHALL authenticate a username and password against a bcrypt hash and issue an HS256 Bearer JWT. Protected endpoints SHALL reject absent, malformed, expired, or revoked tokens with HTTP 401.

#### Scenario: Valid teacher logs in
- **WHEN** a client posts `{"username":"teacher_a","password":"<SEED_PASSWORD>"}` to `POST /api/login`
- **THEN** the response is `200` with a non-empty `token`, an RFC 3339 `expires_at`, and `user` containing `username=teacher_a`, `role=teacher`, `class_id`, and `class_name=班级A`, and the response does not contain `password_hash`

#### Scenario: Wrong password is rejected
- **WHEN** a client posts an existing username with the wrong password to `POST /api/login`
- **THEN** the response is `401` with `error.code=unauthorized`, no `token` field, and the same message shape used for an unknown username

#### Scenario: Protected endpoint requires a bearer token
- **WHEN** a client calls `GET /api/me` with no `Authorization` header, or with `Authorization: Token <jwt>`
- **THEN** the response is `401` with `error.code=unauthorized`

#### Scenario: Logout revokes the presented token
- **WHEN** a logged-in user posts the same JWT to `POST /api/logout` and then calls `GET /api/me` with it
- **THEN** logout returns `204`, and the later call returns `401` with `error.code=unauthorized`

### Requirement: Teacher and student roles are enforced server-side

The system SHALL read the role from the authenticated user record on every request. Material upload and reindex endpoints SHALL be teacher-only, while authenticated students SHALL retain read access to their class materials.

#### Scenario: Student cannot upload
- **WHEN** `student_a1` posts a valid `.txt` file to `POST /api/materials`
- **THEN** the response is `403` with `error.code=forbidden` and no material or knowledge entry is created

#### Scenario: Teacher can upload while student can read
- **WHEN** `teacher_a` uploads a class-A material and `student_a1` then calls `GET /api/materials`
- **THEN** the upload returns `201`, and the student's list returns `200` containing that material

### Requirement: Class isolation is derived from the session

The system SHALL scope every material lookup and list by the authenticated user's `class_id`. A resource owned by another class and a nonexistent resource SHALL both return HTTP 404 with the same error shape.

#### Scenario: Cross-class material is indistinguishable from absent
- **WHEN** `teacher_a` calls `GET /api/materials/{id}` for a class-B material and for an unused id
- **THEN** both responses are `404` with `error.code=not_found` and the same body structure, exposing no class-B title or body

#### Scenario: List only contains the caller's class
- **WHEN** `teacher_a` calls `GET /api/materials`
- **THEN** the `items` array contains class-A seed titles and does not contain `B班物理笔记` or any class-B material id

#### Scenario: Cross-class file download is denied
- **WHEN** `student_a1` calls `GET /api/materials/{class_b_material_id}/file`
- **THEN** the response is `404` with `error.code=not_found` and no file bytes

### Requirement: Teachers upload text into material and knowledge entry rows

The system SHALL accept teacher multipart uploads with a `file` field whose extension is `.txt` or `.md`, non-empty valid UTF-8 content, and size at most `MAX_UPLOAD_BYTES`. A successful upload SHALL create one `materials` row and one `knowledge_entries` row in the session class with the file bytes stored in `body_text`.

#### Scenario: Markdown upload persists the body
- **WHEN** `teacher_a` posts a `.md` file titled `课堂补充` whose UTF-8 content is `# 标题\n\n正文`
- **THEN** the response is `201` with `id`, `title=课堂补充`, and `chunks`, and `knowledge_entries.body_text` for that material equals the uploaded content while `materials.class_id` equals `teacher_a`'s class

#### Scenario: Unsupported extension is rejected
- **WHEN** a teacher uploads `notes.pdf`
- **THEN** the response is `400` with `error.code=bad_request` and `materials` gains no row

#### Scenario: Empty or non-UTF-8 content is rejected
- **WHEN** a teacher uploads an empty file or bytes that are not valid UTF-8
- **THEN** the response is `400` with `error.code=bad_request` and no material row is created

#### Scenario: Oversized file is rejected
- **WHEN** a teacher uploads a file larger than `MAX_UPLOAD_BYTES`
- **THEN** the response is `413` with `error.code=too_large` and no material row is created

### Requirement: Upload survives a downstream indexing failure

The system SHALL commit the material and knowledge entry before attempting chunk indexing. If indexing fails, the upload SHALL still return HTTP 201 with `index_error`, and the stored `body_text` SHALL remain readable.

#### Scenario: Vector store outage does not discard the upload
- **WHEN** Qdrant is unreachable and a teacher uploads a valid `.md` file
- **THEN** the response is `201` containing a non-empty `index_error`, and `GET /api/materials/{id}` still returns the original `body_text` with `chunk_count=0` or only non-ready chunk rows

### Requirement: Seed data is deterministic and password-safe

On a fresh database the seed SHALL create classes `班级A` and `班级B`, teachers `teacher_a` / `teacher_b`, students `student_a1` / `student_b1`, and sample class-scoped materials. Seed passwords SHALL be stored only as bcrypt hashes, and reseeding SHALL NOT duplicate or overwrite existing rows.

#### Scenario: Fresh database contains two isolated classes
- **WHEN** the API starts against an empty schema
- **THEN** `classes` contains `班级A` and `班级B`, `teacher_a` and `student_a1` have `class_id` of `班级A`, `teacher_b` and `student_b1` have `class_id` of `班级B`, and each `users.password_hash` begins with `$2` and differs from the configured seed password

#### Scenario: Restart preserves uploaded and seeded data
- **WHEN** the API restarts after a teacher uploaded an extra material
- **THEN** the extra material still exists, the seed material and accounts are not duplicated, and existing password hashes are unchanged

### Requirement: Security-sensitive configuration is environment-only

The server SHALL read `DB_PASSWORD`, `JWT_SECRET`, `SEED_PASSWORD`, and `MCP_AUTH_TOKEN` from the environment. It SHALL refuse to start when `DB_PASSWORD`, `JWT_SECRET` (fewer than 16 characters), or `MCP_AUTH_TOKEN` is missing.

#### Scenario: Missing JWT secret stops startup
- **WHEN** the server process starts without `JWT_SECRET` or with a value shorter than 16 characters
- **THEN** it exits with a non-zero status and logs a configuration error naming `JWT_SECRET`, and it does not bind the HTTP port

#### Scenario: Compose requires secrets instead of embedding them
- **WHEN** `docker compose config` is evaluated without `DB_PASSWORD`
- **THEN** the command fails with a required-variable error, and the Compose file contains no literal database password, JWT secret, or MCP token

### Requirement: Compose topology exposes a dependency-free liveness check

The deployment SHALL run MySQL, Qdrant, the MCP demo service, the API, and nginx. The API SHALL expose `GET /health` returning HTTP 200 `{"status":"ok"}` without querying MySQL or Qdrant, and API startup SHALL wait for the MySQL healthcheck.

#### Scenario: Health remains live during a database outage
- **WHEN** MySQL is stopped and a client calls `GET /health`
- **THEN** the response is `200` with body `{"status":"ok"}` and does not return a database error

#### Scenario: Compose gates the API on MySQL health
- **WHEN** the Compose configuration is inspected
- **THEN** `db` defines a `mysqladmin ping` healthcheck and `api` declares `depends_on.db.condition=service_healthy`
