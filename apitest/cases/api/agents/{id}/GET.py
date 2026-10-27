"""GET /api/agents/{id} — visibility of enabled/disabled/cross-class agents."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    agent = ctx.create_agent(ctx.unique("agent-get"), enabled=True)
    disabled = ctx.create_agent(ctx.unique("agent-get-off"), enabled=False)
    try:
        response = ctx.get(f"/api/agents/{agent['id']}", user="teacher_a", expect=200)
        payload = ctx.json(response)
        ctx.eq(payload.get("id"), agent["id"], "agent detail id")
        ctx.eq(payload.get("name"), agent["name"], "agent detail name")
        ctx.eq(payload.get("class_id"), ctx.class_id("teacher_a"), "agent detail class")
        ctx.eq(payload.get("enabled"), True, "agent detail enabled")

        student = ctx.get(f"/api/agents/{agent['id']}", user="student_a1", expect=200)
        ctx.eq(ctx.json(student).get("id"), agent["id"], "student sees enabled agent")

        hidden = ctx.get(f"/api/agents/{disabled['id']}", user="student_a1", expect=404)
        ctx.assert_error(hidden, 404, "not_found", "student disabled agent detail")

        teacher_disabled = ctx.get(
            f"/api/agents/{disabled['id']}", user="teacher_a", expect=200
        )
        ctx.eq(ctx.json(teacher_disabled).get("enabled"), False, "teacher sees disabled")

        cross = ctx.get(f"/api/agents/{agent['id']}", user="teacher_b", expect=404)
        ctx.assert_error(cross, 404, "not_found", "cross-class agent detail")
        cross_student = ctx.get(f"/api/agents/{agent['id']}", user="student_b1", expect=404)
        ctx.assert_error(cross_student, 404, "not_found", "cross-class student detail")

        missing = ctx.get("/api/agents/999999999", user="teacher_a", expect=404)
        ctx.assert_error(missing, 404, "not_found", "missing agent detail")

        unauth = ctx.get(f"/api/agents/{agent['id']}", expect=401)
        ctx.assert_error(unauth, 401, "unauthorized", "agent detail without token")
    finally:
        ctx.remove_agent(agent["id"])
        ctx.remove_agent(disabled["id"])
