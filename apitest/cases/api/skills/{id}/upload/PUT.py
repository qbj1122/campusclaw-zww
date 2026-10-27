"""PUT /api/skills/{id}/upload — replace a skill from a multipart file."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    old_name = ctx.unique("skill-up-file")
    old_source = (
        f"名称：{old_name}\n适用条件：旧触发词\n工具：knowledge_search\n启用：是\n\n旧正文。"
    )
    skill = ctx.upload_skill(old_source, filename=f"{old_name}.md")
    other = ctx.upload_skill(
        ctx.skill_source(ctx.unique("skill-up-other"), "占位", ["knowledge_search"]),
        filename=f"{ctx.unique('other')}.md",
    )
    try:
        new_name = ctx.unique("skill-replaced")
        marker = ctx.unique("replaced-body", max_len=24)
        new_source = (
            f"名称：{new_name}\n"
            "适用条件：替换后,分数\n"
            "资料：数学\n"
            "工具：knowledge_search\n"
            "启用：否\n\n"
            f"{marker} 替换后的技能正文。"
        )
        response = ctx.put(
            f"/api/skills/{skill['id']}/upload",
            user="teacher_a",
            expect=200,
            files={"file": (f"{new_name}.txt", new_source.encode(), "text/plain")},
        )
        payload = ctx.json(response)
        ctx.eq(payload.get("id"), skill["id"], "uploaded update id")
        ctx.eq(payload.get("name"), new_name, "uploaded update name")
        ctx.eq(payload.get("trigger_text"), "替换后,分数", "uploaded update trigger")
        ctx.eq(payload.get("enabled"), False, "uploaded update enabled")
        ctx.eq(payload.get("source_text"), new_source, "uploaded update source echo")

        row = ctx.one(
            "SELECT name, trigger_text, source_text, enabled FROM skills WHERE id=%s",
            (skill["id"],),
        )
        ctx.eq(row["name"], new_name, "DB replaced skill name")
        ctx.eq(row["trigger_text"], "替换后,分数", "DB replaced skill trigger")
        ctx.eq(row["source_text"], new_source, "DB replaced skill source_text")
        ctx.eq(bool(row["enabled"]), False, "DB replaced skill enabled")
        ctx.contains(row["source_text"], marker, "DB replaced skill body marker")

        bad_ext = ctx.put(
            f"/api/skills/{skill['id']}/upload",
            user="teacher_a",
            expect=400,
            files={"file": ("replacement.pdf", new_source.encode(), "application/pdf")},
        )
        ctx.assert_error(bad_ext, 400, "bad_request", "update with bad extension")
        row = ctx.one("SELECT name, source_text FROM skills WHERE id=%s", (skill["id"],))
        ctx.eq(row["name"], new_name, "bad extension left skill name unchanged")
        ctx.eq(row["source_text"], new_source, "bad extension left source unchanged")

        invalid = ctx.put(
            f"/api/skills/{skill['id']}/upload",
            user="teacher_a",
            expect=400,
            files={"file": ("invalid.md", b"missing metadata", "text/markdown")},
        )
        ctx.assert_error(invalid, 400, "bad_request", "update with invalid content")
        ctx.eq(
            ctx.scalar("SELECT source_text FROM skills WHERE id=%s", (skill["id"],)),
            new_source,
            "invalid update left source unchanged",
        )

        duplicate = ctx.put(
            f"/api/skills/{skill['id']}/upload",
            user="teacher_a",
            expect=400,
            files={
                "file": (
                    "duplicate.md",
                    ctx.skill_source(other["name"], "冲突", ["knowledge_search"]).encode(),
                    "text/markdown",
                )
            },
        )
        ctx.assert_error(duplicate, 400, "bad_request", "upload update duplicate name")

        cross = ctx.put(
            f"/api/skills/{skill['id']}/upload",
            user="teacher_b",
            expect=404,
            files={"file": ("cross.md", new_source.encode(), "text/markdown")},
        )
        ctx.assert_error(cross, 404, "not_found", "cross-class skill upload update")

        student = ctx.put(
            f"/api/skills/{skill['id']}/upload",
            user="student_a1",
            expect=403,
            files={"file": ("student.md", new_source.encode(), "text/markdown")},
        )
        ctx.assert_error(student, 403, "forbidden", "student skill upload update")

        missing = ctx.put(
            "/api/skills/999999999/upload",
            user="teacher_a",
            expect=404,
            files={"file": ("missing.md", new_source.encode(), "text/markdown")},
        )
        ctx.assert_error(missing, 404, "not_found", "missing skill upload update")
    finally:
        ctx.remove_skill(skill["id"])
        ctx.remove_skill(other["id"])
