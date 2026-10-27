"""Skill upload lifecycle: upload, replace from file, persist and delete."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    first_name = ctx.unique("life-skill")
    first_source = (
        f"名称：{first_name}\n适用条件：生命周期\n工具：knowledge_search\n启用：是\n\n"
        "生命周期初始正文。"
    )
    skill = ctx.upload_skill(first_source, filename=f"{first_name}.md")
    try:
        ctx.eq(
            ctx.scalar("SELECT source_text FROM skills WHERE id=%s", (skill["id"],)),
            first_source,
            "lifecycle initial source_text",
        )
        replaced_name = ctx.unique("life-replaced")
        replaced_source = (
            f"名称：{replaced_name}\n"
            "适用条件：生命周期,替换\n"
            "资料：数学\n"
            "工具：knowledge_search\n"
            "启用：否\n\n"
            "生命周期替换后的正文。"
        )
        replaced = ctx.put(
            f"/api/skills/{skill['id']}/upload",
            user="teacher_a",
            expect=200,
            files={
                "file": (
                    f"{replaced_name}.txt",
                    replaced_source.encode("utf-8"),
                    "text/plain",
                )
            },
        )
        replaced_payload = ctx.json(replaced)
        ctx.eq(replaced_payload.get("id"), skill["id"], "lifecycle replaced id")
        ctx.eq(replaced_payload.get("name"), replaced_name, "lifecycle replaced name")
        ctx.eq(replaced_payload.get("enabled"), False, "lifecycle replaced enabled")
        row = ctx.one(
            "SELECT name, source_text, enabled FROM skills WHERE id=%s", (skill["id"],)
        )
        ctx.eq(row["name"], replaced_name, "lifecycle DB replaced name")
        ctx.eq(row["source_text"], replaced_source, "lifecycle DB replaced source_text")
        ctx.eq(bool(row["enabled"]), False, "lifecycle DB replaced enabled")

        invalid = ctx.put(
            f"/api/skills/{skill['id']}/upload",
            user="teacher_a",
            expect=400,
            files={"file": ("bad.md", b"bad", "text/markdown")},
        )
        ctx.assert_error(invalid, 400, "bad_request", "lifecycle invalid replacement")
        ctx.eq(
            ctx.scalar("SELECT source_text FROM skills WHERE id=%s", (skill["id"],)),
            replaced_source,
            "lifecycle invalid replacement did not change source",
        )

        denied = ctx.post(
            "/api/skills/upload",
            user="student_a1",
            expect=403,
            files={"file": ("student.md", first_source.encode(), "text/markdown")},
        )
        ctx.assert_error(denied, 403, "forbidden", "lifecycle student upload")

        deleted = ctx.delete(f"/api/skills/{skill['id']}", user="teacher_a", expect=204)
        ctx.eq(deleted.text, "", "lifecycle delete body")
        ctx.untrack("skill", skill["id"])
        ctx.check(
            ctx.one("SELECT id FROM skills WHERE id=%s", (skill["id"],)) is None,
            "lifecycle skill row deleted",
        )
        detail = ctx.get(f"/api/skills/{skill['id']}", user="teacher_a", expect=(404, 405))
        ctx.check(
            detail.status_code in (404, 405),
            "lifecycle deleted skill not retrievable",
            detail.status_code,
        )
    finally:
        ctx.remove_skill(skill["id"])
