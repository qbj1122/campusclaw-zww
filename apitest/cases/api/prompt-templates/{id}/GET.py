"""GET /api/prompt-templates/{id} — detail, variables and class isolation."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    name = ctx.unique("tpl-detail")
    content = "学生 {{student_name}}，技能 {{skill_name}}，学科 {{subject}}。"
    template = ctx.create_prompt_template(name, content, description="详情模板")
    try:
        response = ctx.get(
            f"/api/prompt-templates/{template['id']}", user="teacher_a", expect=200
        )
        payload = ctx.json(response)
        ctx.eq(payload.get("id"), template["id"], "template detail id")
        ctx.eq(payload.get("name"), name, "template detail name")
        ctx.eq(payload.get("description"), "详情模板", "template detail description")
        ctx.eq(payload.get("content"), content, "template detail content")
        ctx.eq(
            payload.get("variables"),
            ["student_name", "skill_name", "subject"],
            "template detail variables",
        )
        ctx.eq(payload.get("class_id"), ctx.class_id("teacher_a"), "template detail class")

        row = ctx.one("SELECT content FROM prompt_templates WHERE id=%s", (template["id"],))
        ctx.eq(row["content"], payload["content"], "DB template content vs detail")

        cross = ctx.get(
            f"/api/prompt-templates/{template['id']}", user="teacher_b", expect=404
        )
        ctx.assert_error(cross, 404, "not_found", "cross-class template detail")

        student = ctx.get(
            f"/api/prompt-templates/{template['id']}", user="student_a1", expect=403
        )
        ctx.assert_error(student, 403, "forbidden", "student template detail")

        missing = ctx.get("/api/prompt-templates/999999999", user="teacher_a", expect=404)
        ctx.assert_error(missing, 404, "not_found", "missing template detail")

        unauth = ctx.get(f"/api/prompt-templates/{template['id']}", expect=401)
        ctx.assert_error(unauth, 401, "unauthorized", "template detail without token")
    finally:
        ctx.remove_prompt_template(template["id"])
