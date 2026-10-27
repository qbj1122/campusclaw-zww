"""Template deletion keeps agents/apps alive and clears their template links."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    template = ctx.create_prompt_template(
        ctx.unique("fallback-tpl"), "回退模板 {{agent_name}}", description="回退测试"
    )
    agent = ctx.create_agent(
        ctx.unique("fallback-agent"),
        prompt="模板删除后保留的回退提示词",
        prompt_template_id=template["id"],
    )
    app = ctx.create_app(
        ctx.unique("fallback-app"),
        prompt_template_id=template["id"],
        default_prompt="应用回退提示词",
    )
    try:
        deleted = ctx.delete(
            f"/api/prompt-templates/{template['id']}", user="teacher_a", expect=204
        )
        ctx.eq(deleted.text, "", "fallback template delete body")
        ctx.untrack("template", template["id"])
        ctx.check(
            ctx.one("SELECT id FROM prompt_templates WHERE id=%s", (template["id"],)) is None,
            "fallback template row gone",
        )
        ctx.check(
            ctx.one("SELECT id FROM agents WHERE id=%s", (agent["id"],)) is not None,
            "fallback agent row survives",
        )
        ctx.check(
            ctx.one("SELECT id FROM ai_apps WHERE id=%s", (app["id"],)) is not None,
            "fallback app row survives",
        )
        ctx.check(
            ctx.scalar(
                "SELECT prompt_template_id FROM agents WHERE id=%s", (agent["id"],)
            )
            is None,
            "fallback agent link null",
        )
        ctx.check(
            ctx.scalar(
                "SELECT prompt_template_id FROM ai_apps WHERE id=%s", (app["id"],)
            )
            is None,
            "fallback app link null",
        )
        agent_detail = ctx.get(f"/api/agents/{agent['id']}", user="teacher_a", expect=200)
        ctx.eq(
            ctx.json(agent_detail).get("prompt"),
            "模板删除后保留的回退提示词",
            "fallback agent prompt intact",
        )
        app_detail = ctx.get(f"/api/apps/{app['id']}", user="teacher_a", expect=200)
        ctx.eq(ctx.json(app_detail).get("id"), app["id"], "fallback app readable")
        listing = ctx.get("/api/prompt-templates", user="teacher_a", expect=200)
        ctx.not_contains(
            {item["id"] for item in ctx.json(listing).get("items", [])},
            template["id"],
            "fallback template absent from list",
        )
    finally:
        ctx.remove_agent(agent["id"])
        ctx.remove_app(app["id"])
        ctx.remove_prompt_template(template["id"])
