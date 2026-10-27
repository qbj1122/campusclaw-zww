"""POST /api/logout — revokes the presented JWT immediately."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    before = ctx.scalar("SELECT COUNT(*) AS n FROM token_revocations") or 0
    token = ctx.fresh_login("teacher_b")

    response = ctx.post("/api/logout", token=token, expect=204)
    ctx.eq(response.text, "", "logout body")

    revoked = ctx.get("/api/me", token=token, expect=401)
    ctx.assert_error(revoked, 401, "unauthorized", "me after logout")

    after = ctx.scalar("SELECT COUNT(*) AS n FROM token_revocations") or 0
    ctx.eq(after, before + 1, "logout inserted one revocation row")

    # The revoked token stays rejected even if logout is repeated (the auth
    # middleware rejects it before the handler).
    again = ctx.post("/api/logout", token=token, expect=401)
    ctx.assert_error(again, 401, "unauthorized", "logout with revoked token")

    unauth = ctx.post("/api/logout", expect=401)
    ctx.assert_error(unauth, 401, "unauthorized", "logout without token")

    # A fresh login still works; revocation only affects the old jti.
    fresh = ctx.fresh_login("teacher_b")
    ok = ctx.get("/api/me", token=fresh, expect=200)
    ctx.eq(ctx.json(ok).get("user", {}).get("username"), "teacher_b", "fresh login after logout")
