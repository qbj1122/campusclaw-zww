"""PUT /api/skills/{id} — update, duplicate rejection and class isolation."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    server = ctx.connected_server()
    first_name = ctx.unique("skill-upd-a")
    second_name = ctx.unique("skill-upd-b")
    first = ctx.create_skill(ctx.skill_source(first_name, "更新前", ["knowledge_search"]))
    second = ctx.create_skill(ctx.skill_source(second_name, "占位", ["knowledge_search"]))
    try:
        new_name = ctx.unique("skill-updated")
        response = ctx.put(
            f"/api/skills/{first['id']}",
            user="teacher_a",
            expect=200,
            json={
                "source_text": ctx.skill_source(
                    new_name,
                    "更新后,方程",
                    ["knowledge_search", f"unit_convert@{server['name']}"],
                    description="更新后的技能说明正文。",
                )
            },
        )
        payload = ctx.json(response)
        ctx.eq(payload.get("id"), first["id"], "update skill id")
        ctx.eq(payload.get("name"), new_name, "update skill name")
        ctx.eq(payload.get("trigger_text"), "更新后,方程", "update skill trigger")
        ctx.contains(payload.get("tools", []), f"unit_convert@{server['name']}", "update tools")

        row = ctx.one(
            "SELECT name, trigger_text, description, tools_json FROM skills WHERE id=%s",
            (first["id"],),
        )
        ctx.eq(row["name"], new_name, "DB updated skill name")
        ctx.eq(row["trigger_text"], "更新后,方程", "DB updated trigger")
        ctx.eq(row["description"], "更新后的技能说明正文。", "DB updated description")
        ctx.contains(row["tools_json"], "unit_convert@", "DB updated tools")

        duplicate = ctx.put(
            f"/api/skills/{first['id']}",
            user="teacher_a",
            expect=400,
            json={"source_text": ctx.skill_source(second_name, "冲突", ["knowledge_search"])},
        )
        ctx.assert_error(duplicate, 400, "bad_request", "update to duplicate name")

        unknown = ctx.put(
            f"/api/skills/{first['id']}",
            user="teacher_a",
            expect=400,
            json={
                "source_text": ctx.skill_source(
                    new_name, "更新后", [f"nope@{server['name']}"]
                )
            },
        )
        ctx.assert_error(unknown, 400, "bad_request", "update with unknown tool")

        cross = ctx.put(
            f"/api/skills/{first['id']}",
            user="teacher_b",
            expect=404,
            json={"source_text": ctx.skill_source(ctx.unique("cross"), "跨班", ["knowledge_search"])},
        )
        ctx.assert_error(cross, 404, "not_found", "cross-class skill update")

        student = ctx.put(
            f"/api/skills/{first['id']}",
            user="student_a1",
            expect=403,
            json={"source_text": ctx.skill_source(ctx.unique("student"), "学生", ["knowledge_search"])},
        )
        ctx.assert_error(student, 403, "forbidden", "student skill update")

        missing = ctx.put(
            "/api/skills/999999999",
            user="teacher_a",
            expect=404,
            json={"source_text": ctx.skill_source(ctx.unique("missing"), "不存在", ["knowledge_search"])},
        )
        ctx.assert_error(missing, 404, "not_found", "missing skill update")
    finally:
        ctx.remove_skill(first["id"])
        ctx.remove_skill(second["id"])
