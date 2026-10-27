"""End-to-end catalog flow: template, skill upload, app, assistant and chat.

Creates a prompt template with variables, previews it, uploads a skill file,
creates an AI app referencing template+skill+material, updates it, instantiates
an assistant, chats through the assistant and verifies the database rows.  The
cleanup exercises product deletes and tolerates FK constraints.
"""

import json

from lib import Ctx


def run(ctx: Ctx) -> None:
    template = None
    skill = None
    app = None
    agent = None
    conversation = None
    try:
        # -- prompt template -------------------------------------------------
        template_name = ctx.unique("cat-tpl")
        template_content = (
            "你是 {{agent_name}}，为 {{student_name}} 在 {{class_name}} 答疑。"
            "本次技能：{{skill_name}}；资料：{{materials}}；日期：{{date}}。"
            "请先给下一步提示，不直接给出最终答案。"
        )
        template = ctx.create_prompt_template(
            template_name,
            template_content,
            description="目录特性端到端模板",
        )
        ctx.eq(
            set(template.get("variables") or []),
            {"agent_name", "student_name", "class_name", "skill_name", "materials", "date"},
            "E2E template variables",
        )
        tpl_row = ctx.one(
            "SELECT class_id, name, content FROM prompt_templates WHERE id=%s",
            (template["id"],),
        )
        ctx.check(tpl_row is not None, "E2E template DB row exists", template["id"])
        ctx.eq(tpl_row["class_id"], ctx.class_id("teacher_a"), "E2E template DB class")
        ctx.eq(tpl_row["name"], template_name, "E2E template DB name")
        ctx.eq(tpl_row["content"], template_content, "E2E template DB content")
        listed_tpl = ctx.get("/api/prompt-templates", user="teacher_a", expect=200)
        tpl_item = next(
            item
            for item in ctx.json(listed_tpl)["items"]
            if item["id"] == template["id"]
        )
        ctx.eq(tpl_item.get("content"), template_content, "E2E template list content")
        ctx.eq(
            set(tpl_item.get("variables") or []),
            {"agent_name", "student_name", "class_name", "skill_name", "materials", "date"},
            "E2E template list variables",
        )

        preview = ctx.post(
            f"/api/prompt-templates/{template['id']}/preview",
            user="teacher_a",
            expect=200,
            json={},
        )
        preview_payload = ctx.json(preview)
        rendered = preview_payload.get("rendered") or ""
        ctx.not_contains(rendered, "{{", "E2E preview rendered")
        ctx.contains(rendered, "示例智能体", "E2E preview agent sample")
        ctx.contains(rendered, ctx.user("teacher_a")["class_name"], "E2E preview class sample")

        # -- skill upload ----------------------------------------------------
        trigger = ctx.unique("cat", max_len=18)
        skill_name = ctx.unique("cat-skill")
        skill_source = (
            f"名称：{skill_name}\n"
            f"适用条件：{trigger},方程,去括号\n"
            "资料：数学\n"
            "工具：knowledge_search\n"
            "启用：是\n\n"
            "先判断学生卡在哪一步，只提示下一步，不直接给出最终答案。"
        )
        skill = ctx.upload_skill(skill_source, filename=f"{skill_name}.md")
        ctx.eq(skill.get("name"), skill_name, "E2E uploaded skill name")
        ctx.eq(skill.get("tools"), ["knowledge_search"], "E2E uploaded skill tools")
        skill_row = ctx.one(
            "SELECT class_id, name, source_text FROM skills WHERE id=%s", (skill["id"],)
        )
        ctx.check(skill_row is not None, "E2E skill DB row exists", skill["id"])
        ctx.eq(skill_row["class_id"], ctx.class_id("teacher_a"), "E2E skill DB class")
        ctx.eq(skill_row["name"], skill_name, "E2E skill DB name")
        ctx.eq(skill_row["source_text"], skill_source, "E2E skill DB source_text")

        material = ctx.ensure_material("catalog")
        ctx.contains(material["body"], material["marker"], "E2E material marker fixture")

        # -- app create + update --------------------------------------------
        app_name = ctx.unique("cat-app")
        app = ctx.create_app(
            app_name,
            description="目录特性端到端应用",
            category="教学",
            icon="🧪",
            prompt_template_id=template["id"],
            default_prompt="目录应用默认提示词",
            skill_ids=[skill["id"]],
            material_ids=[material["id"]],
        )
        ctx.eq(app.get("prompt_template_id"), template["id"], "E2E app template id")
        ctx.eq(app.get("default_skill_ids"), [skill["id"]], "E2E app skill ids")
        ctx.eq(app.get("default_material_ids"), [material["id"]], "E2E app material ids")

        updated_name = ctx.unique("cat-app-upd")
        updated = ctx.put(
            f"/api/apps/{app['id']}",
            user="teacher_a",
            expect=200,
            json={
                "name": updated_name,
                "description": "更新后的目录应用",
                "category": "复习",
                "icon": "🧭",
                "prompt_template_id": template["id"],
                "default_prompt": "更新后的目录应用默认提示词",
                "skill_ids": [skill["id"]],
                "material_ids": [material["id"]],
                "enabled": True,
            },
        )
        updated_payload = ctx.json(updated)
        ctx.eq(updated_payload.get("name"), updated_name, "E2E updated app name")
        ctx.eq(updated_payload.get("prompt_template_id"), template["id"], "E2E updated template")
        ctx.eq(updated_payload.get("default_skill_ids"), [skill["id"]], "E2E updated skills")
        ctx.eq(
            updated_payload.get("default_material_ids"),
            [material["id"]],
            "E2E updated materials",
        )
        app_row = ctx.one(
            "SELECT class_id, name, description, category, icon, prompt_template_id, "
            "default_prompt, default_skill_ids_json, default_material_ids_json, enabled "
            "FROM ai_apps WHERE id=%s",
            (app["id"],),
        )
        ctx.check(app_row is not None, "E2E app DB row exists", app["id"])
        ctx.eq(app_row["class_id"], ctx.class_id("teacher_a"), "E2E app DB class")
        ctx.eq(app_row["name"], updated_name, "E2E app DB name")
        ctx.eq(app_row["description"], "更新后的目录应用", "E2E app DB description")
        ctx.eq(app_row["category"], "复习", "E2E app DB category")
        ctx.eq(app_row["icon"], "🧭", "E2E app DB icon")
        ctx.eq(app_row["prompt_template_id"], template["id"], "E2E app DB template")
        ctx.eq(app_row["default_prompt"], "更新后的目录应用默认提示词", "E2E app DB prompt")
        ctx.eq(
            json.loads(app_row["default_skill_ids_json"]),
            [skill["id"]],
            "E2E app DB skill ids",
        )
        ctx.eq(
            json.loads(app_row["default_material_ids_json"]),
            [material["id"]],
            "E2E app DB material ids",
        )
        ctx.eq(bool(app_row["enabled"]), True, "E2E app DB enabled")

        # -- instantiate assistant ------------------------------------------
        instantiated = ctx.post(
            f"/api/apps/{app['id']}/instantiate", user="teacher_a", expect=201, json={}
        )
        agent = ctx.json(instantiated)
        agent_id = agent.get("id")
        ctx.is_instance(agent_id, int, "E2E assistant id")
        ctx.eq(agent.get("name"), f"{updated_name}助手", "E2E default assistant name")
        ctx.eq(agent.get("prompt_template_id"), template["id"], "E2E assistant template id")
        ctx.eq(agent.get("app_id"), app["id"], "E2E assistant app id")
        ctx.eq(agent.get("skill_ids"), [skill["id"]], "E2E assistant skill ids")
        ctx.eq(agent.get("material_ids"), [material["id"]], "E2E assistant material ids")

        agent_row = ctx.one(
            "SELECT class_id, name, prompt, prompt_template_id, app_id, enabled "
            "FROM agents WHERE id=%s",
            (agent_id,),
        )
        ctx.check(agent_row is not None, "E2E assistant DB row exists", agent_id)
        ctx.eq(agent_row["class_id"], ctx.class_id("teacher_a"), "E2E assistant DB class")
        ctx.eq(agent_row["name"], f"{updated_name}助手", "E2E assistant DB name")
        ctx.eq(agent_row["prompt_template_id"], template["id"], "E2E assistant DB template")
        ctx.eq(agent_row["app_id"], app["id"], "E2E assistant DB app")
        ctx.eq(bool(agent_row["enabled"]), True, "E2E assistant DB enabled")
        ctx.eq(
            [
                row["skill_id"]
                for row in ctx.query(
                    "SELECT skill_id FROM agent_skills WHERE agent_id=%s", (agent_id,)
                )
            ],
            [skill["id"]],
            "E2E assistant DB skill binding",
        )
        ctx.eq(
            [
                row["material_id"]
                for row in ctx.query(
                    "SELECT material_id FROM agent_materials WHERE agent_id=%s", (agent_id,)
                )
            ],
            [material["id"]],
            "E2E assistant DB material binding",
        )
        agent_detail = ctx.get(f"/api/agents/{agent_id}", user="teacher_a", expect=200)
        agent_payload = ctx.json(agent_detail)
        ctx.eq(
            agent_payload.get("prompt_template_name"),
            template_name,
            "E2E assistant GET template name",
        )
        ctx.eq(agent_payload.get("app_name"), updated_name, "E2E assistant GET app name")
        app_detail = ctx.get(f"/api/apps/{app['id']}", user="teacher_a", expect=200)
        ctx.eq(ctx.json(app_detail).get("agent_count"), 1, "E2E app agent_count")

        # -- conversation + streamed answer ---------------------------------
        conversation = ctx.create_conversation(agent_id, user="student_a1")
        question = f"{trigger} 这道方程去括号应该怎么去？请只给下一步提示。"
        _response, events = ctx.stream(
            f"/api/conversations/{conversation['id']}/messages/stream",
            user="student_a1",
            content=question,
        )
        ctx.eq(events[0].event, "meta", "E2E meta is first")
        meta = events[0].data
        ctx.eq(meta.get("status"), "ok", "E2E stream status")
        ctx.eq((meta.get("skill") or {}).get("name"), skill_name, "E2E routed skill")
        ctx.is_instance(meta.get("citations"), list, "E2E citations list")
        for citation in meta.get("citations") or []:
            ctx.is_instance(citation.get("material_id"), int, "E2E citation material_id")
            ctx.is_instance(citation.get("chunk_id"), int, "E2E citation chunk_id")

        deltas = ctx.deltas(events)
        ctx.check(deltas, "E2E stream has deltas", events)
        answer = "".join(deltas)
        ctx.check(bool(answer.strip()), "E2E answer is non-empty", answer)
        ctx.eq(events[-1].event, "done", "E2E done is last")
        message_id = events[-1].data.get("message_id")
        ctx.is_instance(message_id, int, "E2E done message_id")

        assistant_row = ctx.one(
            "SELECT id, content, skill_name FROM messages "
            "WHERE conversation_id=%s AND role='assistant' ORDER BY id DESC LIMIT 1",
            (conversation["id"],),
        )
        ctx.check(assistant_row is not None, "E2E assistant message persisted", conversation)
        ctx.eq(assistant_row["id"], message_id, "E2E persisted message id")
        ctx.eq(assistant_row["content"], answer, "E2E persisted message content")
        ctx.eq(assistant_row["skill_name"], skill_name, "E2E persisted skill name")

        # -- delete the app: the assistant must survive ----------------------
        deleted_app = ctx.delete(f"/api/apps/{app['id']}", user="teacher_a", expect=204)
        ctx.eq(deleted_app.text, "", "E2E delete app body")
        ctx.untrack("app", app["id"])
        ctx.check(
            ctx.one("SELECT id FROM ai_apps WHERE id=%s", (app["id"],)) is None,
            "E2E app row deleted",
        )
        surviving = ctx.get(f"/api/agents/{agent_id}", user="teacher_a", expect=200)
        surviving_payload = ctx.json(surviving)
        ctx.check(
            surviving_payload.get("app_id") is None,
            "E2E assistant app_id null after app delete",
            surviving_payload,
        )
        surviving_row = ctx.one("SELECT app_id FROM agents WHERE id=%s", (agent_id,))
        ctx.check(
            surviving_row["app_id"] is None,
            "E2E assistant DB app_id null after app delete",
            surviving_row,
        )
    finally:
        if conversation is not None:
            try:
                ctx.remove_conversation(conversation["id"])
                conversation = None
            except Exception as exc:  # pragma: no cover - best effort
                ctx.warn(f"E2E conversation cleanup: {exc}")
        if agent is not None and isinstance(agent.get("id"), int):
            try:
                response = ctx.delete(
                    f"/api/agents/{agent['id']}", user="teacher_a", expect=(204, 409)
                )
                if response.status_code == 204:
                    ctx.untrack("agent", agent["id"])
                else:
                    ctx.warn("E2E assistant delete returned 409; removing via DB")
                    ctx.remove_agent(agent["id"])
            except Exception as exc:  # pragma: no cover - best effort
                ctx.warn(f"E2E assistant cleanup: {exc}")
                try:
                    ctx.remove_agent(agent["id"])
                except Exception:
                    pass
        if skill is not None and isinstance(skill.get("id"), int):
            try:
                response = ctx.delete(
                    f"/api/skills/{skill['id']}", user="teacher_a", expect=(204, 404)
                )
                if response.status_code == 204:
                    ctx.untrack("skill", skill["id"])
                else:
                    ctx.warn("E2E skill already absent during cleanup")
            except Exception as exc:  # pragma: no cover - best effort
                ctx.warn(f"E2E skill cleanup: {exc}")
                try:
                    ctx.remove_skill(skill["id"])
                except Exception:
                    pass
        if template is not None and isinstance(template.get("id"), int):
            try:
                response = ctx.delete(
                    f"/api/prompt-templates/{template['id']}",
                    user="teacher_a",
                    expect=(204, 404),
                )
                if response.status_code == 204:
                    ctx.untrack("template", template["id"])
                else:
                    ctx.warn("E2E template already absent during cleanup")
            except Exception as exc:  # pragma: no cover - best effort
                ctx.warn(f"E2E template cleanup: {exc}")
                try:
                    ctx.remove_prompt_template(template["id"])
                except Exception:
                    pass
        if app is not None and isinstance(app.get("id"), int):
            try:
                ctx.remove_app(app["id"])
            except Exception as exc:  # pragma: no cover - best effort
                ctx.warn(f"E2E app cleanup: {exc}")
