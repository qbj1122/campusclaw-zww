"""POST /api/login — success, wrong password and unknown user."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    response = ctx.post(
        "/api/login",
        json={"username": "teacher_a", "password": ctx.password},
        expect=200,
    )
    payload = ctx.json(response)
    token = payload.get("token")
    ctx.check(isinstance(token, str) and len(token) > 20, "login token", payload)
    ctx.check(isinstance(payload.get("expires_at"), str), "login expires_at", payload)
    user = payload.get("user") or {}
    ctx.eq(user.get("username"), "teacher_a", "login.user.username")
    ctx.eq(user.get("role"), "teacher", "login.user.role")
    ctx.is_instance(user.get("class_id"), int, "login.user.class_id")
    ctx.eq(user.get("class_name"), "班级A", "login.user.class_name")

    row = ctx.one(
        "SELECT id, username, role, class_id FROM users WHERE username=%s",
        ("teacher_a",),
    )
    ctx.check(row is not None, "teacher_a exists in database")
    ctx.eq(user.get("id"), row["id"], "login.user.id vs DB")
    ctx.eq(user.get("role"), row["role"], "login.user.role vs DB")
    ctx.eq(user.get("class_id"), row["class_id"], "login.user.class_id vs DB")

    # Use throw-away usernames for failures so repeated suite runs never trip
    # the per-account login limiter for a real demo account.
    fake_user = ctx.unique("nouser")
    response = ctx.post(
        "/api/login",
        json={"username": fake_user, "password": ctx.password},
        expect=401,
    )
    ctx.assert_error(response, 401, "unauthorized", "unknown user login")
    ctx.not_contains(ctx.safe_text(response), '"token"', "unknown user login body")

    response = ctx.post(
        "/api/login",
        json={"username": fake_user, "password": "definitely-wrong"},
        expect=401,
    )
    ctx.assert_error(response, 401, "unauthorized", "wrong password login")

    response = ctx.post("/api/login", json={}, expect=401)
    ctx.assert_error(response, 401, "unauthorized", "empty login body")
