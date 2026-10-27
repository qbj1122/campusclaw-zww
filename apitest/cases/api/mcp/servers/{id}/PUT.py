"""PUT /api/mcp/servers/{id} — update clears connection/cache, token optional."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    server = ctx.create_server()
    other = ctx.create_server()
    try:
        connected = ctx.post(
            f"/api/mcp/servers/{server['id']}/connect", user="teacher_a", expect=200
        )
        ctx.eq(ctx.json(connected).get("server", {}).get("connected"), True, "precondition connected")

        # Omitting auth_token must keep the stored token but clear the cache.
        response = ctx.put(
            f"/api/mcp/servers/{server['id']}",
            user="teacher_a",
            expect=200,
            json={
                "name": server["name"],
                "base_url": ctx.mcp_base_url,
            },
        )
        payload = ctx.json(response)
        ctx.eq(payload.get("id"), server["id"], "updated server id")
        ctx.eq(payload.get("has_auth"), True, "omitted token keeps has_auth")
        ctx.eq(payload.get("connected"), False, "update clears connected")
        ctx.eq(payload.get("tool_count"), 0, "update clears tool_count")

        row = ctx.one(
            "SELECT auth_token, connected FROM mcp_servers WHERE id=%s", (server["id"],)
        )
        ctx.eq(row["auth_token"], ctx.mcp_token, "omitted token keeps DB token")
        ctx.eq(bool(row["connected"]), False, "DB connected cleared")
        tools = ctx.scalar(
            "SELECT COUNT(*) AS n FROM mcp_tools WHERE server_id=%s", (server["id"],)
        )
        ctx.eq(tools, 0, "update clears cached tools")

        # Reconnect, then supply the token explicitly.
        reconnect = ctx.post(
            f"/api/mcp/servers/{server['id']}/connect", user="teacher_a", expect=200
        )
        ctx.eq(ctx.json(reconnect).get("server", {}).get("tool_count"), 2, "reconnect tools")
        updated = ctx.put(
            f"/api/mcp/servers/{server['id']}",
            user="teacher_a",
            expect=200,
            json={
                "name": server["name"],
                "base_url": ctx.mcp_base_url,
                "auth_token": ctx.mcp_token,
            },
        )
        ctx.eq(ctx.json(updated).get("has_auth"), True, "explicit token has_auth")

        duplicate = ctx.put(
            f"/api/mcp/servers/{server['id']}",
            user="teacher_a",
            expect=400,
            json={"name": other["name"], "base_url": ctx.mcp_base_url},
        )
        ctx.assert_error(duplicate, 400, "bad_request", "update to duplicate server name")

        cross = ctx.put(
            f"/api/mcp/servers/{server['id']}",
            user="teacher_b",
            expect=404,
            json={"name": ctx.unique("cross-mcp"), "base_url": ctx.mcp_base_url},
        )
        ctx.assert_error(cross, 404, "not_found", "cross-class server update")

        student = ctx.put(
            f"/api/mcp/servers/{server['id']}",
            user="student_a1",
            expect=403,
            json={"name": ctx.unique("student-mcp"), "base_url": ctx.mcp_base_url},
        )
        ctx.assert_error(student, 403, "forbidden", "student server update")

        missing = ctx.put(
            "/api/mcp/servers/999999999",
            user="teacher_a",
            expect=404,
            json={"name": ctx.unique("missing-mcp"), "base_url": ctx.mcp_base_url},
        )
        ctx.assert_error(missing, 404, "not_found", "missing server update")
    finally:
        for server_id in (server["id"], other["id"]):
            ctx.delete(f"/api/mcp/servers/{server_id}", user="teacher_a")
            ctx.untrack("server", server_id)
