"""POST /api/ask — evidence answer, citations and fixed no-evidence text."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    class_a = ctx.class_id("teacher_a")

    response = ctx.post(
        "/api/ask",
        user="student_a1",
        expect=200,
        json={"q": "这道方程去括号怎么去", "history": []},
    )
    payload = ctx.json(response)
    ctx.eq(payload.get("status"), "ok", "ask status")
    answer = payload.get("answer") or ""
    ctx.check(len(answer) > 10, "ask answer is non-empty", payload)
    citations = payload.get("citations")
    ctx.is_instance(citations, list, "ask citations")
    ctx.check(bool(citations), "ask evidence returns citations", payload)
    ctx.eq(
        [citation["index"] for citation in citations],
        list(range(1, len(citations) + 1)),
        "ask citation indices",
    )
    ctx.contains(answer, "[1]", "ask answer cites first evidence item")
    for citation in citations:
        ctx.eq(
            ctx.scalar(
                "SELECT class_id FROM materials WHERE id=%s",
                (citation["material_id"],),
            ),
            class_a,
            "citation material belongs to class A",
        )
        chunk = ctx.one(
            "SELECT kc.id, kc.class_id, kc.material_id, kc.chunk_index "
            "FROM knowledge_chunks kc WHERE kc.id=%s",
            (citation["chunk_id"],),
        )
        ctx.check(chunk is not None, "citation chunk exists in DB", citation)
        ctx.eq(chunk["class_id"], class_a, "citation chunk class vs DB")
        ctx.eq(chunk["material_id"], citation["material_id"], "citation material vs DB")
        ctx.eq(chunk["chunk_index"], citation["chunk_index"], "citation chunk_index vs DB")
        material = ctx.one(
            "SELECT title FROM materials WHERE id=%s", (citation["material_id"],)
        )
        ctx.eq(material["title"], citation["material_title"], "citation material title vs DB")

    no_evidence = ctx.post(
        "/api/ask",
        user="student_a1",
        expect=200,
        json={"q": ctx.no_evidence_query()},
    )
    no_evidence_payload = ctx.json(no_evidence)
    ctx.eq(no_evidence_payload.get("status"), "no_evidence", "ask no-evidence status")
    ctx.eq(
        no_evidence_payload.get("answer"),
        "资料中未找到相关内容",
        "ask no-evidence fixed answer",
    )
    ctx.eq(no_evidence_payload.get("citations"), [], "ask no-evidence citations")

    short = ctx.post("/api/ask", user="student_a1", expect=400, json={"q": "题"})
    ctx.assert_error(short, 400, "bad_request", "ask too-short question")

    unauth = ctx.post("/api/ask", expect=401, json={"q": "这道方程怎么解"})
    ctx.assert_error(unauth, 401, "unauthorized", "ask without token")
