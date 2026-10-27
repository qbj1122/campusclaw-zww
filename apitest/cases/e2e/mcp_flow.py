"""End-to-end MCP flow: register, connect, skill, agent, chat, audit.

The server is intentionally left behind because an audit row now references it
and the product contract requires DELETE to return 503 in that situation.
Skills/agents/conversations created here are removed after the assertions.
"""

import json

from lib import Ctx


def run(ctx: Ctx) -> None:
    server_name = ctx.unique("e2e-tools")
    server = ctx.create_server(name=server_name, track=False)
    skill = None
    agent = None
    conversation = None
    try:
        connect = ctx.post(
            f"/api/mcp/servers/{server['id']}/connect", user="teacher_a", expect=200
        )
        connect_payload = ctx.json(connect)
        ctx.eq(connect_payload.get("server", {}).get("connected"), True, "E2E connected")
        ctx.eq(
            {tool["name"] for tool in connect_payload.get("tools", [])},
            {"formula_hint", "unit_convert"},
            "E2E discovered tools",
        )

        trigger = ctx.unique("e2e", max_len=18)
        ref = f"formula_hint@{server_name}"
        skill_name = ctx.unique("e2e-skill")
        skill = ctx.create_skill(
            ctx.skill_source(
                skill_name,
                f"{trigger},方程,去括号",
                ["knowledge_search", ref],
                description="E2E 技能：先提示下一步，必要时使用公式提示工具。",
            )
        )
        ctx.contains(skill.get("tools", []), ref, "E2E skill saved tool ref")

        material = ctx.ensure_material("e2e")
        agent = ctx.create_agent(
            ctx.unique("e2e-agent"),
            prompt="你是 E2E 数学助手，只给下一步提示。",
            skill_ids=[skill["id"]],
            material_ids=[material["id"]],
        )
        updated = ctx.put(
            f"/api/agents/{agent['id']}",
            user="teacher_a",
            expect=200,
            json={
                "name": agent["name"],
                "prompt": "你是 E2E 数学助手，更新后的提示词：先给下一步提示。",
                "enabled": True,
                "skill_ids": [skill["id"]],
                "material_ids": [material["id"]],
            },
        )
        updated_payload = ctx.json(updated)
        ctx.eq(updated_payload.get("skill_ids"), [skill["id"]], "E2E agent binding after update")
        ctx.contains(
            updated_payload.get("prompt", ""), "更新后的提示词", "E2E agent update persisted"
        )

        conversation = ctx.create_conversation(agent["id"], user="student_a1")
        question = f"{trigger} 这道方程去括号怎么去，请给一个提示"
        _response, events = ctx.stream(
            f"/api/conversations/{conversation['id']}/messages/stream",
            user="student_a1",
            content=question,
        )
        ctx.eq(events[0].event, "meta", "E2E meta is first")
        meta = events[0].data
        ctx.eq(meta.get("status"), "ok", "E2E meta.status")
        ctx.eq((meta.get("skill") or {}).get("name"), skill_name, "E2E meta.skill")
        ctx.check(bool(meta.get("citations")), "E2E has citations", meta)
        calls = meta.get("tool_calls") or []
        ctx.check(bool(calls), "E2E meta.tool_calls present", meta)
        ctx.eq(calls[0].get("ref"), ref, "E2E tool ref")
        ctx.eq(calls[0].get("status"), "ok", "E2E tool call status")
        ctx.check(bool(calls[0].get("text")), "E2E tool call text", calls[0])

        deltas = ctx.deltas(events)
        ctx.check(len(deltas) >= 2, "E2E multiple deltas", deltas)
        answer = "".join(deltas)
        ctx.eq(events[-1].event, "done", "E2E done is last")
        message_id = events[-1].data.get("message_id")
        ctx.is_instance(message_id, int, "E2E done message_id")

        audit = ctx.one(
            "SELECT id, class_id, tool_ref, server_id, status, detail, user_id, conversation_id "
            "FROM mcp_tool_calls WHERE conversation_id=%s AND tool_ref=%s "
            "ORDER BY id DESC LIMIT 1",
            (conversation["id"], ref),
        )
        ctx.check(audit is not None, "E2E audit row exists", conversation["id"])
        ctx.eq(audit["status"], "ok", "E2E audit status")
        ctx.eq(audit["tool_ref"], ref, "E2E audit ref")
        ctx.eq(audit["server_id"], server["id"], "E2E audit server_id")
        ctx.eq(audit["user_id"], ctx.user_id("student_a1"), "E2E audit user_id")
        ctx.eq(audit["class_id"], ctx.class_id("student_a1"), "E2E audit class_id")

        assistant = ctx.one(
            "SELECT id, role, content, citations_json, skill_name, tool_calls_json "
            "FROM messages WHERE conversation_id=%s AND role='assistant' ORDER BY id DESC LIMIT 1",
            (conversation["id"],),
        )
        ctx.eq(assistant["id"], message_id, "E2E assistant message id")
        ctx.eq(assistant["content"], answer, "E2E assistant content vs deltas")
        ctx.eq(assistant["skill_name"], skill_name, "E2E assistant skill_name")
        ctx.check(assistant["citations_json"] is not None, "E2E assistant citations persisted")
        ctx.check(assistant["tool_calls_json"] is not None, "E2E assistant tool_calls persisted")
        ctx.eq(
            json.loads(assistant["tool_calls_json"])[0]["status"],
            "ok",
            "E2E stored tool_calls status",
        )

        listing = ctx.get(
            "/api/mcp/tool-calls", user="teacher_a", params={"limit": 100}, expect=200
        )
        api_matches = [
            row
            for row in ctx.json(listing).get("items", [])
            if row.get("tool_ref") == ref and row.get("conversation_id") == conversation["id"]
        ]
        ctx.check(bool(api_matches), "E2E audit visible in tool-calls API")
        ctx.eq(api_matches[0]["status"], "ok", "E2E API audit status")

        # The audited server must survive: product DELETE is expected to 503.
        blocked = ctx.delete(
            f"/api/mcp/servers/{server['id']}", user="teacher_a", expect=503
        )
        ctx.assert_error(blocked, 503, "unavailable", "E2E audited server delete")
        ctx.check(
            ctx.one("SELECT id FROM mcp_servers WHERE id=%s", (server["id"],)) is not None,
            "E2E audited server retained",
        )
    finally:
        if conversation is not None:
            ctx.remove_conversation(conversation["id"])
        if agent is not None:
            ctx.remove_agent(agent["id"])
        if skill is not None:
            ctx.remove_skill(skill["id"])
        # server is deliberately not deleted (audit evidence must remain).
