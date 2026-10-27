"""POST /api/prompt-templates — create, variables validation, duplicates, RBAC."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    name = ctx.unique("tpl-create")
    content = (
        "你是 {{agent_name}}，班级是 {{class_name}}。"
        "本次资料：{{materials}}；日期：{{date}}。"
    )
    template = None
    try:
        response = ctx.post(
            "/api/prompt-templates",
            user="teacher_a",
            expect=201,
            json={"name": name, "description": "创建模板", "content": content},
        )
        payload = ctx.json(response)
        template = payload.get("id")
        ctx.is_instance(template, int, "created template id")
        ctx.eq(payload.get("name"), name, "created template name")
        ctx.eq(payload.get("description"), "创建模板", "created template description")
        ctx.eq(payload.get("content"), content, "created template content")
        ctx.eq(payload.get("class_id"), ctx.class_id("teacher_a"), "created template class")
        ctx.eq(
            payload.get("variables"),
            ["agent_name", "class_name", "materials", "date"],
            "created template variables",
        )

        row = ctx.one(
            "SELECT class_id, name, description, content FROM prompt_templates WHERE id=%s",
            (template,),
        )
        ctx.check(row is not None, "created template row exists", template)
        ctx.eq(row["class_id"], ctx.class_id("teacher_a"), "DB template class_id")
        ctx.eq(row["name"], name, "DB template name")
        ctx.eq(row["description"], "创建模板", "DB template description")
        ctx.eq(row["content"], content, "DB template content")
        listed = ctx.get("/api/prompt-templates", user="teacher_a", expect=200)
        listed_item = next(
            item for item in ctx.json(listed)["items"] if item["id"] == template
        )
        ctx.eq(listed_item.get("variables"), payload.get("variables"), "DB/API template variables")

        invalid_variable = ctx.post(
            "/api/prompt-templates",
            user="teacher_a",
            expect=400,
            json={
                "name": ctx.unique("tpl-bad-var"),
                "description": "",
                "content": "请回答 {{question}}",
            },
        )
        ctx.assert_error(invalid_variable, 400, "bad_request", "question variable rejected")

        malformed = ctx.post(
            "/api/prompt-templates",
            user="teacher_a",
            expect=400,
            json={
                "name": ctx.unique("tpl-malformed"),
                "description": "",
                "content": "你是 {{ class-name }}",
            },
        )
        ctx.assert_error(malformed, 400, "bad_request", "malformed variable rejected")

        empty_name = ctx.post(
            "/api/prompt-templates",
            user="teacher_a",
            expect=400,
            json={"name": " ", "description": "", "content": "正文"},
        )
        ctx.assert_error(empty_name, 400, "bad_request", "empty template name")

        empty_content = ctx.post(
            "/api/prompt-templates",
            user="teacher_a",
            expect=400,
            json={"name": ctx.unique("tpl-empty"), "description": "", "content": " "},
        )
        ctx.assert_error(empty_content, 400, "bad_request", "empty template content")

        duplicate = ctx.post(
            "/api/prompt-templates",
            user="teacher_a",
            expect=400,
            json={"name": name, "description": "", "content": "另一个正文"},
        )
        ctx.assert_error(duplicate, 400, "bad_request", "duplicate template name")
        count = ctx.scalar(
            "SELECT COUNT(*) AS n FROM prompt_templates WHERE class_id=%s AND name=%s",
            (ctx.class_id("teacher_a"), name),
        )
        ctx.eq(count, 1, "duplicate template did not create a second row")

        student = ctx.post(
            "/api/prompt-templates",
            user="student_a1",
            expect=403,
            json={"name": ctx.unique("student-tpl"), "description": "", "content": "x"},
        )
        ctx.assert_error(student, 403, "forbidden", "student template create")

        unauth = ctx.post(
            "/api/prompt-templates",
            expect=401,
            json={"name": ctx.unique("anon-tpl"), "description": "", "content": "x"},
        )
        ctx.assert_error(unauth, 401, "unauthorized", "template create without token")
    finally:
        if template is not None:
            ctx.remove_prompt_template(template)
