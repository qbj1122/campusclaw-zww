"""GET /api/mcp/servers/{id}/tools — cached discovery and class isolation."""

import json

from lib import Ctx


def run(ctx: Ctx) -> None:
    server = ctx.create_server()
    try:
        connect = ctx.post(
            f"/api/mcp/servers/{server['id']}/connect", user="teacher_a", expect=200
        )
        ctx.eq(ctx.json(connect).get("server", {}).get("tool_count"), 2, "precondition connect")

        response = ctx.get(
            f"/api/mcp/servers/{server['id']}/tools", user="teacher_a", expect=200
        )
        payload = ctx.json(response)
        ctx.eq(payload.get("server", {}).get("id"), server["id"], "tools.server.id")
        ctx.eq(payload.get("server", {}).get("connected"), True, "tools.server.connected")
        tools = payload.get("tools")
        ctx.is_instance(tools, list, "tools list")
        ctx.eq({tool["name"] for tool in tools}, {"formula_hint", "unit_convert"}, "tools names")
        for tool in tools:
            ctx.eq(tool["server_id"], server["id"], "tool.server_id")
            ctx.check(bool(tool.get("description")), "tool description non-empty")
            schema = json.loads(tool["input_schema"])
            ctx.check("properties" in schema, "tool input_schema properties", schema)

        cross = ctx.get(
            f"/api/mcp/servers/{server['id']}/tools", user="teacher_b", expect=404
        )
        ctx.assert_error(cross, 404, "not_found", "cross-class tools")

        student = ctx.get(
            f"/api/mcp/servers/{server['id']}/tools", user="student_a1", expect=403
        )
        ctx.assert_error(student, 403, "forbidden", "student tools")

        missing = ctx.get("/api/mcp/servers/999999999/tools", user="teacher_a", expect=404)
        ctx.assert_error(missing, 404, "not_found", "missing server tools")
    finally:
        ctx.delete(f"/api/mcp/servers/{server['id']}", user="teacher_a")
        ctx.untrack("server", server["id"])
