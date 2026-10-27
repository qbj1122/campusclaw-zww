# assistant-streaming Specification

## Purpose
Let a class configure reusable prompts, skills, and agents, then hold user-owned conversations that stream grounded, citation-bearing answers with deterministic skill selection.

## Requirements

### Requirement: Class prompt is readable and teacher-updatable

`GET /api/prompt` SHALL return the authenticated user's class default prompt as `{"prompt":"..."}`. `PUT /api/prompt` SHALL be teacher-only, accept up to 2000 characters, persist per class, and return the stored prompt.

#### Scenario: Teacher updates the class prompt
- **WHEN** `teacher_a` sends `{"prompt":"先问清楚学科"}` to `PUT /api/prompt`
- **THEN** the response is `200` with that prompt, and a later `GET /api/prompt` by `student_a1` returns the same value

#### Scenario: Prompt length is bounded
- **WHEN** a teacher submits a prompt longer than 2000 characters
- **THEN** the response is `400` with `error.code=bad_request` and the previous prompt remains unchanged

#### Scenario: Student cannot update the prompt
- **WHEN** `student_a1` calls `PUT /api/prompt`
- **THEN** the response is `403` with `error.code=forbidden` and the stored prompt is unchanged

### Requirement: Agents bind prompts, skills, and materials within one class

The system SHALL let teachers create, update, and delete class agents with `name`, `prompt`, `enabled`, `skill_ids`, and `material_ids`. Names SHALL be unique within a class, every binding SHALL resolve in the same class, and students SHALL see and open only enabled agents.

#### Scenario: Teacher creates an agent with valid bindings
- **WHEN** `teacher_a` posts a class-A agent with an existing class-A skill id and material id
- **THEN** the response is `201` with the agent id, `enabled`, `skill_ids`, and `material_ids`, and a subsequent `GET /api/agents` by `student_a1` includes it

#### Scenario: Duplicate agent name is rejected
- **WHEN** a teacher creates a second class-A agent named `数学解题助手`
- **THEN** the response is `400` with `error.code=bad_request` and no second row is created

#### Scenario: Cross-class or missing binding is rejected
- **WHEN** a teacher creates an agent using a class-B skill id or an unused material id
- **THEN** the response is `400` with `error.code=bad_request` and no agent or binding row is created

#### Scenario: Student cannot see a disabled agent
- **WHEN** a teacher disables an agent and `student_a1` calls `GET /api/agents/{id}`
- **THEN** the response is `404` with `error.code=not_found`, and the agent is absent from the student's list

### Requirement: Skill files use a strict two-part metadata format

A skill SHALL consist of metadata lines above the first blank line and non-empty guidance below it. Metadata SHALL allow only `名称`, `适用条件`, `资料`, `工具`, and `启用`, with no duplicates; `名称`, `适用条件`, and `启用` are required. The source SHALL be at most 8000 characters, the guidance at most 2000, the name at most 32, and the trigger text at most 120.

#### Scenario: Valid skill file is parsed
- **WHEN** a teacher posts a file with `名称：数学解题引导`, `适用条件：方程,去括号`, `资料：数学`, `工具：knowledge_search`, `启用：是`, a blank line, and guidance text
- **THEN** the response is `201` with `name=数学解题引导`, `trigger_text=方程,去括号`, `subject_text=数学`, `tools=["knowledge_search"]`, `enabled=true`, and the trimmed guidance as `description`

#### Scenario: Missing guidance is rejected
- **WHEN** a skill file has metadata but no blank line and no guidance body
- **THEN** the response is `400` with `error.code=bad_request` and no skill row is created

#### Scenario: Unknown or duplicate metadata field is rejected
- **WHEN** a skill file contains `作者：某老师` or repeats `名称` twice
- **THEN** the response is `400` with `error.code=bad_request` and the parser reports the offending line or field

#### Scenario: Size limits are enforced
- **WHEN** a skill source exceeds 8000 characters or its guidance exceeds 2000 characters
- **THEN** the response is `400` with `error.code=bad_request` and no skill is stored

#### Scenario: Malformed tool reference is rejected
- **WHEN** the `工具` line contains `formula_hint` without `@server` or contains `BadTool@server`
- **THEN** the response is `400` with `error.code=bad_request` identifying the invalid tool reference

### Requirement: Skill names are unique within a class

The system SHALL enforce a unique `(class_id, name)` skill name and return HTTP 400 on create or update conflicts without modifying the existing skill.

#### Scenario: Duplicate skill name is rejected
- **WHEN** a teacher creates or renames a skill to a name already used in the same class
- **THEN** the response is `400` with `error.code=bad_request` and the original skill row is unchanged

#### Scenario: Same skill name is allowed in another class
- **WHEN** `teacher_b` creates a class-B skill using a name that exists in class A
- **THEN** the response is `201` and both class-scoped rows coexist

### Requirement: Skill routing is deterministic and selects at most one skill

For each turn the system SHALL score only enabled skills bound to the conversation's agent, using case-insensitive substring matches of the skill's trigger terms against the question. It SHALL select the single highest score, keep the lower skill id on a tie, and select no skill when every score is zero.

#### Scenario: Higher lexical score wins
- **WHEN** a question matches two trigger terms of skill A and one trigger term of skill B
- **THEN** `meta.skill.name` is skill A's name and no second skill is applied

#### Scenario: Tie selects the lower skill id
- **WHEN** two enabled bound skills each match the same number of trigger terms
- **THEN** `meta.skill.name` is the skill with the lower `skills.id`

