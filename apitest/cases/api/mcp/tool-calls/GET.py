"""GET /api/mcp/tool-calls — class-scoped audit listing."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    server_a = ctx.create_server()
    server_b = ctx.create_server(user="teacher_b")
    audit_ids: list[int] = []
    try:
        ref_a = f"formula_hint@{server_a['name']}"
        ref_b = f"unit_convert@{server_b['name']}"
        audit_ids.append(
            ctx.insert(
                "INSERT INTO mcp_tool_calls"
                "(class_id, tool_ref, server_id, status, detail, user_id, conversation_id) "
                "VALUES (%s,%s,%s,%s,%s,%s,NULL)",
                (
                    ctx.class_id("teacher_a"),
                    ref_a,
                    server_a["id"],
                    "ok",
                    "class A audit fixture",
                    ctx.user_id("teacher_a"),
                ),
            )
        )
        audit_ids.append(
            ctx.insert(
                "INSERT INTO mcp_tool_calls"
                "(class_id, tool_ref, server_id, status, detail, user_id, conversation_id) "
                "VALUES (%s,%s,%s,%s,%s,%s,NULL)",
                (
                    ctx.class_id("teacher_b"),
                    ref_b,
                    server_b["id"],
                    "denied",
                    "class B audit fixture",
                    ctx.user_id("teacher_b"),
                ),
            )
        )

        response = ctx.get(
            "/api/mcp/tool-calls", user="teacher_a", params={"limit": 100}, expect=200
        )
        items = ctx.json(response).get("items")
        ctx.is_instance(items, list, "tool-calls items")
        ctx.check(
            all(item["class_id"] == ctx.class_id("teacher_a") for item in items),
            "class A tool-calls are class scoped",
            items[:5],
        )
        ctx.contains({item["id"] for item in items}, audit_ids[0], "class A audit present")
        ctx.not_contains({item["id"] for item in items}, audit_ids[1], "class B audit hidden")

        class_a_db = ctx.scalar(
            "SELECT COUNT(*) AS n FROM mcp_tool_calls WHERE class_id=%s",
            (ctx.class_id("teacher_a"),),
        )
        ctx.eq(len(items), min(100, class_a_db), "class A list length vs DB (limit 100)")

        other = ctx.get(
            "/api/mcp/tool-calls", user="teacher_b", params={"limit": 100}, expect=200
        )
        other_items = ctx.json(other).get("items") or []
        ctx.check(
            all(item["class_id"] == ctx.class_id("teacher_b") for item in other_items),
            "class B tool-calls are class scoped",
            other_items[:5],
        )
        ctx.contains({item["id"] for item in other_items}, audit_ids[1], "class B audit present")
        ctx.not_contains({item["id"] for item in other_items}, audit_ids[0], "class A audit hidden")

        limited = ctx.get(
            "/api/mcp/tool-calls", user="teacher_a", params={"limit": 1}, expect=200
        )
        ctx.eq(len(ctx.json(limited).get("items") or []), 1, "limit=1 returns one row")

        student = ctx.get("/api/mcp/tool-calls", user="student_a1", expect=403)
        ctx.assert_error(student, 403, "forbidden", "student tool-calls")

        unauth = ctx.get("/api/mcp/tool-calls", expect=401)
        ctx.assert_error(unauth, 401, "unauthorized", "tool-calls without token")
    finally:
        for audit_id in audit_ids:
            ctx.execute("DELETE FROM mcp_tool_calls WHERE id=%s", (audit_id,))
        for server in (server_a, server_b):
            ctx.delete(f"/api/mcp/servers/{server['id']}", user="teacher_a" if server["class_id"] == ctx.class_id("teacher_a") else "teacher_b")
            ctx.untrack("server", server["id"])
