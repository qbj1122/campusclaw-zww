"""DELETE /api/mcp/servers/{id} — plain delete, audit 503 and RBAC."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    plain = ctx.create_server()
    audited = ctx.create_server()
    audit_id = None
    try:
        student = ctx.delete(
            f"/api/mcp/servers/{plain['id']}", user="student_a1", expect=403
        )
        ctx.assert_error(student, 403, "forbidden", "student server delete")

        cross = ctx.delete(
            f"/api/mcp/servers/{plain['id']}", user="teacher_b", expect=404
        )
        ctx.assert_error(cross, 404, "not_found", "cross-class server delete")

        response = ctx.delete(
            f"/api/mcp/servers/{plain['id']}", user="teacher_a", expect=204
        )
        ctx.eq(response.text, "", "plain server delete body")
        ctx.check(
            ctx.one("SELECT id FROM mcp_servers WHERE id=%s", (plain["id"],)) is None,
            "plain server removed from DB",
        )
        ctx.untrack("server", plain["id"])

        audit_id = ctx.insert(
            "INSERT INTO mcp_tool_calls"
            "(class_id, tool_ref, server_id, status, detail, user_id, conversation_id) "
            "VALUES (%s,%s,%s,%s,%s,%s,NULL)",
            (
                ctx.class_id("teacher_a"),
                f"formula_hint@{audited['name']}",
                audited["id"],
                "ok",
                "audit evidence for delete test",
                ctx.user_id("teacher_a"),
            ),
        )
        blocked = ctx.delete(
            f"/api/mcp/servers/{audited['id']}", user="teacher_a", expect=503
        )
        ctx.assert_error(blocked, 503, "unavailable", "server delete with audit")
        ctx.check(
            ctx.one("SELECT id FROM mcp_servers WHERE id=%s", (audited["id"],)) is not None,
            "audited server preserved",
        )
        ctx.check(
            ctx.one("SELECT id FROM mcp_tool_calls WHERE id=%s", (audit_id,)) is not None,
            "audit row preserved",
        )
    finally:
        if audit_id is not None:
            ctx.execute("DELETE FROM mcp_tool_calls WHERE id=%s", (audit_id,))
        ctx.remove_server(audited["id"])
        ctx.delete(f"/api/mcp/servers/{plain['id']}", user="teacher_a")
        ctx.untrack("server", plain["id"])
