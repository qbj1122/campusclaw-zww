"""POST /api/mcp/servers/{id}/connect — discovery, idempotence, bad token 502."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    server = ctx.create_server()
    bad = ctx.create_server(token=ctx.unique("wrong-token", max_len=40))
    try:
        response = ctx.post(
            f"/api/mcp/servers/{server['id']}/connect", user="teacher_a", expect=200
        )
        payload = ctx.json(response)
        updated = payload.get("server") or {}
        ctx.eq(updated.get("connected"), True, "connect marks server connected")
        ctx.eq(updated.get("tool_count"), 2, "connect discovers two tools")
        tools = payload.get("tools") or []
        names = {tool["name"] for tool in tools}
        ctx.eq(names, {"formula_hint", "unit_convert"}, "discovered tool names")
        for tool in tools:
            ctx.contains(tool.get("description", ""), "提示" if tool["name"] == "formula_hint" else "换算", "tool description")
            import json

            schema = json.loads(tool["input_schema"])
            ctx.eq(schema.get("type"), "object", f"{tool['name']} input_schema type")

        db_tools = ctx.query(
            "SELECT tool_name FROM mcp_tools WHERE server_id=%s", (server["id"],)
        )
        ctx.eq({row["tool_name"] for row in db_tools}, names, "DB cached tool names")
        ctx.eq(len(db_tools), 2, "DB cached tool count")
        ctx.eq(
            bool(ctx.scalar("SELECT connected FROM mcp_servers WHERE id=%s", (server["id"],))),
            True,
            "DB connected flag",
        )

        again = ctx.post(
            f"/api/mcp/servers/{server['id']}/connect", user="teacher_a", expect=200
        )
        again_payload = ctx.json(again)
        ctx.eq(again_payload.get("server", {}).get("tool_count"), 2, "second connect tool_count")
        ctx.eq(len(again_payload.get("tools") or []), 2, "second connect tool list")
        ctx.eq(
            ctx.scalar("SELECT COUNT(*) AS n FROM mcp_tools WHERE server_id=%s", (server["id"],)),
            2,
            "second connect keeps DB tool count at 2",
        )

        invalid = ctx.post(
            f"/api/mcp/servers/{bad['id']}/connect", user="teacher_a", expect=502
        )
        ctx.assert_error(invalid, 502, "bad_gateway", "invalid MCP token connect")
        ctx.eq(
            bool(ctx.scalar("SELECT connected FROM mcp_servers WHERE id=%s", (bad["id"],))),
            False,
            "failed connect leaves server disconnected",
        )
        ctx.eq(
            ctx.scalar("SELECT COUNT(*) AS n FROM mcp_tools WHERE server_id=%s", (bad["id"],)),
            0,
            "failed connect caches no tools",
        )

        cross = ctx.post(
            f"/api/mcp/servers/{server['id']}/connect", user="teacher_b", expect=404
        )
        ctx.assert_error(cross, 404, "not_found", "cross-class connect")

        student = ctx.post(
            f"/api/mcp/servers/{server['id']}/connect", user="student_a1", expect=403
        )
        ctx.assert_error(student, 403, "forbidden", "student connect")

        missing = ctx.post(
            "/api/mcp/servers/999999999/connect", user="teacher_a", expect=404
        )
        ctx.assert_error(missing, 404, "not_found", "missing server connect")
    finally:
        for server_id in (server["id"], bad["id"]):
            ctx.delete(f"/api/mcp/servers/{server_id}", user="teacher_a")
            ctx.untrack("server", server_id)
