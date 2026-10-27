"""POST /api/skills/upload — multipart .md/.txt upload, validation and RBAC."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    name = ctx.unique("skill-upload")
    marker = ctx.unique("skill-body", max_len=24)
    source = (
        f"名称：{name}\n"
        "适用条件：上传测试,方程\n"
        "资料：数学\n"
        "工具：knowledge_search\n"
        "启用：是\n\n"
        f"{marker} 上传技能：先提示下一步，不直接给出答案。"
    )
    created = []
    try:
        response = ctx.post(
            "/api/skills/upload",
            user="teacher_a",
            expect=201,
            files={"file": (f"{name}.md", source.encode("utf-8"), "text/markdown")},
        )
        payload = ctx.json(response)
        skill_id = payload.get("id")
        ctx.is_instance(skill_id, int, "uploaded skill id")
        created.append(skill_id)
        ctx.eq(payload.get("name"), name, "uploaded skill name")
        ctx.eq(payload.get("trigger_text"), "上传测试,方程", "uploaded skill trigger")
        ctx.eq(payload.get("subject_text"), "数学", "uploaded skill subject")
        ctx.eq(payload.get("tools"), ["knowledge_search"], "uploaded skill tools")
        ctx.eq(payload.get("enabled"), True, "uploaded skill enabled")
        ctx.eq(payload.get("class_id"), ctx.class_id("teacher_a"), "uploaded skill class")
        ctx.eq(payload.get("source_text"), source, "uploaded skill source_text echo")
        ctx.contains(payload.get("description", ""), marker, "uploaded skill description")

        row = ctx.one(
            "SELECT class_id, name, trigger_text, source_text, enabled FROM skills WHERE id=%s",
            (skill_id,),
        )
        ctx.check(row is not None, "uploaded skill row exists", skill_id)
        ctx.eq(row["class_id"], ctx.class_id("teacher_a"), "DB uploaded skill class")
        ctx.eq(row["name"], name, "DB uploaded skill name")
        ctx.eq(row["trigger_text"], "上传测试,方程", "DB uploaded skill trigger")
        ctx.eq(row["source_text"], source, "DB uploaded skill source_text")
        ctx.eq(bool(row["enabled"]), True, "DB uploaded skill enabled")

        txt_name = ctx.unique("skill-txt")
        txt_source = (
            f"名称：{txt_name}\n适用条件：文本上传\n启用：是\n\n纯文本技能正文。"
        )
        txt_response = ctx.post(
            "/api/skills/upload",
            user="teacher_a",
            expect=201,
            files={"file": (f"{txt_name}.txt", txt_source.encode("utf-8"), "text/plain")},
        )
        txt_payload = ctx.json(txt_response)
        created.append(txt_payload["id"])
        ctx.eq(txt_payload.get("name"), txt_name, ".txt upload accepted")
        ctx.eq(
            ctx.scalar("SELECT source_text FROM skills WHERE id=%s", (txt_payload["id"],)),
            txt_source,
            ".txt source_text persisted",
        )

        bad_ext = ctx.post(
            "/api/skills/upload",
            user="teacher_a",
            expect=400,
            files={"file": (f"{ctx.unique('skill-bad')}.pdf", source.encode(), "application/pdf")},
        )
        ctx.assert_error(bad_ext, 400, "bad_request", "bad skill extension")

        invalid = ctx.post(
            "/api/skills/upload",
            user="teacher_a",
            expect=400,
            files={"file": ("invalid.md", b"not a valid skill", "text/markdown")},
        )
        ctx.assert_error(invalid, 400, "bad_request", "invalid skill content")

        empty = ctx.post(
            "/api/skills/upload",
            user="teacher_a",
            expect=400,
            files={"file": ("empty.md", b"", "text/markdown")},
        )
        ctx.assert_error(empty, 400, "bad_request", "empty skill file")

        too_large = ctx.post(
            "/api/skills/upload",
            user="teacher_a",
            expect=413,
            files={"file": ("large.md", b"x" * (65 * 1024), "text/markdown")},
        )
        ctx.assert_error(too_large, 413, "too_large", "oversized skill file")

        duplicate = ctx.post(
            "/api/skills/upload",
            user="teacher_a",
            expect=400,
            files={"file": (f"{name}.md", source.encode(), "text/markdown")},
        )
        ctx.assert_error(duplicate, 400, "bad_request", "duplicate uploaded skill name")

        student = ctx.post(
            "/api/skills/upload",
            user="student_a1",
            expect=403,
            files={"file": ("student.md", source.encode(), "text/markdown")},
        )
        ctx.assert_error(student, 403, "forbidden", "student skill upload")

        unauth = ctx.post(
            "/api/skills/upload",
            expect=401,
            files={"file": ("anon.md", source.encode(), "text/markdown")},
        )
        ctx.assert_error(unauth, 401, "unauthorized", "skill upload without token")
    finally:
        for skill_id in created:
            ctx.remove_skill(skill_id)
