"""POST /api/prompt-templates/{id}/preview — sample rendering and RBAC."""

import re

from lib import Ctx


def run(ctx: Ctx) -> None:
    content = (
        "班级：{{class_name}}\n智能体：{{agent_name}}\n学生：{{student_name}}\n"
        "技能：{{skill_name}}\n学科：{{subject}}\n日期：{{date}}\n资料：{{materials}}"
    )
    template = ctx.create_prompt_template(
        ctx.unique("tpl-preview"), content, description="预览模板"
    )
    try:
        response = ctx.post(
            f"/api/prompt-templates/{template['id']}/preview",
            user="teacher_a",
            expect=200,
            json={},
        )
        payload = ctx.json(response)
        ctx.eq(payload.get("content"), content, "preview original content")
        rendered = payload.get("rendered")
        ctx.is_instance(rendered, str, "preview rendered")
        ctx.not_contains(rendered, "{{", "preview rendered has no placeholders")
        ctx.contains(rendered, ctx.user("teacher_a")["class_name"], "preview class name")
        ctx.contains(rendered, "示例智能体", "preview example agent name")
        ctx.contains(rendered, ctx.user("teacher_a")["display_name"], "preview student name")
        ctx.contains(rendered, "数学解题引导", "preview example skill name")
        ctx.contains(rendered, "数学", "preview subject")
        ctx.contains(rendered, "数学自学笔记", "preview materials")
        ctx.check(
            re.search(r"\d{4}-\d{2}-\d{2}", rendered) is not None,
            "preview date is YYYY-MM-DD",
            rendered,
        )

        cross = ctx.post(
            f"/api/prompt-templates/{template['id']}/preview",
            user="teacher_b",
            expect=404,
            json={},
        )
        ctx.assert_error(cross, 404, "not_found", "cross-class template preview")

        student = ctx.post(
            f"/api/prompt-templates/{template['id']}/preview",
            user="student_a1",
            expect=403,
            json={},
        )
        ctx.assert_error(student, 403, "forbidden", "student template preview")

        missing = ctx.post(
            "/api/prompt-templates/999999999/preview",
            user="teacher_a",
            expect=404,
            json={},
        )
        ctx.assert_error(missing, 404, "not_found", "missing template preview")

        unauth = ctx.post(
            f"/api/prompt-templates/{template['id']}/preview", expect=401, json={}
        )
        ctx.assert_error(unauth, 401, "unauthorized", "template preview without token")
    finally:
        ctx.remove_prompt_template(template["id"])
