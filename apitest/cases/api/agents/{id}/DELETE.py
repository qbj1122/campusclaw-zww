"""DELETE /api/agents/{id} — deletion, conversation conflict and RBAC."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    agent = ctx.create_agent(ctx.unique("agent-del"))
    busy = ctx.create_agent(ctx.unique("agent-del-busy"))
    conversation = None
    try:
        student = ctx.delete(f"/api/agents/{agent['id']}", user="student_a1", expect=403)
        ctx.assert_error(student, 403, "forbidden", "student delete agent")
        ctx.check(
            ctx.one("SELECT id FROM agents WHERE id=%s", (agent["id"],)) is not None,
            "student delete kept agent",
        )

        cross = ctx.delete(
            f"/api/agents/{agent['id']}", user="teacher_b", expect=404
        )
        ctx.assert_error(cross, 404, "not_found", "cross-class agent delete")
        ctx.check(
            ctx.one("SELECT id FROM agents WHERE id=%s", (agent["id"],)) is not None,
            "cross-class delete must not remove the agent",
        )

        response = ctx.delete(f"/api/agents/{agent['id']}", user="teacher_a", expect=204)
        ctx.eq(response.text, "", "delete agent response body")
        ctx.check(
            ctx.one("SELECT id FROM agents WHERE id=%s", (agent["id"],)) is None,
            "agent row deleted from DB",
        )

        conversation = ctx.create_conversation(busy["id"], user="student_a1")
        conflict = ctx.delete(
            f"/api/agents/{busy['id']}", user="teacher_a", expect=409
        )
        ctx.assert_error(conflict, 409, "conflict", "delete agent used by conversation")
        ctx.check(
            ctx.one("SELECT id FROM agents WHERE id=%s", (busy["id"],)) is not None,
            "conflicted agent remains in DB",
        )

        ctx.remove_conversation(conversation["id"])
        conversation = None
        freed = ctx.delete(f"/api/agents/{busy['id']}", user="teacher_a", expect=204)
        ctx.eq(freed.text, "", "delete freed agent body")
        ctx.check(
            ctx.one("SELECT id FROM agents WHERE id=%s", (busy["id"],)) is None,
            "freed agent deleted from DB",
        )
    finally:
        if conversation is not None:
            ctx.remove_conversation(conversation["id"])
        ctx.remove_agent(agent["id"])
        ctx.remove_agent(busy["id"])
