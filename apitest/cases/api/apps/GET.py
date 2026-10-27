"""GET /api/apps — teacher sees all apps, students only enabled class apps."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    template = ctx.create_prompt_template(
        ctx.unique("app-list-tpl"), "应用列表模板 {{agent_name}}"
    )
    skill = ctx.create_skill(
        ctx.skill_source(ctx.unique("app-list-skill"), "应用列表", ["knowledge_search"])
    )
    enabled = ctx.create_app(
        ctx.unique("app-list-on"),
        prompt_template_id=template["id"],
        skill_ids=[skill["id"]],
        description="启用的应用",
        category="教学",
        icon="📚",
    )
    disabled = ctx.create_app(
        ctx.unique("app-list-off"),
        description="停用的应用",
        enabled=False,
    )
    try:
        response = ctx.get("/api/apps", user="teacher_a", expect=200)
        payload = ctx.json(response)
        items = payload.get("items")
        ctx.is_instance(items, list, "apps.items")
        ids = {item["id"] for item in items}
        ctx.contains(ids, enabled["id"], "teacher list contains enabled app")
        ctx.contains(ids, disabled["id"], "teacher list contains disabled app")
        ctx.check(
            all(item.get("class_id") == ctx.class_id("teacher_a") for item in items),
            "teacher app list stays in class A",
            items,
        )
        listed = next(item for item in items if item["id"] == enabled["id"])
        ctx.eq(listed.get("name"), enabled["name"], "listed app name")
        ctx.eq(listed.get("description"), "启用的应用", "listed app description")
        ctx.eq(listed.get("category"), "教学", "listed app category")
        ctx.eq(listed.get("icon"), "📚", "listed app icon")
        ctx.eq(listed.get("prompt_template_id"), template["id"], "listed app template id")
        ctx.eq(listed.get("prompt_template_name"), template["name"], "listed app template name")
        ctx.eq(listed.get("default_skill_ids"), [skill["id"]], "listed app skill ids")
        ctx.eq(listed.get("enabled"), True, "listed app enabled")
        ctx.eq(listed.get("agent_count"), 0, "listed app agent_count")

        student = ctx.get("/api/apps", user="student_a1", expect=200)
        student_payload = ctx.json(student)
        student_items = student_payload.get("items") or []
        student_ids = {item["id"] for item in student_items}
        ctx.contains(student_ids, enabled["id"], "student sees enabled app")
        ctx.not_contains(student_ids, disabled["id"], "student does not see disabled app")
        ctx.check(
            all(item.get("enabled") is True for item in student_items),
            "student app list only enabled apps",
            student_items,
        )
        ctx.check(
            all(item.get("class_id") == ctx.class_id("student_a1") for item in student_items),
            "student app list stays in class A",
            student_items,
        )

        other = ctx.get("/api/apps", user="teacher_b", expect=200)
        other_ids = {item["id"] for item in ctx.json(other).get("items", [])}
        ctx.not_contains(other_ids, enabled["id"], "class B app list")
        ctx.not_contains(other_ids, disabled["id"], "class B app list excludes disabled class A app")

        unauth = ctx.get("/api/apps", expect=401)
        ctx.assert_error(unauth, 401, "unauthorized", "apps without token")
    finally:
        ctx.remove_app(enabled["id"])
        ctx.remove_app(disabled["id"])
        ctx.remove_skill(skill["id"])
        ctx.remove_prompt_template(template["id"])
