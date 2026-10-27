"""App deletion keeps every instantiated assistant and clears only app_id."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    material = ctx.ensure_material("app-multi")
    template = ctx.create_prompt_template(
        ctx.unique("multi-tpl"), "多助手模板 {{agent_name}}"
    )
    skill = ctx.create_skill(
        ctx.skill_source(ctx.unique("multi-skill"), "多助手", ["knowledge_search"])
    )
    app = ctx.create_app(
        ctx.unique("multi-app"),
        prompt_template_id=template["id"],
        default_prompt="多助手应用提示词",
        skill_ids=[skill["id"]],
        material_ids=[material["id"]],
    )
    assistants = []
    try:
        for custom in (None, ctx.unique("multi-custom")):
            body = {} if custom is None else {"name": custom}
            response = ctx.post(
                f"/api/apps/{app['id']}/instantiate",
                user="teacher_a",
                expect=201,
                json=body,
            )
            payload = ctx.json(response)
            assistants.append(payload["id"])
            ctx.eq(payload.get("app_id"), app["id"], "multi assistant app id")
            if custom is not None:
                ctx.eq(payload.get("name"), custom, "multi custom assistant name")

        ctx.eq(
            ctx.scalar("SELECT COUNT(*) AS n FROM agents WHERE app_id=%s", (app["id"],)),
            len(assistants),
            "multi assistants reference app before delete",
        )
        deleted = ctx.delete(f"/api/apps/{app['id']}", user="teacher_a", expect=204)
        ctx.eq(deleted.text, "", "multi app delete body")
        ctx.untrack("app", app["id"])
        ctx.check(
            ctx.one("SELECT id FROM ai_apps WHERE id=%s", (app["id"],)) is None,
            "multi app row deleted",
        )
        for agent_id in assistants:
            row = ctx.one(
                "SELECT prompt_template_id, app_id FROM agents WHERE id=%s", (agent_id,)
            )
            ctx.check(row is not None, "multi assistant survives app delete", agent_id)
            ctx.check(row["app_id"] is None, "multi assistant app_id null", row)
            ctx.eq(
                row["prompt_template_id"],
                template["id"],
                "multi assistant template link retained",
            )
            ctx.eq(
                ctx.scalar(
                    "SELECT COUNT(*) AS n FROM agent_skills WHERE agent_id=%s AND skill_id=%s",
                    (agent_id, skill["id"]),
                ),
                1,
                "multi assistant skill binding retained",
            )
            ctx.eq(
                ctx.scalar(
                    "SELECT COUNT(*) AS n FROM agent_materials WHERE agent_id=%s AND material_id=%s",
                    (agent_id, material["id"]),
                ),
                1,
                "multi assistant material binding retained",
            )
            detail = ctx.get(f"/api/agents/{agent_id}", user="teacher_a", expect=200)
            ctx.check(
                ctx.json(detail).get("app_id") is None,
                "multi assistant API app_id null",
                ctx.json(detail),
            )
    finally:
        for agent_id in assistants:
            ctx.remove_agent(agent_id)
        ctx.remove_app(app["id"])
        ctx.remove_skill(skill["id"])
        ctx.remove_prompt_template(template["id"])
