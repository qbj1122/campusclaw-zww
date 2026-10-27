"""POST /api/materials — teacher upload persistence and student 403."""

import os

from lib import Ctx


def run(ctx: Ctx) -> None:
    title = ctx.unique("upload-md")
    marker = ctx.unique("marker", max_len=24)
    body = (
        f"# {title}\n\n"
        f"{marker} 上传测试正文。\n\n"
        "一元一次方程去括号时，括号外的系数要乘到括号内的每一项。\n\n"
        "分母不同的分数先通分，再计算。\n"
    )
    created = None
    try:
        response = ctx.post(
            "/api/materials",
            user="teacher_a",
            expect=201,
            files={"file": (f"{title}.md", body.encode("utf-8"), "text/markdown")},
            data={
                "title": title,
                "strategy": "auto",
                "chunk_max_chars": "200",
                "chunk_overlap": "20",
                "collapse_whitespace": "1",
            },
        )
        payload = ctx.json(response)
        material_id = payload.get("id")
        ctx.is_instance(material_id, int, "upload id")
        ctx.eq(payload.get("title"), title, "upload.title")
        ctx.is_instance(payload.get("chunks"), int, "upload.chunks")
        ctx.check(payload.get("chunks", 0) >= 1, "upload indexed at least one chunk", payload)
        ctx.check("index_error" not in payload, "upload index_error absent", payload)
        created = material_id

        row = ctx.one(
            "SELECT id, class_id, uploaded_by, title, file_path, size_bytes "
            "FROM materials WHERE id=%s",
            (material_id,),
        )
        ctx.check(row is not None, "material row exists", material_id)
        ctx.eq(row["class_id"], ctx.class_id("teacher_a"), "DB material.class_id")
        ctx.eq(row["uploaded_by"], ctx.user_id("teacher_a"), "DB material.uploaded_by")
        ctx.eq(row["title"], title, "DB material.title")
        ctx.eq(row["size_bytes"], len(body.encode("utf-8")), "DB material.size_bytes")

        entry = ctx.one(
            "SELECT material_id, class_id, body_text FROM knowledge_entries WHERE material_id=%s",
            (material_id,),
        )
        ctx.check(entry is not None, "knowledge_entries row exists", material_id)
        ctx.eq(entry["class_id"], row["class_id"], "DB entry.class_id")
        ctx.eq(entry["body_text"], body, "DB entry.body_text")

        chunk_count = ctx.scalar(
            "SELECT COUNT(*) AS n FROM knowledge_chunks WHERE material_id=%s", (material_id,)
        )
        ctx.eq(chunk_count, payload.get("chunks"), "DB chunk count vs API chunks")
        statuses = ctx.query(
            "SELECT index_status FROM knowledge_chunks WHERE material_id=%s", (material_id,)
        )
        ctx.check(
            all(item["index_status"] == "ready" for item in statuses),
            "all uploaded chunks are ready",
            statuses,
        )

        ctx.check(os.path.exists(row["file_path"]), "uploaded file exists", row["file_path"])
        with open(row["file_path"], "rb") as handle:
            ctx.eq(handle.read(), body.encode("utf-8"), "uploaded file content")

        detail = ctx.get(f"/api/materials/{material_id}", user="teacher_a", expect=200)
        detail_payload = ctx.json(detail)
        ctx.eq(detail_payload.get("title"), title, "detail title after upload")
        ctx.eq(detail_payload.get("body_text"), body, "detail body after upload")
        ctx.eq(detail_payload.get("chunk_count"), payload.get("chunks"), "detail chunk_count")

        # Students cannot upload; no material or knowledge entry may be created.
        denied_title = ctx.unique("student-upload")
        denied_body = "学生上传测试正文，不应入库。"
        before = ctx.scalar("SELECT COUNT(*) AS n FROM materials WHERE title=%s", (denied_title,))
        denied = ctx.post(
            "/api/materials",
            user="student_a1",
            expect=403,
            files={"file": (f"{denied_title}.txt", denied_body.encode("utf-8"), "text/plain")},
            data={"title": denied_title},
        )
        ctx.assert_error(denied, 403, "forbidden", "student upload")
        after = ctx.scalar("SELECT COUNT(*) AS n FROM materials WHERE title=%s", (denied_title,))
        ctx.eq(before, 0, "student upload title absent before request")
        ctx.eq(after, 0, "student upload did not create DB row")
        entries = ctx.scalar(
            "SELECT COUNT(*) AS n FROM knowledge_entries WHERE body_text=%s", (denied_body,)
        )
        ctx.eq(entries, 0, "student upload did not create knowledge entry")
    finally:
        if created is not None:
            ctx.remove_material(created)
