"""DELETE /api/skills/{id} — deletion, RBAC and class isolation invariant."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    name = ctx.unique("skill-del")
    skill = ctx.create_skill(ctx.skill_source(name, "删除测试", ["knowledge_search"]))
    try:
        student = ctx.delete(
            f"/api/skills/{skill['id']}", user="student_a1", expect=403
        )
        ctx.assert_error(student, 403, "forbidden", "student delete skill")
        ctx.check(
            ctx.one("SELECT id FROM skills WHERE id=%s", (skill["id"],)) is not None,
            "student delete left the skill intact",
        )

        cross = ctx.delete(
            f"/api/skills/{skill['id']}", user="teacher_b", expect=404
        )
        ctx.assert_error(cross, 404, "not_found", "cross-class skill delete")
        ctx.check(
            ctx.one("SELECT id FROM skills WHERE id=%s", (skill["id"],)) is not None,
            "cross-class delete must not remove the skill",
        )

        response = ctx.delete(f"/api/skills/{skill['id']}", user="teacher_a", expect=204)
        ctx.eq(response.text, "", "delete skill response body")
        ctx.check(
            ctx.one("SELECT id FROM skills WHERE id=%s", (skill["id"],)) is None,
            "skill row deleted from DB",
        )
        listed = ctx.get("/api/skills", user="teacher_a", expect=200)
        ctx.not_contains(
            {item["id"] for item in ctx.json(listed).get("items", [])},
            skill["id"],
            "deleted skill absent from list",
        )

        # Re-deleting an absent id returns the same 404 as a cross-class id.
        again = ctx.delete(
            f"/api/skills/{skill['id']}", user="teacher_a", expect=404
        )
        ctx.assert_error(again, 404, "not_found", "delete missing skill")
    finally:
        ctx.remove_skill(skill["id"])
