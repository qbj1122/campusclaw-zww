"""POST /api/mcp/servers — registration, validation and duplicate names."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    name = ctx.unique("mcp-register")
    server = None
    try:
        response = ctx.post(
            "/api/mcp/servers",
            user="teacher_a",
            expect=201,
            json={
                "name": name,
                "base_url": ctx.mcp_base_url,
                "auth_token": ctx.mcp_token,
            },
        )
        raw = ctx.safe_text(response)
        payload = ctx.json(response)
        server = payload.get("id")
        ctx.is_instance(server, int, "registered server id")
        ctx.eq(payload.get("name"), name, "registered name")
        ctx.eq(payload.get("class_id"), ctx.class_id("teacher_a"), "registered class")
        ctx.eq(payload.get("has_auth"), True, "registered has_auth")
        ctx.eq(payload.get("connected"), False, "registered not connected")
        ctx.eq(payload.get("tool_count"), 0, "registered tool_count")
        ctx.not_contains(raw, ctx.mcp_token, "token absent from registration response")
        ctx.not_contains(raw, '"auth_token"', "auth_token absent from registration response")

        row = ctx.one(
            "SELECT class_id, name, base_url, auth_token FROM mcp_servers WHERE id=%s",
            (server,),
        )
        ctx.check(row is not None, "registered server row exists", server)
        ctx.eq(row["class_id"], ctx.class_id("teacher_a"), "DB server class")
        ctx.eq(row["name"], name, "DB server name")
        ctx.eq(row["base_url"], ctx.mcp_base_url, "DB server base_url")
        ctx.eq(row["auth_token"], ctx.mcp_token, "DB server token")

        duplicate = ctx.post(
            "/api/mcp/servers",
            user="teacher_a",
            expect=400,
            json={
                "name": name,
                "base_url": ctx.mcp_base_url,
                "auth_token": ctx.mcp_token,
            },
        )
        ctx.assert_error(duplicate, 400, "bad_request", "duplicate server name")

        bad_name = ctx.post(
            "/api/mcp/servers",
            user="teacher_a",
            expect=400,
            json={"name": "Bad Name", "base_url": ctx.mcp_base_url},
        )
        ctx.assert_error(bad_name, 400, "bad_request", "invalid server name")

        bad_url = ctx.post(
            "/api/mcp/servers",
            user="teacher_a",
            expect=400,
            json={"name": ctx.unique("bad-url"), "base_url": "not-a-url"},
        )
        ctx.assert_error(bad_url, 400, "bad_request", "invalid base_url")

        student = ctx.post(
            "/api/mcp/servers",
            user="student_a1",
            expect=403,
            json={"name": ctx.unique("student-mcp"), "base_url": ctx.mcp_base_url},
        )
        ctx.assert_error(student, 403, "forbidden", "student server registration")
    finally:
        if server is not None:
            ctx.delete(f"/api/mcp/servers/{server}", user="teacher_a")
            ctx.untrack("server", server)
