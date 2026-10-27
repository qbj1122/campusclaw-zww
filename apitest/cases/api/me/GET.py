"""GET /api/me — authenticated identity and unauthenticated 401."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    response = ctx.get("/api/me", user="teacher_a", expect=200)
    payload = ctx.json(response)
    user = payload.get("user") or {}
    ctx.eq(user.get("username"), "teacher_a", "me.user.username")
    ctx.eq(user.get("role"), "teacher", "me.user.role")
    ctx.eq(user.get("class_id"), ctx.class_id("teacher_a"), "me.user.class_id")
    ctx.eq(user.get("class_name"), "班级A", "me.user.class_name")

    row = ctx.one(
        "SELECT id, username, role, class_id FROM users WHERE username=%s",
        ("teacher_a",),
    )
    ctx.eq(user.get("id"), row["id"], "me.user.id vs DB")
    ctx.eq(user.get("role"), row["role"], "me.user.role vs DB")

    student = ctx.get("/api/me", user="student_a1", expect=200)
    student_user = ctx.json(student).get("user") or {}
    ctx.eq(student_user.get("username"), "student_a1", "me student username")
    ctx.eq(student_user.get("role"), "student", "me student role")
    ctx.eq(student_user.get("class_id"), row["class_id"], "student shares teacher class")

    response = ctx.get("/api/me", expect=401)
    ctx.assert_error(response, 401, "unauthorized", "me without token")

    response = ctx.get(
        "/api/me", token="not-a-real-jwt", expect=401
    )
    ctx.assert_error(response, 401, "unauthorized", "me with malformed token")
