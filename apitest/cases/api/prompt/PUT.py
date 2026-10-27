"""PUT /api/prompt — teacher-only update with persistence and restore."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    class_id = ctx.class_id("teacher_a")
    original_response = ctx.get("/api/prompt", user="teacher_a", expect=200)
    original = ctx.json(original_response)["prompt"]
    updated = f"自动化测试提示词 {ctx.unique('prompt')}：依据本班材料回答，不编造出处。"
    try:
        response = ctx.put(
            "/api/prompt", user="teacher_a", expect=200, json={"prompt": updated}
        )
        payload = ctx.json(response)
        ctx.eq(payload.get("prompt"), updated, "PUT prompt echo")

        fetched = ctx.get("/api/prompt", user="student_a1", expect=200)
        ctx.eq(ctx.json(fetched).get("prompt"), updated, "updated prompt visible to student")

        row = ctx.one("SELECT prompt FROM class_settings WHERE class_id=%s", (class_id,))
        ctx.eq(row["prompt"], updated, "prompt persisted to DB")

        student_put = ctx.put(
            "/api/prompt",
            user="student_a1",
            expect=403,
            json={"prompt": "学生不能修改"},
        )
        ctx.assert_error(student_put, 403, "forbidden", "student prompt update")
        row = ctx.one("SELECT prompt FROM class_settings WHERE class_id=%s", (class_id,))
        ctx.eq(row["prompt"], updated, "student update did not change DB")

        too_long = ctx.put(
            "/api/prompt", user="teacher_a", expect=400, json={"prompt": "长" * 2001}
        )
        ctx.assert_error(too_long, 400, "bad_request", "prompt over 2000 chars")
    finally:
        restore = ctx.put(
            "/api/prompt", user="teacher_a", expect=200, json={"prompt": original}
        )
        ctx.eq(ctx.json(restore).get("prompt"), original, "prompt restored")
