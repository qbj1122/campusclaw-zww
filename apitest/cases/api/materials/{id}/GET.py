"""GET /api/materials/{id} — detail, class isolation and 404s."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    fixture = ctx.ensure_material("detail")
    material_id = fixture["id"]

    response = ctx.get(f"/api/materials/{material_id}", user="teacher_a", expect=200)
    payload = ctx.json(response)
    ctx.eq(payload.get("id"), material_id, "detail.id")
    ctx.eq(payload.get("title"), fixture["title"], "detail.title")
    ctx.eq(payload.get("class_id"), ctx.class_id("teacher_a"), "detail.class_id")
    ctx.eq(payload.get("uploaded_by"), ctx.user_id("teacher_a"), "detail.uploaded_by")
    ctx.eq(payload.get("body_text"), fixture["body"], "detail.body_text")
    ctx.contains(payload.get("body_text", ""), fixture["marker"], "detail body marker")
    ctx.eq(payload.get("index_status"), "ready", "detail.index_status")

    db_chunks = ctx.scalar(
        "SELECT COUNT(*) AS n FROM knowledge_chunks WHERE material_id=%s", (material_id,)
    )
    ctx.eq(payload.get("chunk_count"), db_chunks, "detail.chunk_count vs DB")

    student = ctx.get(f"/api/materials/{material_id}", user="student_a1", expect=200)
    ctx.eq(ctx.json(student).get("id"), material_id, "same-class student detail")

    cross = ctx.get(f"/api/materials/{material_id}", user="teacher_b", expect=404)
    ctx.assert_error(cross, 404, "not_found", "cross-class detail")

    missing = ctx.get("/api/materials/999999999", user="teacher_a", expect=404)
    ctx.assert_error(missing, 404, "not_found", "missing material detail")

    unauth = ctx.get(f"/api/materials/{material_id}", expect=401)
    ctx.assert_error(unauth, 401, "unauthorized", "detail without token")
