"""App visibility matrix for students/teachers and cross-class writes."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    template = ctx.create_prompt_template(
        ctx.unique("vis-tpl"), "可见性模板 {{class_name}}"
    )
    b_template = ctx.create_prompt_template(
        ctx.unique("vis-btpl"), "B 班可见性模板 {{date}}", user="teacher_b"
    )
    enabled = ctx.create_app(
        ctx.unique("vis-on"), prompt_template_id=template["id"], description="可见应用"
    )
    disabled = ctx.create_app(
        ctx.unique("vis-off"), description="不可见应用", enabled=False
    )
    try:
        teacher = ctx.get("/api/apps", user="teacher_a", expect=200)
        teacher_ids = {item["id"] for item in ctx.json(teacher).get("items", [])}
        ctx.contains(teacher_ids, enabled["id"], "visibility teacher enabled app")
        ctx.contains(teacher_ids, disabled["id"], "visibility teacher disabled app")

        student = ctx.get("/api/apps", user="student_a1", expect=200)
        student_items = ctx.json(student).get("items") or []
        student_ids = {item["id"] for item in student_items}
        ctx.contains(student_ids, enabled["id"], "visibility student enabled app")
        ctx.not_contains(student_ids, disabled["id"], "visibility student disabled app")
        ctx.check(
            all(item.get("enabled") is True for item in student_items),
            "visibility student list enabled only",
            student_items,
        )

        ctx.expect_status(
            ctx.get(f"/api/apps/{enabled['id']}", user="student_a1"), 200, "student enabled detail"
        )
        ctx.assert_error(
            ctx.get(f"/api/apps/{disabled['id']}", user="student_a1"),
            404,
            "not_found",
            "student disabled detail",
        )
        ctx.expect_status(
            ctx.get(f"/api/apps/{disabled['id']}", user="teacher_a"),
            200,
            "teacher disabled detail",
        )
        ctx.assert_error(
            ctx.get(f"/api/apps/{enabled['id']}", user="teacher_b"),
            404,
            "not_found",
            "teacher B class A app detail",
        )
        ctx.assert_error(
            ctx.get(f"/api/apps/{enabled['id']}", user="student_b1"),
            404,
            "not_found",
            "student B class A app detail",
        )

        ctx.assert_error(
            ctx.post(
                f"/api/apps/{disabled['id']}/instantiate",
                user="teacher_a",
                json={},
            ),
            400,
            "bad_request",
            "visibility disabled instantiate",
        )

        student_create = ctx.post(
            "/api/apps",
            user="student_a1",
            json={"name": ctx.unique("vis-student"), "default_prompt": "x"},
        )
        ctx.assert_error(student_create, 403, "forbidden", "visibility student create")
        student_update = ctx.put(
            f"/api/apps/{enabled['id']}",
            user="student_a1",
            json={"name": ctx.unique("vis-student-upd"), "default_prompt": "x"},
        )
        ctx.assert_error(student_update, 403, "forbidden", "visibility student update")
        student_delete = ctx.delete(
            f"/api/apps/{enabled['id']}", user="student_a1"
        )
        ctx.assert_error(student_delete, 403, "forbidden", "visibility student delete")
        ctx.check(
            ctx.one("SELECT id FROM ai_apps WHERE id=%s", (enabled["id"],)) is not None,
            "visibility student delete left app intact",
        )

        cross_template = ctx.post(
            "/api/apps",
            user="teacher_a",
            expect=400,
            json={
                "name": ctx.unique("vis-cross"),
                "prompt_template_id": b_template["id"],
                "default_prompt": "x",
            },
        )
        ctx.assert_error(cross_template, 400, "bad_request", "visibility cross-class template")
    finally:
        ctx.remove_app(enabled["id"])
        ctx.remove_app(disabled["id"])
        ctx.remove_prompt_template(template["id"])
        ctx.remove_prompt_template(b_template["id"])
