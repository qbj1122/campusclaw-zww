"""GET /health — public liveness probe."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    response = ctx.get("/health", expect=200)
    payload = ctx.json(response)
    ctx.eq(payload.get("status"), "ok", "health.status")
    # Health deliberately does not touch the database; it must still answer
    # while the rest of the app is usable.
    ctx.is_instance(payload, dict, "health body")
