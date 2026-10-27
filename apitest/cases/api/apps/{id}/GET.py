"""GET /api/apps/{id} — detail, defaults and class/enabled visibility."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    material = ctx.ensure_material("app-detail")
    template = ctx.create_prompt_template(
        ctx.unique("app-detail-tpl"), "应用详情模板 {{class_name}}"
    )
    skill = ctx.create_skill(
        ctx.skill_source(ctx.unique("app-detail-skill"), "应用详情", ["knowledge_search"])
    )
    app = ctx.create_app(
        ctx.unique("app-detail"),
        prompt_template_id=template["id"],
        skill_ids=[skill["id"]],
        material_ids=[material["id"]],
        description="详情应用",
    )
    disabled = ctx.create_app(ctx.unique("app-detail-off"), enabled=False)
    try:
        response = ctx.get(f"/api/apps/{app['id']}", user="teacher_a", expect=200)
        payload = ctx.json(response)
        ctx.eq(payload.get("id"), app["id"], "app detail id")
        ctx.eq(payload.get("name"), app["name"], "app detail name")
        ctx.eq(payload.get("class_id"), ctx.class_id("teacher_a"), "app detail class")
        ctx.eq(payload.get("description"), "详情应用", "app detail description")
        ctx.eq(payload.get("prompt_template_id"), template["id"], "app detail template id")
        ctx.eq(
            payload.get("prompt_template_name"),
            template["name"],
            "app detail template name",
        )
        ctx.eq(payload.get("default_skill_ids"), [skill["id"]], "app detail skill ids")
        ctx.eq(payload.get("default_material_ids"), [material["id"]], "app detail material ids")
        ctx.eq(payload.get("enabled"), True, "app detail enabled")
        ctx.eq(payload.get("agent_count"), 0, "app detail agent_count")

        row = ctx.one(
            "SELECT class_id, name, prompt_template_id, enabled FROM ai_apps WHERE id=%s",
            (app["id"],),
        )
        ctx.eq(row["class_id"], payload["class_id"], "DB app detail class")
        ctx.eq(row["name"], payload["name"], "DB app detail name")
        ctx.eq(row["prompt_template_id"], payload["prompt_template_id"], "DB app detail template")
        ctx.eq(bool(row["enabled"]), payload["enabled"], "DB app detail enabled")

        student = ctx.get(f"/api/apps/{app['id']}", user="student_a1", expect=200)
        ctx.eq(ctx.json(student).get("id"), app["id"], "same-class student app detail")

        hidden = ctx.get(f"/api/apps/{disabled['id']}", user="student_a1", expect=404)
        ctx.assert_error(hidden, 404, "not_found", "disabled app hidden from student")

        teacher_disabled = ctx.get(
            f"/api/apps/{disabled['id']}", user="teacher_a", expect=200
        )
        ctx.eq(
            ctx.json(teacher_disabled).get("enabled"),
            False,
            "teacher sees disabled app detail",
        )

        cross = ctx.get(f"/api/apps/{app['id']}", user="teacher_b", expect=404)
        ctx.assert_error(cross, 404, "not_found", "cross-class app detail")

        student_b = ctx.get(f"/api/apps/{app['id']}", user="student_b1", expect=404)
        ctx.assert_error(student_b, 404, "not_found", "class B student app detail")

        missing = ctx.get("/api/apps/999999999", user="teacher_a", expect=404)
        ctx.assert_error(missing, 404, "not_found", "missing app detail")

        unauth = ctx.get(f"/api/apps/{app['id']}", expect=401)
        ctx.assert_error(unauth, 401, "unauthorized", "app detail without token")
    finally:
        ctx.remove_app(app["id"])
        ctx.remove_app(disabled["id"])
        ctx.remove_skill(skill["id"])
        ctx.remove_prompt_template(template["id"])
