"""GET /api/materials/{id}/file — original download and class isolation."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    fixture = ctx.ensure_material("file")
    material_id = fixture["id"]

    response = ctx.get(
        f"/api/materials/{material_id}/file", user="teacher_a", expect=200
    )
    ctx.eq(response.content.decode("utf-8"), fixture["body"], "download body")
    disposition = response.headers.get("Content-Disposition", "")
    ctx.contains(disposition, "filename*", "download Content-Disposition")

    student = ctx.get(
        f"/api/materials/{material_id}/file", user="student_a1", expect=200
    )
    ctx.eq(student.content.decode("utf-8"), fixture["body"], "student download body")

    cross = ctx.get(
        f"/api/materials/{material_id}/file", user="teacher_b", expect=404
    )
    ctx.assert_error(cross, 404, "not_found", "cross-class download")

    missing = ctx.get("/api/materials/999999999/file", user="teacher_a", expect=404)
    ctx.assert_error(missing, 404, "not_found", "missing material download")

    unauth = ctx.get(f"/api/materials/{material_id}/file", expect=401)
    ctx.assert_error(unauth, 401, "unauthorized", "download without token")
