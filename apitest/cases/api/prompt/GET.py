"""GET /api/prompt — class default prompt is readable by everyone in class."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    response = ctx.get("/api/prompt", user="teacher_a", expect=200)
    payload = ctx.json(response)
    ctx.is_instance(payload.get("prompt"), str, "prompt value")
    ctx.check(bool(payload["prompt"].strip()), "class A prompt is non-empty", payload)

    row = ctx.one(
        "SELECT prompt FROM class_settings WHERE class_id=%s", (ctx.class_id("teacher_a"),)
    )
    ctx.eq(payload.get("prompt"), row["prompt"], "prompt vs DB")

    student = ctx.get("/api/prompt", user="student_a1", expect=200)
    ctx.eq(ctx.json(student).get("prompt"), row["prompt"], "student sees class prompt")

    other = ctx.get("/api/prompt", user="teacher_b", expect=200)
    other_prompt = ctx.json(other).get("prompt")
    other_row = ctx.one(
        "SELECT prompt FROM class_settings WHERE class_id=%s", (ctx.class_id("teacher_b"),)
    )
    ctx.eq(other_prompt, other_row["prompt"], "class B prompt vs DB")
    ctx.ne(other_prompt, row["prompt"], "classes have independent prompts")

    unauth = ctx.get("/api/prompt", expect=401)
    ctx.assert_error(unauth, 401, "unauthorized", "prompt without token")
