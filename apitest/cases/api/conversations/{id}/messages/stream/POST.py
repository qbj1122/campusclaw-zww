"""POST /api/conversations/{id}/messages/stream — SSE contract and DB rows."""

import json

from lib import Ctx


def run(ctx: Ctx) -> None:
    trigger = ctx.unique("stream", max_len=20)
    skill_name = ctx.unique("stream-skill")
    material = ctx.ensure_material("stream")
    skill = ctx.create_skill(
        ctx.skill_source(
            skill_name,
            f"{trigger},方程,去括号",
            ["knowledge_search"],
        )
    )
    agent = ctx.create_agent(
        ctx.unique("stream-agent"),
        prompt="你是班级A的数学助手，只给下一步提示。",
        skill_ids=[skill["id"]],
        material_ids=[material["id"]],
    )
    conversation = ctx.create_conversation(agent["id"], user="student_a1")
    question = f"{trigger} 这道方程去括号怎么去"
    try:
        response, events = ctx.stream(
            f"/api/conversations/{conversation['id']}/messages/stream",
            user="student_a1",
            content=question,
        )
        names = ctx.event_names(events)
        ctx.eq(names[0], "meta", "SSE first event is meta")
        ctx.eq(names[-1], "done", "SSE last event is done")
        ctx.not_contains(names, "error", "SSE has no error event")

        meta = events[0].data
        ctx.is_instance(meta, dict, "meta data")
        ctx.eq(meta.get("status"), "ok", "meta.status")
        ctx.is_instance(meta.get("citations"), list, "meta.citations")
        ctx.check(bool(meta.get("citations")), "stream returned citations", meta)
        skill_meta = meta.get("skill") or {}
        ctx.eq(skill_meta.get("name"), skill_name, "meta.skill.name")

        deltas = ctx.deltas(events)
        ctx.check(len(deltas) >= 2, "stream has multiple delta events", deltas)
        answer = "".join(deltas)
        ctx.check(bool(answer.strip()), "stream answer text non-empty")
        ctx.contains(answer, "[1]", "stream answer cites evidence")

        done_data = events[-1].data
        ctx.is_instance(done_data, dict, "done data")
        message_id = done_data.get("message_id")
        ctx.is_instance(message_id, int, "done message_id")

        rows = ctx.query(
            "SELECT id, role, content, citations_json, skill_name, tool_calls_json "
            "FROM messages WHERE conversation_id=%s ORDER BY id",
            (conversation["id"],),
        )
        ctx.eq(len(rows), 2, "DB persisted user and assistant messages")
        user_row, assistant_row = rows
        ctx.eq(user_row["role"], "user", "DB user message role")
        ctx.eq(user_row["content"], question, "DB user message content")
        ctx.eq(assistant_row["id"], message_id, "DB assistant id vs done.message_id")
        ctx.eq(assistant_row["role"], "assistant", "DB assistant role")
        ctx.eq(assistant_row["content"], answer, "DB assistant content vs delta stream")
        ctx.eq(assistant_row["skill_name"], skill_name, "DB assistant skill_name")
        ctx.check(
            assistant_row["citations_json"] is not None,
            "DB assistant citations_json persisted",
            assistant_row,
        )
        ctx.eq(
            json.loads(assistant_row["citations_json"]),
            meta.get("citations"),
            "DB citations vs meta citations",
        )
        ctx.check(
            assistant_row["tool_calls_json"] is None,
            "knowledge_search does not write tool_calls_json",
            assistant_row,
        )

        detail = ctx.get(
            f"/api/conversations/{conversation['id']}", user="student_a1", expect=200
        )
        detail_payload = ctx.json(detail)
        ctx.eq(len(detail_payload.get("messages") or []), 2, "detail message count")
        ctx.eq(
            detail_payload["messages"][1]["content"], answer, "detail assistant content"
        )
        conv = detail_payload.get("conversation") or {}
        ctx.eq(conv.get("title"), question[:24], "conversation title from first question")

        cross = ctx.stream(
            f"/api/conversations/{conversation['id']}/messages/stream",
            user="teacher_a",
            content=question,
            expect=404,
        )
        ctx.expect_status(cross[0], 404, "cross-user stream")

        short = ctx.stream(
            f"/api/conversations/{conversation['id']}/messages/stream",
            user="student_a1",
            content="题",
            expect=400,
        )
        ctx.expect_status(short[0], 400, "short stream content")
    finally:
        ctx.remove_conversation(conversation["id"])
        ctx.remove_agent(agent["id"])
        ctx.remove_skill(skill["id"])
