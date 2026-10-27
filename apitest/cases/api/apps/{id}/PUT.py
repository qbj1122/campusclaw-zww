"""PUT /api/apps/{id} — update, validation, duplicates and class isolation."""

import json

from lib import Ctx


def run(ctx: Ctx) -> None:
    material = ctx.ensure_material("app-update")
    template = ctx.create_prompt_template(
        ctx.unique("app-upd-tpl"), "更新前模板 {{agent_name}}"
    )
    new_template = ctx.create_prompt_template(
        ctx.unique("app-upd-tpl2"), "更新后模板 {{class_name}}"
    )
    skill = ctx.create_skill(
        ctx.skill_source(ctx.unique("app-upd-skill"), "应用更新", ["knowledge_search"])
    )
    b_skill = ctx.create_skill(
        ctx.skill_source(ctx.unique("app-upd-bskill"), "B 班技能", ["knowledge_search"]),
        user="teacher_b",
    )
    b_template = ctx.create_prompt_template(
        ctx.unique("app-upd-btpl"), "B 班模板 {{date}}", user="teacher_b"
    )
    b_material = ctx.one(
        "SELECT id FROM materials WHERE class_id=%s ORDER BY id LIMIT 1",
        (ctx.class_id("teacher_b"),),
    )
    ctx.check(b_material is not None, "class B seed material exists")
    app = ctx.create_app(
        ctx.unique("app-update"),
        prompt_template_id=template["id"],
        skill_ids=[skill["id"]],
        material_ids=[material["id"]],
    )
    other = ctx.create_app(ctx.unique("app-update-other"))
    agent = None
    try:
        instantiated = ctx.post(
            f"/api/apps/{app['id']}/instantiate", user="teacher_a", expect=201, json={}
        )
        agent = ctx.json(instantiated).get("id")
        ctx.track("agent", agent, {})

        new_name = ctx.unique("app-updated")
        response = ctx.put(
            f"/api/apps/{app['id']}",
            user="teacher_a",
            expect=200,
            json={
                "name": new_name,
                "description": "更新后的应用说明",
                "category": "复习",
                "icon": "🧭",
                "prompt_template_id": new_template["id"],
                "default_prompt": "更新后的默认提示词",
                "skill_ids": [skill["id"]],
                "material_ids": [material["id"]],
                "enabled": False,
            },
        )
        payload = ctx.json(response)
        ctx.eq(payload.get("id"), app["id"], "updated app id")
        ctx.eq(payload.get("name"), new_name, "updated app name")
        ctx.eq(payload.get("description"), "更新后的应用说明", "updated app description")
        ctx.eq(payload.get("category"), "复习", "updated app category")
        ctx.eq(payload.get("icon"), "🧭", "updated app icon")
        ctx.eq(payload.get("prompt_template_id"), new_template["id"], "updated app template")
        ctx.eq(payload.get("default_prompt"), "更新后的默认提示词", "updated app default prompt")
        ctx.eq(payload.get("default_skill_ids"), [skill["id"]], "updated app skill ids")
        ctx.eq(payload.get("default_material_ids"), [material["id"]], "updated app material ids")
        ctx.eq(payload.get("enabled"), False, "updated app enabled")
        # UpdateAIApp does not reload agent_count; the persisted/detail value is
        # authoritative.  The response omits it as 0 (see README findings).
        detail_after_update = ctx.get(f"/api/apps/{app['id']}", user="teacher_a", expect=200)
        ctx.eq(
            ctx.json(detail_after_update).get("agent_count"),
            1,
            "updated app detail agent_count",
        )

        row = ctx.one(
            "SELECT name, description, category, icon, prompt_template_id, default_prompt, "
            "default_skill_ids_json, default_material_ids_json, enabled "
            "FROM ai_apps WHERE id=%s",
            (app["id"],),
        )
        ctx.eq(row["name"], new_name, "DB updated app name")
        ctx.eq(row["description"], "更新后的应用说明", "DB updated app description")
        ctx.eq(row["category"], "复习", "DB updated app category")
        ctx.eq(row["icon"], "🧭", "DB updated app icon")
        ctx.eq(row["prompt_template_id"], new_template["id"], "DB updated app template")
        ctx.eq(row["default_prompt"], "更新后的默认提示词", "DB updated default_prompt")
        ctx.eq(json.loads(row["default_skill_ids_json"]), [skill["id"]], "DB updated skills")
        ctx.eq(
            json.loads(row["default_material_ids_json"]),
            [material["id"]],
            "DB updated materials",
        )
        ctx.eq(bool(row["enabled"]), False, "DB updated app enabled")

        hidden = ctx.get(f"/api/apps/{app['id']}", user="student_a1", expect=404)
        ctx.assert_error(hidden, 404, "not_found", "updated disabled app hidden from student")
        teacher_view = ctx.get(f"/api/apps/{app['id']}", user="teacher_a", expect=200)
        ctx.eq(
            ctx.json(teacher_view).get("prompt_template_name"),
            new_template["name"],
            "teacher detail after update template name",
        )

        duplicate = ctx.put(
            f"/api/apps/{app['id']}",
            user="teacher_a",
            expect=400,
            json={"name": other["name"], "default_prompt": "duplicate"},
        )
        ctx.assert_error(duplicate, 400, "bad_request", "app update duplicate name")
        ctx.eq(
            ctx.scalar("SELECT name FROM ai_apps WHERE id=%s", (app["id"],)),
            new_name,
            "duplicate update left app name unchanged",
        )

        cross_template = ctx.put(
            f"/api/apps/{app['id']}",
            user="teacher_a",
            expect=400,
            json={
                "name": new_name,
                "prompt_template_id": b_template["id"],
                "default_prompt": "x",
            },
        )
        ctx.assert_error(cross_template, 400, "bad_request", "update cross-class template")

        cross_skill = ctx.put(
            f"/api/apps/{app['id']}",
            user="teacher_a",
            expect=400,
            json={"name": new_name, "default_prompt": "x", "skill_ids": [b_skill["id"]]},
        )
        ctx.assert_error(cross_skill, 400, "bad_request", "update cross-class skill")

        cross_material = ctx.put(
            f"/api/apps/{app['id']}",
            user="teacher_a",
            expect=400,
            json={
                "name": new_name,
                "default_prompt": "x",
                "material_ids": [b_material["id"]],
            },
        )
        ctx.assert_error(cross_material, 400, "bad_request", "update cross-class material")

        cross = ctx.put(
            f"/api/apps/{app['id']}",
            user="teacher_b",
            expect=404,
            json={"name": ctx.unique("cross-app"), "default_prompt": "x"},
        )
        ctx.assert_error(cross, 404, "not_found", "cross-class app update")

        student = ctx.put(
            f"/api/apps/{app['id']}",
            user="student_a1",
            expect=403,
            json={"name": ctx.unique("student-app"), "default_prompt": "x"},
        )
        ctx.assert_error(student, 403, "forbidden", "student app update")

        missing = ctx.put(
            "/api/apps/999999999",
            user="teacher_a",
            expect=404,
            json={"name": ctx.unique("missing-app"), "default_prompt": "x"},
        )
        ctx.assert_error(missing, 404, "not_found", "missing app update")
    finally:
        if agent is not None:
            ctx.remove_agent(agent)
        ctx.remove_app(app["id"])
        ctx.remove_app(other["id"])
        ctx.remove_skill(skill["id"])
        ctx.remove_skill(b_skill["id"])
        ctx.remove_prompt_template(template["id"])
        ctx.remove_prompt_template(new_template["id"])
        ctx.remove_prompt_template(b_template["id"])
