"""POST /api/agents — create with bindings, duplicates and RBAC."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    material = ctx.ensure_material("agent-create")
    skill_name = ctx.unique("agent-skill")
    skill = ctx.create_skill(ctx.skill_source(skill_name, "智能体创建", ["knowledge_search"]))
    b_skill = ctx.create_skill(
        ctx.skill_source(ctx.unique("b-skill"), "B班技能", ["knowledge_search"]),
        user="teacher_b",
    )
    name = ctx.unique("agent-create")
    agent = None
    try:
        response = ctx.post(
            "/api/agents",
            user="teacher_a",
            expect=201,
            json={
                "name": name,
                "prompt": "创建智能体提示词",
                "enabled": True,
                "skill_ids": [skill["id"]],
                "material_ids": [material["id"]],
            },
        )
        payload = ctx.json(response)
        agent = payload.get("id")
        ctx.is_instance(agent, int, "created agent id")
        ctx.eq(payload.get("name"), name, "created agent name")
        ctx.eq(payload.get("prompt"), "创建智能体提示词", "created agent prompt")
        ctx.eq(payload.get("enabled"), True, "created agent enabled")
        ctx.eq(payload.get("skill_ids"), [skill["id"]], "created agent skill_ids")
        ctx.eq(payload.get("material_ids"), [material["id"]], "created agent material_ids")
        ctx.eq(payload.get("class_id"), ctx.class_id("teacher_a"), "created agent class")

        row = ctx.one(
            "SELECT class_id, name, prompt, enabled FROM agents WHERE id=%s", (agent,)
        )
        ctx.check(row is not None, "created agent row exists", agent)
        ctx.eq(row["class_id"], ctx.class_id("teacher_a"), "DB agent class_id")
        ctx.eq(row["name"], name, "DB agent name")
        ctx.eq(row["prompt"], "创建智能体提示词", "DB agent prompt")
        ctx.eq(bool(row["enabled"]), True, "DB agent enabled")
        ctx.eq(
            [r["skill_id"] for r in ctx.query("SELECT skill_id FROM agent_skills WHERE agent_id=%s", (agent,))],
            [skill["id"]],
            "DB agent_skills binding",
        )
        ctx.eq(
            [
                r["material_id"]
                for r in ctx.query(
                    "SELECT material_id FROM agent_materials WHERE agent_id=%s", (agent,)
                )
            ],
            [material["id"]],
            "DB agent_materials binding",
        )

        duplicate = ctx.post(
            "/api/agents",
            user="teacher_a",
            expect=400,
            json={
                "name": name,
                "prompt": "duplicate",
                "enabled": True,
                "skill_ids": [],
                "material_ids": [],
            },
        )
        ctx.assert_error(duplicate, 400, "bad_request", "duplicate agent name")

        cross_skill = ctx.post(
            "/api/agents",
            user="teacher_a",
            expect=400,
            json={
                "name": ctx.unique("agent-cross-skill"),
                "prompt": "x",
                "enabled": True,
                "skill_ids": [b_skill["id"]],
                "material_ids": [],
            },
        )
        ctx.assert_error(cross_skill, 400, "bad_request", "cross-class skill binding")

        b_material = ctx.one(
            "SELECT id FROM materials WHERE class_id=%s LIMIT 1",
            (ctx.class_id("teacher_b"),),
        )
        ctx.check(b_material is not None, "class B seed material exists")
        cross_material = ctx.post(
            "/api/agents",
            user="teacher_a",
            expect=400,
            json={
                "name": ctx.unique("agent-cross-mat"),
                "prompt": "x",
                "enabled": True,
                "skill_ids": [],
                "material_ids": [b_material["id"]],
            },
        )
        ctx.assert_error(cross_material, 400, "bad_request", "cross-class material binding")

        empty_name = ctx.post(
            "/api/agents",
            user="teacher_a",
            expect=400,
            json={"name": " ", "prompt": "x", "skill_ids": [], "material_ids": []},
        )
        ctx.assert_error(empty_name, 400, "bad_request", "empty agent name")

        student = ctx.post(
            "/api/agents",
            user="student_a1",
            expect=403,
            json={"name": ctx.unique("student-agent"), "prompt": "x"},
        )
        ctx.assert_error(student, 403, "forbidden", "student create agent")
    finally:
        if agent is not None:
            ctx.remove_agent(agent)
        ctx.remove_skill(skill["id"])
        ctx.remove_skill(b_skill["id"])
