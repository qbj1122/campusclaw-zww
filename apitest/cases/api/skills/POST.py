"""POST /api/skills — create, validation, duplicates and MCP tool refs."""

import json

from lib import Ctx


def run(ctx: Ctx) -> None:
    # A connected server is required before a tool@server reference can be saved.
    server = ctx.connected_server()
    name = ctx.unique("skill-create")
    created = []
    try:
        response = ctx.post(
            "/api/skills",
            user="teacher_a",
            expect=201,
            json={"source_text": ctx.skill_source(name, "创建测试,方程", ["knowledge_search"])},
        )
        payload = ctx.json(response)
        skill_id = payload.get("id")
        ctx.is_instance(skill_id, int, "create skill id")
        created.append(skill_id)
        ctx.eq(payload.get("name"), name, "create skill name")
        ctx.eq(payload.get("trigger_text"), "创建测试,方程", "create skill trigger")
        ctx.eq(payload.get("tools"), ["knowledge_search"], "create skill tools")
        ctx.eq(payload.get("class_id"), ctx.class_id("teacher_a"), "create skill class_id")
        ctx.eq(payload.get("enabled"), True, "create skill enabled")

        row = ctx.one(
            "SELECT class_id, name, trigger_text, tools_json, enabled, source_text "
            "FROM skills WHERE id=%s",
            (skill_id,),
        )
        ctx.check(row is not None, "created skill row exists", skill_id)
        ctx.eq(row["class_id"], payload["class_id"], "DB skill class_id")
        ctx.eq(row["name"], name, "DB skill name")
        ctx.eq(row["trigger_text"], payload["trigger_text"], "DB skill trigger")
        ctx.eq(json.loads(row["tools_json"]), ["knowledge_search"], "DB skill tools")
        ctx.eq(bool(row["enabled"]), True, "DB skill enabled")
        ctx.contains(row["source_text"], name, "DB skill source_text")

        duplicate = ctx.post(
            "/api/skills",
            user="teacher_a",
            expect=400,
            json={
                "source_text": ctx.skill_source(name, "另一个触发词", ["knowledge_search"])
            },
        )
        ctx.assert_error(duplicate, 400, "bad_request", "duplicate skill name")
        count = ctx.scalar("SELECT COUNT(*) AS n FROM skills WHERE class_id=%s AND name=%s",
                           (ctx.class_id("teacher_a"), name))
        ctx.eq(count, 1, "duplicate did not create a second DB row")

        missing_enabled = (
            f"名称：{ctx.unique('bad-skill')}\n适用条件：方程\n工具：knowledge_search\n\n正文"
        )
        invalid = ctx.post(
            "/api/skills", user="teacher_a", expect=400, json={"source_text": missing_enabled}
        )
        ctx.assert_error(invalid, 400, "bad_request", "skill missing 启用")

        no_body = f"名称：{ctx.unique('bad-body')}\n适用条件：方程\n启用：是"
        invalid = ctx.post(
            "/api/skills", user="teacher_a", expect=400, json={"source_text": no_body}
        )
        ctx.assert_error(invalid, 400, "bad_request", "skill without description body")

        unknown_tool = ctx.skill_source(
            ctx.unique("unknown-tool"),
            "未知工具",
            [f"does_not_exist@{server['name']}"],
        )
        invalid = ctx.post(
            "/api/skills", user="teacher_a", expect=400, json={"source_text": unknown_tool}
        )
        ctx.assert_error(invalid, 400, "bad_request", "unknown MCP tool")

        unknown_server = ctx.skill_source(
            ctx.unique("unknown-server"),
            "未知服务器",
            [f"formula_hint@{ctx.unique('missing-server')}"],
        )
        invalid = ctx.post(
            "/api/skills", user="teacher_a", expect=400, json={"source_text": unknown_server}
        )
        ctx.assert_error(invalid, 400, "bad_request", "unknown MCP server")

        tool_name = ctx.unique("skill-tool")
        tool_ref = f"formula_hint@{server['name']}"
        with_tool = ctx.post(
            "/api/skills",
            user="teacher_a",
            expect=201,
            json={
                "source_text": ctx.skill_source(
                    tool_name, "工具测试,方程", ["knowledge_search", tool_ref]
                )
            },
        )
        tool_payload = ctx.json(with_tool)
        created.append(tool_payload["id"])
        ctx.contains(tool_payload.get("tools", []), tool_ref, "saved tool@server ref")
        row = ctx.one("SELECT tools_json FROM skills WHERE id=%s", (tool_payload["id"],))
        ctx.contains(json.loads(row["tools_json"]), tool_ref, "DB tools_json has tool@server")

        student = ctx.post(
            "/api/skills",
            user="student_a1",
            expect=403,
            json={"source_text": ctx.skill_source(ctx.unique("student-skill"), "学生")},
        )
        ctx.assert_error(student, 403, "forbidden", "student create skill")
    finally:
        for skill_id in created:
            ctx.remove_skill(skill_id)
