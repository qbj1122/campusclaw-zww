"""GET /api/materials — class-scoped list and title filtering."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    fixture = ctx.ensure_material("list")
    class_a = ctx.class_id("teacher_a")
    class_b = ctx.class_id("teacher_b")

    response = ctx.get("/api/materials", user="teacher_a", expect=200)
    payload = ctx.json(response)
    items = payload.get("items")
    ctx.is_instance(items, list, "materials.items")
    ids = {item["id"] for item in items}
    ctx.contains(ids, fixture["id"], "class A list contains fixture")
    ctx.check(
        all(item["class_id"] == class_a for item in items),
        "class A list must not leak other classes",
        items,
    )

    db_rows = ctx.query(
        "SELECT id, title, class_id FROM materials WHERE class_id=%s", (class_a,)
    )
    ctx.eq(ids, {row["id"] for row in db_rows}, "list ids vs DB for class A")
    by_id = {item["id"]: item for item in items}
    for row in db_rows:
        ctx.eq(by_id[row["id"]]["title"], row["title"], f"material {row['id']} title vs DB")

    filtered = ctx.get(
        "/api/materials", user="teacher_a", params={"q": fixture["title"]}, expect=200
    )
    filtered_items = ctx.json(filtered).get("items") or []
    ctx.eq([item["id"] for item in filtered_items], [fixture["id"]], "q=title filter")

    empty = ctx.get(
        "/api/materials", user="teacher_a", params={"q": ctx.unique("no-such-title")}, expect=200
    )
    ctx.eq(ctx.json(empty).get("items"), [], "q=missing returns empty list")

    student = ctx.get("/api/materials", user="student_a1", expect=200)
    student_items = ctx.json(student).get("items") or []
    ctx.contains({item["id"] for item in student_items}, fixture["id"], "student sees class material")

    other = ctx.get("/api/materials", user="student_b1", expect=200)
    other_items = ctx.json(other).get("items") or []
    ctx.check(
        all(item["class_id"] == class_b for item in other_items),
        "class B list must stay in class B",
        other_items,
    )
    ctx.not_contains({item["id"] for item in other_items}, fixture["id"], "class B list")

    unauth = ctx.get("/api/materials", expect=401)
    ctx.assert_error(unauth, 401, "unauthorized", "materials without token")
