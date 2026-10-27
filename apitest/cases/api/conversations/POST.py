"""POST /api/conversations — valid/enabled/class-scoped agent selection."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    agent = ctx.create_agent(ctx.unique("conv-create-agent"))
    disabled = ctx.create_agent(ctx.unique("conv-create-off"), enabled=False)
    b_agent = ctx.create_agent(
        ctx.unique("conv-create-b"), user="teacher_b"
    )
    conversation = None
    try:
        response = ctx.post(
            "/api/conversations",
            user="student_a1",
            expect=201,
            json={"agent_id": agent["id"]},
        )
        payload = ctx.json(response)
        conversation = payload.get("id")
        ctx.is_instance(conversation, int, "created conversation id")
        ctx.eq(payload.get("class_id"), ctx.class_id("student_a1"), "conversation class_id")
        ctx.eq(payload.get("user_id"), ctx.user_id("student_a1"), "conversation user_id")
        ctx.eq(payload.get("agent_id"), agent["id"], "conversation agent_id")
        ctx.eq(payload.get("title"), "新的提问", "conversation default title")

        row = ctx.one(
            "SELECT class_id, user_id, agent_id, title FROM conversations WHERE id=%s",
            (conversation,),
        )
        ctx.check(row is not None, "conversation row persisted", conversation)
        ctx.eq(row["class_id"], payload["class_id"], "DB conversation class_id")
        ctx.eq(row["user_id"], payload["user_id"], "DB conversation user_id")
        ctx.eq(row["agent_id"], payload["agent_id"], "DB conversation agent_id")
        ctx.eq(row["title"], "新的提问", "DB conversation title")

        missing = ctx.post(
            "/api/conversations",
            user="student_a1",
            expect=404,
            json={"agent_id": 999999999},
        )
        ctx.assert_error(missing, 404, "not_found", "conversation with missing agent")

        disabled_resp = ctx.post(
            "/api/conversations",
            user="student_a1",
            expect=404,
            json={"agent_id": disabled["id"]},
        )
        ctx.assert_error(disabled_resp, 404, "not_found", "conversation with disabled agent")

        other_class = ctx.post(
            "/api/conversations",
            user="student_a1",
            expect=404,
            json={"agent_id": b_agent["id"]},
        )
        ctx.assert_error(other_class, 404, "not_found", "conversation with other-class agent")

        cross_student = ctx.post(
            "/api/conversations",
            user="student_b1",
            expect=404,
            json={"agent_id": agent["id"]},
        )
        ctx.assert_error(cross_student, 404, "not_found", "class B student with class A agent")

        unauth = ctx.post(
            "/api/conversations", expect=401, json={"agent_id": agent["id"]}
        )
        ctx.assert_error(unauth, 401, "unauthorized", "conversation without token")
    finally:
        if conversation is not None:
            ctx.remove_conversation(conversation)
        ctx.remove_agent(agent["id"])
        ctx.remove_agent(disabled["id"])
        ctx.remove_agent(b_agent["id"])
