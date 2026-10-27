"""POST /api/apps — create, defaults, class validation, duplicates and RBAC."""

import json

from lib import Ctx


def run(ctx: Ctx) -> None:
    material = ctx.ensure_material("app-create")
    template = ctx.create_prompt_template(
        ctx.unique("app-create-tpl"), "应用创建模板 {{agent_name}}"
    )
    b_template = ctx.create_prompt_template(
        ctx.unique("app-b-tpl"), "B 班模板 {{class_name}}", user="teacher_b"
    )
    skill = ctx.create_skill(
        ctx.skill_source(ctx.unique("app-create-skill"), "应用创建", ["knowledge_search"])
    )
    b_skill = ctx.create_skill(
        ctx.skill_source(ctx.unique("app-b-skill"), "B 班技能", ["knowledge_search"]),
        user="teacher_b",
    )
    b_material = ctx.one(
        "SELECT id FROM materials WHERE class_id=%s ORDER BY id LIMIT 1",
        (ctx.class_id("teacher_b"),),
    )
    ctx.check(b_material is not None, "class B seed material exists")
    app = None
    minimal = None
    try:
        name = ctx.unique("app-create")
        app = ctx.create_app(
            name,
            prompt_template_id=template["id"],
            default_prompt="应用默认提示词",
            skill_ids=[skill["id"]],
            material_ids=[material["id"]],
            description="创建应用",
            category="教学",
            icon="📚",
        )
        ctx.eq(app.get("name"), name, "created app name")
        ctx.eq(app.get("description"), "创建应用", "created app description")
        ctx.eq(app.get("category"), "教学", "created app category")
        ctx.eq(app.get("icon"), "📚", "created app icon")
        ctx.eq(app.get("class_id"), ctx.class_id("teacher_a"), "created app class")
        ctx.eq(app.get("prompt_template_id"), template["id"], "created app template id")
        ctx.eq(app.get("default_prompt"), "应用默认提示词", "created app default prompt")
        ctx.eq(app.get("default_skill_ids"), [skill["id"]], "created app skill ids")
        ctx.eq(app.get("default_material_ids"), [material["id"]], "created app material ids")
        ctx.eq(app.get("enabled"), True, "created app enabled")
        ctx.eq(app.get("agent_count"), 0, "created app agent_count")

        row = ctx.one(
            "SELECT class_id, name, description, category, icon, prompt_template_id, "
            "default_prompt, default_skill_ids_json, default_material_ids_json, enabled "
            "FROM ai_apps WHERE id=%s",
            (app["id"],),
        )
        ctx.check(row is not None, "created app row exists", app["id"])
        ctx.eq(row["class_id"], ctx.class_id("teacher_a"), "DB app class_id")
        ctx.eq(row["name"], name, "DB app name")
        ctx.eq(row["description"], "创建应用", "DB app description")
        ctx.eq(row["category"], "教学", "DB app category")
        ctx.eq(row["icon"], "📚", "DB app icon")
        ctx.eq(row["prompt_template_id"], template["id"], "DB app template id")
        ctx.eq(row["default_prompt"], "应用默认提示词", "DB app default_prompt")
        ctx.eq(
            json.loads(row["default_skill_ids_json"]),
            [skill["id"]],
            "DB app skill ids",
        )
        ctx.eq(
            json.loads(row["default_material_ids_json"]),
            [material["id"]],
            "DB app material ids",
        )
        ctx.eq(bool(row["enabled"]), True, "DB app enabled")

        minimal_response = ctx.post(
            "/api/apps",
            user="teacher_a",
            expect=201,
            json={"name": ctx.unique("app-minimal"), "default_prompt": ""},
        )
        minimal_payload = ctx.json(minimal_response)
        minimal = minimal_payload.get("id")
        ctx.track("app", minimal, minimal_payload)
        ctx.eq(minimal_payload.get("category"), "通用", "minimal app default category")
        ctx.eq(minimal_payload.get("icon"), "✨", "minimal app default icon")
        ctx.eq(minimal_payload.get("enabled"), True, "minimal app default enabled")
        ctx.eq(
            minimal_payload.get("default_skill_ids") or [],
            [],
            "minimal app default skills",
        )
        ctx.eq(
            minimal_payload.get("default_material_ids") or [],
            [],
            "minimal app default materials",
        )
        minimal_row = ctx.one(
            "SELECT default_skill_ids_json, default_material_ids_json "
            "FROM ai_apps WHERE id=%s",
            (minimal,),
        )
        ctx.eq(
            json.loads(minimal_row["default_skill_ids_json"]),
            [],
            "DB minimal app default skills",
        )
        ctx.eq(
            json.loads(minimal_row["default_material_ids_json"]),
            [],
            "DB minimal app default materials",
        )

        duplicate = ctx.post(
            "/api/apps",
            user="teacher_a",
            expect=400,
            json={"name": name, "default_prompt": "duplicate"},
        )
        ctx.assert_error(duplicate, 400, "bad_request", "duplicate app name")
        ctx.eq(
            ctx.scalar(
                "SELECT COUNT(*) AS n FROM ai_apps WHERE class_id=%s AND name=%s",
                (ctx.class_id("teacher_a"), name),
            ),
            1,
            "duplicate app did not create a second row",
        )

        cross_template = ctx.post(
            "/api/apps",
            user="teacher_a",
            expect=400,
            json={
                "name": ctx.unique("app-cross-tpl"),
                "prompt_template_id": b_template["id"],
                "default_prompt": "x",
            },
        )
        ctx.assert_error(cross_template, 400, "bad_request", "cross-class app template")

        cross_skill = ctx.post(
            "/api/apps",
            user="teacher_a",
            expect=400,
            json={
                "name": ctx.unique("app-cross-skill"),
                "default_prompt": "x",
                "skill_ids": [b_skill["id"]],
            },
        )
        ctx.assert_error(cross_skill, 400, "bad_request", "cross-class app skill")

        cross_material = ctx.post(
            "/api/apps",
            user="teacher_a",
            expect=400,
            json={
                "name": ctx.unique("app-cross-mat"),
                "default_prompt": "x",
                "material_ids": [b_material["id"]],
            },
        )
        ctx.assert_error(cross_material, 400, "bad_request", "cross-class app material")

        missing_template = ctx.post(
            "/api/apps",
            user="teacher_a",
            expect=400,
            json={
                "name": ctx.unique("app-missing-tpl"),
                "prompt_template_id": 999999999,
                "default_prompt": "x",
            },
        )
        ctx.assert_error(missing_template, 400, "bad_request", "missing app template")

        empty_name = ctx.post(
            "/api/apps",
            user="teacher_a",
            expect=400,
            json={"name": " ", "default_prompt": "x"},
        )
        ctx.assert_error(empty_name, 400, "bad_request", "empty app name")

        student = ctx.post(
            "/api/apps",
            user="student_a1",
            expect=403,
            json={"name": ctx.unique("student-app"), "default_prompt": "x"},
        )
        ctx.assert_error(student, 403, "forbidden", "student app create")

        unauth = ctx.post(
            "/api/apps",
            expect=401,
            json={"name": ctx.unique("anon-app"), "default_prompt": "x"},
        )
        ctx.assert_error(unauth, 401, "unauthorized", "app create without token")
    finally:
        if app is not None:
            ctx.remove_app(app["id"])
        if minimal is not None:
            ctx.remove_app(minimal)
        ctx.remove_skill(skill["id"])
        ctx.remove_skill(b_skill["id"])
        ctx.remove_prompt_template(template["id"])
        ctx.remove_prompt_template(b_template["id"])
