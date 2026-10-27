"""PUT /api/prompt-templates/{id} — update, revalidation and RBAC."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    template = ctx.create_prompt_template(
        ctx.unique("tpl-upd-a"),
        "原始正文 {{class_name}}",
        description="原始说明",
    )
    second = ctx.create_prompt_template(
        ctx.unique("tpl-upd-b"), "占位正文 {{date}}", description="占位"
    )
    try:
        new_name = ctx.unique("tpl-updated")
        new_content = "更新后：{{agent_name}} 在 {{class_name}}，资料 {{materials}}。"
        response = ctx.put(
            f"/api/prompt-templates/{template['id']}",
            user="teacher_a",
            expect=200,
            json={
                "name": new_name,
                "description": "更新后的说明",
                "content": new_content,
            },
        )
        payload = ctx.json(response)
        ctx.eq(payload.get("id"), template["id"], "updated template id")
        ctx.eq(payload.get("name"), new_name, "updated template name")
        ctx.eq(payload.get("description"), "更新后的说明", "updated template description")
        ctx.eq(payload.get("content"), new_content, "updated template content")
        ctx.eq(
            payload.get("variables"),
            ["agent_name", "class_name", "materials"],
            "updated template variables",
        )

        row = ctx.one(
            "SELECT name, description, content FROM prompt_templates WHERE id=%s",
            (template["id"],),
        )
        ctx.eq(row["name"], new_name, "DB updated template name")
        ctx.eq(row["description"], "更新后的说明", "DB updated template description")
        ctx.eq(row["content"], new_content, "DB updated template content")

        duplicate = ctx.put(
            f"/api/prompt-templates/{template['id']}",
            user="teacher_a",
            expect=400,
            json={
                "name": second["name"],
                "description": "",
                "content": "冲突 {{date}}",
            },
        )
        ctx.assert_error(duplicate, 400, "bad_request", "template update duplicate name")
        still = ctx.one("SELECT name FROM prompt_templates WHERE id=%s", (template["id"],))
        ctx.eq(still["name"], new_name, "duplicate update left name unchanged")

        invalid = ctx.put(
            f"/api/prompt-templates/{template['id']}",
            user="teacher_a",
            expect=400,
            json={"name": new_name, "description": "", "content": "{{question}}"},
        )
        ctx.assert_error(invalid, 400, "bad_request", "template update invalid variable")
        still = ctx.one("SELECT content FROM prompt_templates WHERE id=%s", (template["id"],))
        ctx.eq(still["content"], new_content, "invalid update left content unchanged")

        cross = ctx.put(
            f"/api/prompt-templates/{template['id']}",
            user="teacher_b",
            expect=404,
            json={"name": ctx.unique("cross-tpl"), "description": "", "content": "跨班"},
        )
        ctx.assert_error(cross, 404, "not_found", "cross-class template update")
        still = ctx.one("SELECT name FROM prompt_templates WHERE id=%s", (template["id"],))
        ctx.eq(still["name"], new_name, "cross-class update left template unchanged")

        student = ctx.put(
            f"/api/prompt-templates/{template['id']}",
            user="student_a1",
            expect=403,
            json={"name": ctx.unique("student-tpl"), "description": "", "content": "学生"},
        )
        ctx.assert_error(student, 403, "forbidden", "student template update")

        missing = ctx.put(
            "/api/prompt-templates/999999999",
            user="teacher_a",
            expect=404,
            json={"name": ctx.unique("missing-tpl"), "description": "", "content": "x"},
        )
        ctx.assert_error(missing, 404, "not_found", "missing template update")
    finally:
        ctx.remove_prompt_template(template["id"])
        ctx.remove_prompt_template(second["id"])
