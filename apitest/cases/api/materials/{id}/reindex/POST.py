"""POST /api/materials/{id}/reindex — custom chunking, 404 and 403."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    title = ctx.unique("reindex")
    paragraphs = [
        f"## 第{i}节 {ctx.unique('para', max_len=18)}\n\n"
        + "去括号时括号外的系数要乘到括号内每一项。" * 4
        + "\n\n"
        + "分母不同时先通分，再计算。" * 4
        for i in range(1, 7)
    ]
    body = f"# {title}\n\n" + "\n\n".join(paragraphs) + "\n"
    created = None
    try:
        upload = ctx.post(
            "/api/materials",
            user="teacher_a",
            expect=201,
            files={"file": (f"{title}.md", body.encode("utf-8"), "text/markdown")},
            data={"title": title},
        )
        created = ctx.json(upload)["id"]
        before = ctx.scalar(
            "SELECT COUNT(*) AS n FROM knowledge_chunks WHERE material_id=%s", (created,)
        )

        response = ctx.post(
            f"/api/materials/{created}/reindex",
            user="teacher_a",
            expect=200,
            json={
                "strategy": "custom",
                "chunk_max_chars": 220,
                "chunk_overlap": 20,
                "strip_urls": True,
                "collapse_whitespace": True,
            },
        )
        payload = ctx.json(response)
        ctx.eq(payload.get("strategy"), "custom", "reindex.strategy")
        ctx.is_instance(payload.get("chunks"), int, "reindex.chunks")
        ctx.check(payload.get("chunks", 0) >= 2, "custom chunking produced multiple chunks", payload)

        rows = ctx.query(
            "SELECT chunk_strategy, index_status FROM knowledge_chunks WHERE material_id=%s",
            (created,),
        )
        ctx.eq(len(rows), payload.get("chunks"), "DB chunk count after reindex")
        ctx.check(
            all(row["chunk_strategy"] == "custom" for row in rows),
            "DB chunk_strategy updated to custom",
            rows[:3],
        )
        ctx.check(
            all(row["index_status"] == "ready" for row in rows),
            "reindexed chunks ready",
            rows[:3],
        )

        detail = ctx.get(f"/api/materials/{created}", user="teacher_a", expect=200)
        ctx.eq(ctx.json(detail).get("chunk_count"), payload.get("chunks"), "detail count after reindex")
        ctx.check(before != payload.get("chunks") or before >= 2, "reindex changed/kept chunk count", before)

        cross = ctx.post(
            f"/api/materials/{created}/reindex",
            user="teacher_b",
            expect=404,
            json={"strategy": "auto"},
        )
        ctx.assert_error(cross, 404, "not_found", "cross-class reindex")

        student = ctx.post(
            f"/api/materials/{created}/reindex",
            user="student_a1",
            expect=403,
            json={"strategy": "auto"},
        )
        ctx.assert_error(student, 403, "forbidden", "student reindex")

        missing = ctx.post(
            "/api/materials/999999999/reindex",
            user="teacher_a",
            expect=404,
            json={"strategy": "auto"},
        )
        ctx.assert_error(missing, 404, "not_found", "missing reindex")
    finally:
        if created is not None:
            ctx.remove_material(created)
