"""GET /api/mcp/servers — teacher-only list that never leaks tokens."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    server = ctx.create_server()
    try:
        response = ctx.get("/api/mcp/servers", user="teacher_a", expect=200)
        raw = ctx.safe_text(response)
        payload = ctx.json(response)
        items = payload.get("items")
        ctx.is_instance(items, list, "mcp servers items")
        ids = {item["id"] for item in items}
        ctx.contains(ids, server["id"], "created server in list")

        listed = next(item for item in items if item["id"] == server["id"])
        ctx.eq(listed.get("name"), server["name"], "server name in list")
        ctx.eq(listed.get("has_auth"), True, "server has_auth")
        ctx.eq(listed.get("connected"), False, "new server not connected")
        ctx.eq(listed.get("tool_count"), 0, "new server has no tools")
        ctx.not_contains(raw, ctx.mcp_token, "MCP token not present in response")
        ctx.not_contains(raw, '"auth_token"', "auth_token field not present in response")
        ctx.check(
            all(item["class_id"] == ctx.class_id("teacher_a") for item in items),
            "MCP server list class scoped",
            items,
        )

        db = ctx.one(
            "SELECT class_id, name, base_url, auth_token FROM mcp_servers WHERE id=%s",
            (server["id"],),
        )
        ctx.eq(db["class_id"], listed["class_id"], "DB server class")
        ctx.eq(db["name"], listed["name"], "DB server name")
        ctx.eq(db["auth_token"], ctx.mcp_token, "DB stores the supplied token")

        other = ctx.get("/api/mcp/servers", user="teacher_b", expect=200)
        ctx.not_contains(
            {item["id"] for item in ctx.json(other).get("items", [])},
            server["id"],
            "class B cannot list class A server",
        )

        student = ctx.get("/api/mcp/servers", user="student_a1", expect=403)
        ctx.assert_error(student, 403, "forbidden", "student MCP list")

        unauth = ctx.get("/api/mcp/servers", expect=401)
        ctx.assert_error(unauth, 401, "unauthorized", "MCP list without token")
    finally:
        deleted = ctx.delete(f"/api/mcp/servers/{server['id']}", user="teacher_a")
        ctx.check(
            deleted.status_code == 204,
            "cleanup server delete returns 204",
            ctx.safe_text(deleted),
        )
        ctx.untrack("server", server["id"])
