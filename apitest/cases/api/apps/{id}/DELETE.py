"""DELETE /api/apps/{id} — app removed, agents kept with app_id null."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    template = ctx.create_prompt_template(
        ctx.unique("app-del-tpl"), "删除应用模板 {{agent_name}}"
    )
    skill = ctx.create_skill(
        ctx.skill_source(ctx.unique("app-del-skill"), "应用删除", ["knowledge_search"])
    )
    app = ctx.create_app(
        ctx.unique("app-delete"),
        prompt_template_id=template["id"],
        default_prompt="应用删除后的智能体提示词",
        skill_ids=[skill["id"]],
    )
    other_app = ctx.create_app(ctx.unique("app-delete-other"))
    agent = None
    try:
        instantiated = ctx.post(
            f"/api/apps/{app['id']}/instantiate", user="teacher_a", expect=201, json={}
        )
        agent_payload = ctx.json(instantiated)
        agent = agent_payload.get("id")
        ctx.track("agent", agent, {})

        student = ctx.delete(f"/api/apps/{app['id']}", user="student_a1", expect=403)
        ctx.assert_error(student, 403, "forbidden", "student app delete")
        ctx.check(
            ctx.one("SELECT id FROM ai_apps WHERE id=%s", (app["id"],)) is not None,
            "student delete left the app intact",
        )

        cross = ctx.delete(f"/api/apps/{app['id']}", user="teacher_b", expect=404)
        ctx.assert_error(cross, 404, "not_found", "cross-class app delete")
        ctx.check(
            ctx.one("SELECT id FROM ai_apps WHERE id=%s", (app["id"],)) is not None,
            "cross-class delete left the app intact",
        )

        response = ctx.delete(f"/api/apps/{app['id']}", user="teacher_a", expect=204)
        ctx.eq(response.text, "", "delete app response body")
        ctx.untrack("app", app["id"])
        ctx.check(
            ctx.one("SELECT id FROM ai_apps WHERE id=%s", (app["id"],)) is None,
            "app row deleted from DB",
        )
        ctx.check(
            ctx.one("SELECT id FROM skills WHERE id=%s", (skill["id"],)) is not None,
            "app delete did not remove its skill",
        )

        agent_row = ctx.one(
            "SELECT name, prompt, prompt_template_id, app_id FROM agents WHERE id=%s",
            (agent,),
        )
        ctx.check(agent_row is not None, "instantiated agent survives app deletion", agent)
        ctx.check(agent_row["app_id"] is None, "DB agent app_id null after app delete", agent_row)
        ctx.eq(
            agent_row["prompt_template_id"],
            template["id"],
            "agent template link untouched by app delete",
        )
        ctx.eq(agent_row["prompt"], "应用删除后的智能体提示词", "surviving agent prompt")

        detail = ctx.get(f"/api/agents/{agent}", user="teacher_a", expect=200)
        payload = ctx.json(detail)
        ctx.check(
            payload.get("app_id") is None,
            "API agent app_id null after app delete",
            payload,
        )
        ctx.eq(payload.get("name"), agent_payload.get("name"), "surviving agent name")
        ctx.not_contains(payload.get("app_name") or "", app["name"], "agent app_name cleared")

        missing_app = ctx.get(f"/api/apps/{app['id']}", user="teacher_a", expect=404)
        ctx.assert_error(missing_app, 404, "not_found", "deleted app detail")

        again = ctx.delete(f"/api/apps/{app['id']}", user="teacher_a", expect=404)
        ctx.assert_error(again, 404, "not_found", "delete missing app")
    finally:
        if agent is not None:
            ctx.remove_agent(agent)
        ctx.remove_app(app["id"])
        ctx.remove_app(other_app["id"])
        ctx.remove_skill(skill["id"])
        ctx.remove_prompt_template(template["id"])
