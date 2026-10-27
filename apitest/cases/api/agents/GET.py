"""GET /api/agents — teacher sees disabled, students only enabled."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    enabled = ctx.create_agent(ctx.unique("agent-list-on"), enabled=True)
    disabled = ctx.create_agent(ctx.unique("agent-list-off"), enabled=False)
    try:
        response = ctx.get("/api/agents", user="teacher_a", expect=200)
        items = ctx.json(response).get("items")
        ctx.is_instance(items, list, "agents.items")
        ids = {item["id"] for item in items}
        ctx.contains(ids, enabled["id"], "teacher list contains enabled agent")
        ctx.contains(ids, disabled["id"], "teacher list contains disabled agent")
        ctx.check(
            all(item["class_id"] == ctx.class_id("teacher_a") for item in items),
            "teacher agent list stays in class A",
            items,
        )

        db = ctx.one(
            "SELECT class_id, name, prompt, enabled FROM agents WHERE id=%s",
            (enabled["id"],),
        )
        listed = next(item for item in items if item["id"] == enabled["id"])
        ctx.eq(db["class_id"], listed["class_id"], "agent class vs DB")
        ctx.eq(db["name"], listed["name"], "agent name vs DB")
        ctx.eq(db["prompt"], listed["prompt"], "agent prompt vs DB")

        student = ctx.get("/api/agents", user="student_a1", expect=200)
        student_items = ctx.json(student).get("items") or []
        student_ids = {item["id"] for item in student_items}
        ctx.contains(student_ids, enabled["id"], "student sees enabled agent")
        ctx.not_contains(student_ids, disabled["id"], "student does not see disabled agent")
        ctx.check(
            all(item["enabled"] is True for item in student_items),
            "student list only enabled agents",
            student_items,
        )

        other = ctx.get("/api/agents", user="teacher_b", expect=200)
        other_items = ctx.json(other).get("items") or []
        ctx.not_contains(
            {item["id"] for item in other_items}, enabled["id"], "class B agent list"
        )
        ctx.check(
            all(item["class_id"] == ctx.class_id("teacher_b") for item in other_items),
            "class B agent list stays in class B",
            other_items,
        )

        unauth = ctx.get("/api/agents", expect=401)
        ctx.assert_error(unauth, 401, "unauthorized", "agents without token")
    finally:
        ctx.remove_agent(enabled["id"])
        ctx.remove_agent(disabled["id"])
