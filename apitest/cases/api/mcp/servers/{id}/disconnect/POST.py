"""POST /api/mcp/servers/{id}/disconnect — clears cache and denies later calls."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    material = ctx.ensure_material("disconnect")
    server = ctx.create_server()
    skill = None
    agent = None
    conversation = None
    try:
        connect = ctx.post(
            f"/api/mcp/servers/{server['id']}/connect", user="teacher_a", expect=200
        )
        ctx.eq(ctx.json(connect).get("server", {}).get("tool_count"), 2, "precondition connect")

        cross = ctx.post(
            f"/api/mcp/servers/{server['id']}/disconnect", user="teacher_b", expect=404
        )
        ctx.assert_error(cross, 404, "not_found", "cross-class disconnect")
        student = ctx.post(
            f"/api/mcp/servers/{server['id']}/disconnect", user="student_a1", expect=403
        )
        ctx.assert_error(student, 403, "forbidden", "student disconnect")

        # Skills validate that the server is connected, so build the skill,
        # agent and conversation before breaking the connection.
        trigger = ctx.unique("disc", max_len=18)
        ref = f"formula_hint@{server['name']}"
        skill = ctx.create_skill(
            ctx.skill_source(
                ctx.unique("disc-skill"),
                f"{trigger},方程",
                ["knowledge_search", ref],
            )
        )
        agent = ctx.create_agent(
            ctx.unique("disc-agent"),
            prompt="你是班级A的数学助手。",
            skill_ids=[skill["id"]],
            material_ids=[material["id"]],
        )
        conversation = ctx.create_conversation(agent["id"], user="student_a1")
        question = f"{trigger} 这道方程去括号怎么去"

        response = ctx.post(
            f"/api/mcp/servers/{server['id']}/disconnect", user="teacher_a", expect=204
        )
        ctx.eq(response.text, "", "disconnect body")
        ctx.eq(
            bool(ctx.scalar("SELECT connected FROM mcp_servers WHERE id=%s", (server["id"],))),
            False,
            "DB connected cleared",
        )
        ctx.eq(
            ctx.scalar("SELECT COUNT(*) AS n FROM mcp_tools WHERE server_id=%s", (server["id"],)),
            0,
            "DB tool cache cleared",
        )
        tools = ctx.get(
            f"/api/mcp/servers/{server['id']}/tools", user="teacher_a", expect=200
        )
        ctx.eq(ctx.json(tools).get("tools"), [], "tools empty after disconnect")

        again = ctx.post(
            f"/api/mcp/servers/{server['id']}/disconnect", user="teacher_a", expect=204
        )
        ctx.eq(again.text, "", "second disconnect body")

        _response, events = ctx.stream(
            f"/api/conversations/{conversation['id']}/messages/stream",
            user="student_a1",
            content=question,
        )
        ctx.eq(events[0].event, "meta", "denied stream meta first")
        meta = events[0].data
        calls = meta.get("tool_calls")
        ctx.is_instance(calls, list, "meta.tool_calls")
        ctx.check(bool(calls), "meta reports the attempted tool call", meta)
        call = calls[0]
        ctx.eq(call.get("ref"), ref, "denied tool ref")
        ctx.eq(call.get("status"), "denied", "denied tool status")
        ctx.contains(call.get("detail", ""), "未连接", "denied tool detail")
        ctx.eq(events[-1].event, "done", "denied stream still completes")

        audit = ctx.one(
            "SELECT class_id, tool_ref, server_id, status, detail, user_id, conversation_id "
            "FROM mcp_tool_calls WHERE conversation_id=%s ORDER BY id DESC LIMIT 1",
            (conversation["id"],),
        )
        ctx.check(audit is not None, "denied call wrote audit row", conversation["id"])
        ctx.eq(audit["tool_ref"], ref, "audit tool_ref vs meta")
        ctx.eq(audit["status"], "denied", "audit status vs meta")
        ctx.eq(audit["server_id"], server["id"], "audit server_id")
        ctx.eq(audit["user_id"], ctx.user_id("student_a1"), "audit user_id")
        ctx.eq(audit["class_id"], ctx.class_id("student_a1"), "audit class_id")

        listing = ctx.get(
            "/api/mcp/tool-calls", user="teacher_a", params={"limit": 100}, expect=200
        )
        api_rows = ctx.json(listing).get("items") or []
        matches = [
            row
            for row in api_rows
            if row.get("conversation_id") == conversation["id"] and row.get("tool_ref") == ref
        ]
        ctx.check(bool(matches), "audit row visible through tool-calls API", api_rows[:5])
        ctx.eq(matches[0]["status"], "denied", "tool-calls API status")
        ctx.eq(matches[0]["server_name"], server["name"], "tool-calls API server_name")
    finally:
        if conversation is not None:
            ctx.remove_conversation(conversation["id"])
        if agent is not None:
            ctx.remove_agent(agent["id"])
        if skill is not None:
            ctx.remove_skill(skill["id"])
        ctx.remove_server(server["id"])
