"""DELETE /api/prompt-templates/{id} — agents/apps survive and unlink."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    template = ctx.create_prompt_template(
        ctx.unique("tpl-del"), "删除测试模板 {{agent_name}}"
    )
    agent = ctx.create_agent(
        ctx.unique("tpl-del-agent"),
        prompt="模板删除后的回退提示词",
        prompt_template_id=template["id"],
    )
    app = ctx.create_app(
        ctx.unique("tpl-del-app"),
        prompt_template_id=template["id"],
        default_prompt="应用默认提示词",
    )
    try:
        student = ctx.delete(
            f"/api/prompt-templates/{template['id']}", user="student_a1", expect=403
        )
        ctx.assert_error(student, 403, "forbidden", "student template delete")
        ctx.check(
            ctx.one("SELECT id FROM prompt_templates WHERE id=%s", (template["id"],)) is not None,
            "student delete left the template intact",
        )

        cross = ctx.delete(
            f"/api/prompt-templates/{template['id']}", user="teacher_b", expect=404
        )
        ctx.assert_error(cross, 404, "not_found", "cross-class template delete")
        ctx.check(
            ctx.one("SELECT id FROM prompt_templates WHERE id=%s", (template["id"],)) is not None,
            "cross-class delete left the template intact",
        )

        response = ctx.delete(
            f"/api/prompt-templates/{template['id']}", user="teacher_a", expect=204
        )
        ctx.eq(response.text, "", "delete template response body")
        ctx.untrack("template", template["id"])
        ctx.check(
            ctx.one("SELECT id FROM prompt_templates WHERE id=%s", (template["id"],)) is None,
            "template row deleted from DB",
        )

        agent_row = ctx.one(
            "SELECT name, prompt, prompt_template_id FROM agents WHERE id=%s",
            (agent["id"],),
        )
        ctx.check(agent_row is not None, "agent survives template deletion", agent)
        ctx.eq(agent_row["name"], agent["name"], "surviving agent name")
        ctx.eq(agent_row["prompt"], "模板删除后的回退提示词", "surviving agent fallback prompt")
        ctx.check(
            agent_row["prompt_template_id"] is None,
            "DB agent template link null after template delete",
            agent_row,
        )
        detail = ctx.get(f"/api/agents/{agent['id']}", user="teacher_a", expect=200)
        detail_payload = ctx.json(detail)
        ctx.check(
            detail_payload.get("prompt_template_id") is None,
            "API agent template link null after template delete",
            detail_payload,
        )
        ctx.eq(detail_payload.get("prompt"), "模板删除后的回退提示词", "API fallback prompt")
        ctx.not_contains(
            detail_payload.get("prompt_template_name") or "",
            "tpl-del",
            "agent template name cleared",
        )

        app_row = ctx.one(
            "SELECT name, default_prompt, prompt_template_id FROM ai_apps WHERE id=%s",
            (app["id"],),
        )
        ctx.check(app_row is not None, "app survives template deletion", app)
        ctx.check(
            app_row["prompt_template_id"] is None,
            "DB app template link null after template delete",
            app_row,
        )
        app_detail = ctx.get(f"/api/apps/{app['id']}", user="teacher_a", expect=200)
        app_payload = ctx.json(app_detail)
        ctx.check(
            app_payload.get("prompt_template_id") is None,
            "API app template link null after template delete",
            app_payload,
        )
        ctx.eq(app_payload.get("id"), app["id"], "app still readable after template delete")

        again = ctx.delete(
            f"/api/prompt-templates/{template['id']}", user="teacher_a", expect=404
        )
        ctx.assert_error(again, 404, "not_found", "delete missing template")
    finally:
        ctx.remove_agent(agent["id"])
        ctx.remove_app(app["id"])
        ctx.remove_prompt_template(template["id"])
