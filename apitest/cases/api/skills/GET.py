"""GET /api/skills — class-scoped skill list."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    name = ctx.unique("skill-list")
    skill = ctx.create_skill(
        ctx.skill_source(name, trigger="列表测试,方程", tools=["knowledge_search"])
    )
    try:
        response = ctx.get("/api/skills", user="teacher_a", expect=200)
        items = ctx.json(response).get("items")
        ctx.is_instance(items, list, "skills.items")
        ids = {item["id"] for item in items}
        ctx.contains(ids, skill["id"], "teacher list contains new skill")
        ctx.check(
            all(item["class_id"] == ctx.class_id("teacher_a") for item in items),
            "skill list stays in class A",
            items,
        )
        listed = next(item for item in items if item["id"] == skill["id"])
        ctx.eq(listed["name"], name, "listed skill name")
        ctx.eq(listed["tools"], ["knowledge_search"], "listed skill tools")

        db = ctx.one(
            "SELECT class_id, name, trigger_text, tools_json, enabled FROM skills WHERE id=%s",
            (skill["id"],),
        )
        ctx.eq(db["class_id"], listed["class_id"], "skill class vs DB")
        ctx.eq(db["name"], listed["name"], "skill name vs DB")
        ctx.eq(db["trigger_text"], listed["trigger_text"], "skill trigger vs DB")

        student = ctx.get("/api/skills", user="student_a1", expect=200)
        ctx.contains(
            {item["id"] for item in ctx.json(student).get("items", [])},
            skill["id"],
            "student sees class skill",
        )

        other = ctx.get("/api/skills", user="teacher_b", expect=200)
        other_items = ctx.json(other).get("items") or []
        ctx.not_contains({item["id"] for item in other_items}, skill["id"], "class B skill list")
        ctx.check(
            all(item["class_id"] == ctx.class_id("teacher_b") for item in other_items),
            "class B skill list stays in class B",
            other_items,
        )

        unauth = ctx.get("/api/skills", expect=401)
        ctx.assert_error(unauth, 401, "unauthorized", "skills without token")
    finally:
        ctx.remove_skill(skill["id"])
