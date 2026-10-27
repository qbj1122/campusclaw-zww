"""PUT /api/agents/{id} — update, binding replacement and RBAC."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    material = ctx.ensure_material("agent-update")
    skill = ctx.create_skill(
        ctx.skill_source(ctx.unique("agent-upd-skill"), "更新技能", ["knowledge_search"])
    )
    agent = ctx.create_agent(ctx.unique("agent-update"), skill_ids=[skill["id"]])
    second = ctx.create_agent(ctx.unique("agent-update-b"))
    try:
        new_name = ctx.unique("agent-updated")
        response = ctx.put(
            f"/api/agents/{agent['id']}",
            user="teacher_a",
            expect=200,
            json={
                "name": new_name,
                "prompt": "更新后的智能体提示词",
                "enabled": False,
                "skill_ids": [skill["id"]],
                "material_ids": [material["id"]],
            },
        )
        payload = ctx.json(response)
        ctx.eq(payload.get("id"), agent["id"], "updated agent id")
        ctx.eq(payload.get("name"), new_name, "updated agent name")
        ctx.eq(payload.get("prompt"), "更新后的智能体提示词", "updated agent prompt")
        ctx.eq(payload.get("enabled"), False, "updated agent enabled")
        ctx.eq(payload.get("material_ids"), [material["id"]], "updated agent materials")

        row = ctx.one(
            "SELECT name, prompt, enabled FROM agents WHERE id=%s", (agent["id"],)
        )
        ctx.eq(row["name"], new_name, "DB updated agent name")
        ctx.eq(row["prompt"], "更新后的智能体提示词", "DB updated prompt")
        ctx.eq(bool(row["enabled"]), False, "DB updated enabled")
        ctx.eq(
            ctx.scalar(
                "SELECT COUNT(*) AS n FROM agent_materials WHERE agent_id=%s AND material_id=%s",
                (agent["id"], material["id"]),
            ),
            1,
            "DB updated material binding",
        )

        student = ctx.get("/api/agents", user="student_a1", expect=200)
        ctx.not_contains(
            {item["id"] for item in ctx.json(student).get("items", [])},
            agent["id"],
            "disabled agent hidden from student",
        )

        duplicate = ctx.put(
            f"/api/agents/{agent['id']}",
            user="teacher_a",
            expect=400,
            json={
                "name": second["name"],
                "prompt": "duplicate",
                "enabled": True,
                "skill_ids": [],
                "material_ids": [],
            },
        )
        ctx.assert_error(duplicate, 400, "bad_request", "update to duplicate agent name")

        invalid_binding = ctx.put(
            f"/api/agents/{agent['id']}",
            user="teacher_a",
            expect=400,
            json={
                "name": new_name,
                "prompt": "x",
                "enabled": True,
                "skill_ids": [999999999],
                "material_ids": [],
            },
        )
        ctx.assert_error(invalid_binding, 400, "bad_request", "update with missing skill")

        cross = ctx.put(
            f"/api/agents/{agent['id']}",
            user="teacher_b",
            expect=404,
            json={
                "name": ctx.unique("cross-agent"),
                "prompt": "x",
                "enabled": True,
                "skill_ids": [],
                "material_ids": [],
            },
        )
        ctx.assert_error(cross, 404, "not_found", "cross-class agent update")

        student_put = ctx.put(
            f"/api/agents/{agent['id']}",
            user="student_a1",
            expect=403,
            json={"name": ctx.unique("student-agent"), "prompt": "x"},
        )
        ctx.assert_error(student_put, 403, "forbidden", "student agent update")

        missing = ctx.put(
            "/api/agents/999999999",
            user="teacher_a",
            expect=404,
            json={"name": ctx.unique("missing-agent"), "prompt": "x"},
        )
        ctx.assert_error(missing, 404, "not_found", "missing agent update")
    finally:
        ctx.remove_agent(agent["id"])
        ctx.remove_agent(second["id"])
        ctx.remove_skill(skill["id"])
