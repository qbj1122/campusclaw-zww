"""GET /api/prompt-templates — teacher-only class-scoped template list."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    name = ctx.unique("tpl-list")
    content = (
        "你是 {{agent_name}}，负责 {{class_name}} 的答疑。"
        "今天是 {{date}}，请先给下一步提示。"
    )
    template = ctx.create_prompt_template(name, content, description="列表测试模板")
    try:
        response = ctx.get("/api/prompt-templates", user="teacher_a", expect=200)
        payload = ctx.json(response)
        items = payload.get("items")
        ctx.is_instance(items, list, "prompt-templates.items")
        listed = next((item for item in items if item.get("id") == template["id"]), None)
        ctx.check(listed is not None, "teacher list contains new template", items)
        ctx.eq(listed.get("name"), name, "listed template name")
        ctx.eq(listed.get("description"), "列表测试模板", "listed template description")
        ctx.eq(listed.get("content"), content, "listed template content")
        ctx.eq(
            listed.get("variables"),
            ["agent_name", "class_name", "date"],
            "listed template variables",
        )
        ctx.eq(listed.get("class_id"), ctx.class_id("teacher_a"), "listed template class")
        ctx.check(
            all(item.get("class_id") == ctx.class_id("teacher_a") for item in items),
            "template list stays in class A",
            items,
        )

        row = ctx.one(
            "SELECT class_id, name, description, content FROM prompt_templates WHERE id=%s",
            (template["id"],),
        )
        ctx.check(row is not None, "template row persisted", template["id"])
        ctx.eq(row["class_id"], listed["class_id"], "DB template class_id")
        ctx.eq(row["name"], listed["name"], "DB template name")
        ctx.eq(row["description"], listed["description"], "DB template description")
        ctx.eq(row["content"], listed["content"], "DB template content")

        other = ctx.get("/api/prompt-templates", user="teacher_b", expect=200)
        other_items = ctx.json(other).get("items") or []
        ctx.not_contains(
            {item["id"] for item in other_items},
            template["id"],
            "class B template list",
        )

        student = ctx.get("/api/prompt-templates", user="student_a1", expect=403)
        ctx.assert_error(student, 403, "forbidden", "student template list")

        unauth = ctx.get("/api/prompt-templates", expect=401)
        ctx.assert_error(unauth, 401, "unauthorized", "template list without token")
    finally:
        ctx.remove_prompt_template(template["id"])
