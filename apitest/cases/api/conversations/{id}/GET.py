"""GET /api/conversations/{id} — owner-only detail with message list."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    agent = ctx.create_agent(ctx.unique("conv-detail-agent"))
    conversation = ctx.create_conversation(agent["id"], user="teacher_a")
    try:
        response = ctx.get(
            f"/api/conversations/{conversation['id']}", user="teacher_a", expect=200
        )
        payload = ctx.json(response)
        conv = payload.get("conversation") or {}
        messages = payload.get("messages")
        ctx.eq(conv.get("id"), conversation["id"], "detail conversation id")
        ctx.eq(conv.get("user_id"), ctx.user_id("teacher_a"), "detail conversation user")
        ctx.eq(conv.get("agent_id"), agent["id"], "detail conversation agent")
        ctx.is_instance(messages, list, "detail messages")
        ctx.eq(messages, [], "new conversation has no messages")

        db = ctx.one(
            "SELECT class_id, user_id, agent_id, title FROM conversations WHERE id=%s",
            (conversation["id"],),
        )
        ctx.eq(conv.get("class_id"), db["class_id"], "detail class vs DB")
        ctx.eq(conv.get("title"), db["title"], "detail title vs DB")

        # Same class, different user: still a 404.
        same_class = ctx.get(
            f"/api/conversations/{conversation['id']}", user="student_a1", expect=404
        )
        ctx.assert_error(same_class, 404, "not_found", "same-class other-user detail")

        cross_class = ctx.get(
            f"/api/conversations/{conversation['id']}", user="teacher_b", expect=404
        )
        ctx.assert_error(cross_class, 404, "not_found", "cross-class conversation detail")

        cross_student = ctx.get(
            f"/api/conversations/{conversation['id']}", user="student_b1", expect=404
        )
        ctx.assert_error(cross_student, 404, "not_found", "cross-class student detail")

        missing = ctx.get("/api/conversations/999999999", user="teacher_a", expect=404)
        ctx.assert_error(missing, 404, "not_found", "missing conversation detail")

        unauth = ctx.get(f"/api/conversations/{conversation['id']}", expect=401)
        ctx.assert_error(unauth, 401, "unauthorized", "conversation detail without token")
    finally:
        ctx.remove_conversation(conversation["id"])
        ctx.remove_agent(agent["id"])
