"""GET /api/conversations — only the caller's own conversations."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    agent = ctx.create_agent(ctx.unique("conv-list-agent"))
    teacher_conv = ctx.create_conversation(agent["id"], user="teacher_a")
    student_conv = ctx.create_conversation(agent["id"], user="student_a1")
    try:
        teacher = ctx.get("/api/conversations", user="teacher_a", expect=200)
        teacher_items = ctx.json(teacher).get("items")
        ctx.is_instance(teacher_items, list, "teacher conversations items")
        teacher_ids = {item["id"] for item in teacher_items}
        ctx.contains(teacher_ids, teacher_conv["id"], "teacher sees own conversation")
        ctx.not_contains(teacher_ids, student_conv["id"], "teacher does not see student conversation")
        ctx.check(
            all(item["user_id"] == ctx.user_id("teacher_a") for item in teacher_items),
            "teacher list user_id scoped",
            teacher_items,
        )

        student = ctx.get("/api/conversations", user="student_a1", expect=200)
        student_items = ctx.json(student).get("items") or []
        student_ids = {item["id"] for item in student_items}
        ctx.contains(student_ids, student_conv["id"], "student sees own conversation")
        ctx.not_contains(student_ids, teacher_conv["id"], "student does not see teacher conversation")
        ctx.check(
            all(item["user_id"] == ctx.user_id("student_a1") for item in student_items),
            "student list user_id scoped",
            student_items,
        )

        db_count = ctx.scalar(
            "SELECT COUNT(*) AS n FROM conversations WHERE user_id=%s AND class_id=%s",
            (ctx.user_id("teacher_a"), ctx.class_id("teacher_a")),
        )
        ctx.eq(len(teacher_items), db_count, "teacher conversation count vs DB")

        other = ctx.get("/api/conversations", user="student_b1", expect=200)
        other_items = ctx.json(other).get("items") or []
        ctx.check(
            all(item["class_id"] == ctx.class_id("student_b1") for item in other_items),
            "class B conversation list isolated",
            other_items,
        )
        ctx.not_contains({item["id"] for item in other_items}, student_conv["id"], "B list")

        unauth = ctx.get("/api/conversations", expect=401)
        ctx.assert_error(unauth, 401, "unauthorized", "conversations without token")
    finally:
        ctx.remove_conversation(teacher_conv["id"])
        ctx.remove_conversation(student_conv["id"])
        ctx.remove_agent(agent["id"])