#### Scenario: No match means no skill
- **WHEN** the question shares no trigger term with any enabled bound skill
- **THEN** the `meta` event contains no `skill` field and the answer is still generated

#### Scenario: Disabled skill is ignored immediately
- **WHEN** a teacher disables the currently matching skill and the student sends the next message
- **THEN** that turn's `meta` has no `skill` field; re-enabling it makes the following turn report it again without a restart

### Requirement: Conversations are owned by one user and one class

A conversation SHALL be created only for an enabled agent in the caller's class and SHALL store `user_id` and `class_id`. Listing SHALL return only the caller's conversations; reading or streaming another user's or another class's conversation SHALL return 404.

#### Scenario: Student creates and lists an owned conversation
- **WHEN** `student_a1` posts `{"agent_id":<enabled class-A agent>}` to `POST /api/conversations`
- **THEN** the response is `201` with the conversation `id`, `user_id=student_a1.id`, and `class_id=班级A.id`, and it appears in that student's `GET /api/conversations`

#### Scenario: Another user's conversation is hidden
- **WHEN** `student_a1` calls `GET /api/conversations/{id}` for a conversation owned by another class-A student
- **THEN** the response is `404` with `error.code=not_found` and no messages

#### Scenario: Disabled or other-class agent cannot start a conversation
- **WHEN** a student posts a disabled agent id, a class-B agent id, or an unused agent id
- **THEN** the response is `404` with `error.code=not_found` and no conversation row is created

#### Scenario: Streaming into a foreign conversation is hidden
- **WHEN** a user calls `POST /api/conversations/{id}/messages/stream` for another user's conversation
- **THEN** the response is `404` before any SSE headers are sent and no message row is written

### Requirement: Assistant history is bounded to six prior messages of 500 characters

Each generated turn SHALL include at most the six most recent prior user/assistant messages and SHALL truncate each included message to 500 characters. The current question SHALL be sent separately with its evidence.

#### Scenario: Only six prior messages reach the model
- **WHEN** a conversation already contains eight prior messages and the student sends a ninth
- **THEN** the outbound model request contains six prior messages, in chronological order, plus the current user message

#### Scenario: Long history messages are truncated
- **WHEN** one included prior message is 600 characters
- **THEN** the outbound copy contains exactly its first 500 characters

### Requirement: Streaming emits meta before delta and ends with done or error

`POST /api/conversations/{id}/messages/stream` SHALL return `text/event-stream` with named `meta`, `delta`, `done`, and `error` events. `meta` SHALL be the first event and contain `status` and an always-array `citations`, plus `skill` when one was selected and `tool_calls` only when tools were invoked. `delta` events SHALL carry incremental text; `done` SHALL carry the persisted assistant `message_id`.

#### Scenario: Successful stream has ordered events
- **WHEN** a student posts a valid question to their conversation
- **THEN** the stream contains `meta` first, one or more `delta` events, then `done` with an integer `message_id`, and the assistant message is persisted with the same concatenated content

#### Scenario: Short question fails before streaming
- **WHEN** a student posts a one-character question
- **THEN** the response is HTTP 400 `bad_request` with a JSON error body and no `text/event-stream` headers or SSE frames

#### Scenario: Generation failure after meta emits error without done
- **WHEN** generation fails after the `meta` event has been sent
- **THEN** the stream emits an `error` event with a readable `message` and does not emit `done`, and no assistant message row is stored

### Requirement: Retrieved evidence is citation-first and confined to the user message

The stream SHALL make citation metadata available before any answer text, and retrieved evidence SHALL be placed in the final user-role message as numbered data. The system message SHALL contain the agent prompt and selected skill guidance but SHALL NOT contain retrieved chunk text.

#### Scenario: Citations arrive before deltas
- **WHEN** retrieval finds class-A evidence
- **THEN** the first `meta` event contains a non-empty `citations` array with 1-based indexes, material titles, and chunk indexes, and no `delta` precedes it

#### Scenario: Evidence stays in the user message
- **WHEN** the outbound model request for a grounded turn is captured
- **THEN** the system message contains no `[1] 材料《` evidence block and the final user message contains the numbered evidence block followed by the question

### Requirement: Empty evidence still produces an answer without citation markers

When retrieval returns no citation, the stream SHALL still complete with an answer and `meta.citations=[]`. The answer SHALL contain no `[n]` markers and SHALL not fabricate a material title.

#### Scenario: No-evidence stream completes cleanly
- **WHEN** a student asks a question that matches no class material
- **THEN** the `meta.status` is `ok`, `citations` is `[]`, at least one `delta` and a `done` follow, and the concatenated answer contains no substring matching `[` digit `]`

### Requirement: All prompt, skill, and agent mutations are teacher-only

The system SHALL require the teacher role for prompt updates and every skill or agent create, update, and delete. Authenticated students SHALL retain read access to prompts, skills, and enabled agents.

#### Scenario: Student mutations are forbidden
- **WHEN** `student_a1` calls `PUT /api/prompt`, `POST /api/skills`, `PUT /api/skills/{id}`, `DELETE /api/skills/{id}`, or any agent write
- **THEN** each response is `403` with `error.code=forbidden` and no class configuration row changes

#### Scenario: Student reads remain available
- **WHEN** `student_a1` calls `GET /api/prompt`, `GET /api/skills`, and `GET /api/agents`
- **THEN** each response is `200` and contains only class-A data
