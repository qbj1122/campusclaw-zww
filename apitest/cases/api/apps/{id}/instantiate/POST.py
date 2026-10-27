"""POST /api/apps/{id}/instantiate — assistant creation, defaults and RBAC."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    material = ctx.ensure_material("app-instantiate")
    template = ctx.create_prompt_template(
        ctx.unique("app-inst-tpl"), "实例化模板 {{agent_name}}"
    )
    skill = ctx.create_skill(
        ctx.skill_source(ctx.unique("app-inst-skill"), "应用实例化", ["knowledge_search"])
    )
    app = ctx.create_app(
        ctx.unique("app-inst"),
        prompt_template_id=template["id"],
        default_prompt="实例化默认提示词",
        skill_ids=[skill["id"]],
        material_ids=[material["id"]],
    )
    disabled = ctx.create_app(ctx.unique("app-inst-off"), enabled=False)
    agents = []
    try:
        response = ctx.post(
            f"/api/apps/{app['id']}/instantiate", user="teacher_a", expect=201, json={}
        )
        payload = ctx.json(response)
        agent_id = payload.get("id")
        agents.append(agent_id)
        ctx.is_instance(agent_id, int, "instantiated agent id")
        ctx.eq(payload.get("name"), f"{app['name']}助手", "default assistant name")
        ctx.eq(payload.get("prompt"), "实例化默认提示词", "instantiated agent prompt")
        ctx.eq(payload.get("prompt_template_id"), template["id"], "instantiated template id")
        ctx.eq(payload.get("app_id"), app["id"], "instantiated app id")
        ctx.eq(payload.get("skill_ids"), [skill["id"]], "instantiated skill ids")
        ctx.eq(payload.get("material_ids"), [material["id"]], "instantiated material ids")
        ctx.eq(payload.get("enabled"), True, "instantiated agent enabled")
        ctx.eq(payload.get("class_id"), ctx.class_id("teacher_a"), "instantiated agent class")

        row = ctx.one(
            "SELECT class_id, name, prompt, prompt_template_id, app_id, enabled "
            "FROM agents WHERE id=%s",
            (agent_id,),
        )
        ctx.check(row is not None, "instantiated agent row exists", agent_id)
        ctx.eq(row["class_id"], ctx.class_id("teacher_a"), "DB instantiated class")
        ctx.eq(row["name"], f"{app['name']}助手", "DB instantiated name")
        ctx.eq(row["prompt"], "实例化默认提示词", "DB instantiated prompt")
        ctx.eq(row["prompt_template_id"], template["id"], "DB instantiated template id")
        ctx.eq(row["app_id"], app["id"], "DB instantiated app id")
        ctx.eq(bool(row["enabled"]), True, "DB instantiated enabled")
        ctx.eq(
            [
                item["skill_id"]
                for item in ctx.query(
                    "SELECT skill_id FROM agent_skills WHERE agent_id=%s", (agent_id,)
                )
            ],
            [skill["id"]],
            "DB instantiated skill binding",
        )
        ctx.eq(
            [
                item["material_id"]
                for item in ctx.query(
                    "SELECT material_id FROM agent_materials WHERE agent_id=%s", (agent_id,)
                )
            ],
            [material["id"]],
            "DB instantiated material binding",
        )

        detail = ctx.get(f"/api/agents/{agent_id}", user="teacher_a", expect=200)
        detail_payload = ctx.json(detail)
        ctx.eq(
            detail_payload.get("prompt_template_name"),
            template["name"],
            "instantiated agent template name",
        )
        ctx.eq(detail_payload.get("app_name"), app["name"], "instantiated agent app name")
        ctx.eq(detail_payload.get("app_id"), app["id"], "instantiated agent app id from GET")

        app_detail = ctx.get(f"/api/apps/{app['id']}", user="teacher_a", expect=200)
        ctx.eq(ctx.json(app_detail).get("agent_count"), 1, "app agent_count after instantiate")

        duplicate = ctx.post(
            f"/api/apps/{app['id']}/instantiate", user="teacher_a", expect=400, json={}
        )
        ctx.assert_error(duplicate, 400, "bad_request", "duplicate assistant name")
        ctx.eq(
            ctx.scalar(
                "SELECT COUNT(*) AS n FROM agents WHERE class_id=%s AND name=%s",
                (ctx.class_id("teacher_a"), f"{app['name']}助手"),
            ),
            1,
            "duplicate instantiate did not create a second agent",
        )

        custom_name = ctx.unique("app-custom-assistant")
        custom = ctx.post(
            f"/api/apps/{app['id']}/instantiate",
            user="teacher_a",
            expect=201,
            json={"name": custom_name},
        )
        custom_payload = ctx.json(custom)
        agents.append(custom_payload.get("id"))
        ctx.eq(custom_payload.get("name"), custom_name, "custom assistant name")
        ctx.eq(custom_payload.get("app_id"), app["id"], "custom assistant app id")

        duplicate_custom = ctx.post(
            f"/api/apps/{app['id']}/instantiate",
            user="teacher_a",
            expect=400,
            json={"name": custom_name},
        )
        ctx.assert_error(duplicate_custom, 400, "bad_request", "duplicate custom name")

        disabled_response = ctx.post(
            f"/api/apps/{disabled['id']}/instantiate",
            user="teacher_a",
            expect=400,
            json={},
        )
        ctx.assert_error(disabled_response, 400, "bad_request", "disabled app instantiate")

        cross = ctx.post(
            f"/api/apps/{app['id']}/instantiate", user="teacher_b", expect=404, json={}
        )
        ctx.assert_error(cross, 404, "not_found", "cross-class app instantiate")

        student = ctx.post(
            f"/api/apps/{app['id']}/instantiate", user="student_a1", expect=403, json={}
        )
        ctx.assert_error(student, 403, "forbidden", "student app instantiate")

        missing = ctx.post(
            "/api/apps/999999999/instantiate", user="teacher_a", expect=404, json={}
        )
        ctx.assert_error(missing, 404, "not_found", "missing app instantiate")

        stale_skill = ctx.create_skill(
            ctx.skill_source(ctx.unique("app-stale-skill"), "失效技能", ["knowledge_search"])
        )
        stale_app = ctx.create_app(
            ctx.unique("app-stale"), skill_ids=[stale_skill["id"]]
        )
        deleted = ctx.delete(f"/api/skills/{stale_skill['id']}", user="teacher_a", expect=204)
        ctx.eq(deleted.text, "", "stale skill deleted")
        ctx.untrack("skill", stale_skill["id"])
        stale = ctx.post(
            f"/api/apps/{stale_app['id']}/instantiate",
            user="teacher_a",
            expect=400,
            json={},
        )
        ctx.assert_error(stale, 400, "bad_request", "app with missing skill instantiate")
        ctx.remove_app(stale_app["id"])
    finally:
        for agent_id in agents:
            if isinstance(agent_id, int):
                ctx.remove_agent(agent_id)
        ctx.remove_app(app["id"])
        ctx.remove_app(disabled["id"])
        ctx.remove_skill(skill["id"])
        ctx.remove_prompt_template(template["id"])
