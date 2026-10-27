"""POST /api/search — modes, validation, no-evidence and class isolation."""

from lib import Ctx


def run(ctx: Ctx) -> None:
    fixture = ctx.ensure_material("search")
    class_a = ctx.class_id("teacher_a")
    class_a_materials = {
        row["id"]
        for row in ctx.query("SELECT id FROM materials WHERE class_id=%s", (class_a,))
    }

    for mode in ("keyword", "vector", "hybrid"):
        response = ctx.post(
            "/api/search",
            user="teacher_a",
            expect=200,
            json={"q": "去括号", "mode": mode, "limit": 5, "offset": 0},
        )
        payload = ctx.json(response)
        ctx.eq(payload.get("mode"), mode, f"search[{mode}].mode")
        ctx.eq(payload.get("status"), "ok", f"search[{mode}].status")
        hits = payload.get("hits")
        ctx.is_instance(hits, list, f"search[{mode}].hits")
        ctx.check(bool(hits), f"search[{mode}] returns hits", payload)
        for hit in hits:
            ctx.contains(
                class_a_materials, hit["material_id"], f"search[{mode}] hit class isolation"
            )
            for field in (
                "material_title",
                "chunk_id",
                "chunk_index",
                "start_offset",
                "end_offset",
                "excerpt",
                "score",
                "rank",
            ):
                ctx.check(field in hit, f"search[{mode}] hit has {field}", hit)
        ranks = [hit["rank"] for hit in hits]
        ctx.eq(ranks, sorted(ranks), f"search[{mode}] ranks ascending")

    empty_q = ctx.post(
        "/api/search", user="teacher_a", expect=400, json={"q": "   ", "mode": "keyword"}
    )
    ctx.assert_error(empty_q, 400, "bad_request", "search empty q")

    negative_limit = ctx.post(
        "/api/search",
        user="teacher_a",
        expect=400,
        json={"q": "去括号", "mode": "keyword", "limit": -1},
    )
    ctx.assert_error(negative_limit, 400, "bad_request", "search negative limit")

    negative_offset = ctx.post(
        "/api/search",
        user="teacher_a",
        expect=400,
        json={"q": "去括号", "mode": "keyword", "offset": -2},
    )
    ctx.assert_error(negative_offset, 400, "bad_request", "search negative offset")

    no_evidence = ctx.post(
        "/api/search",
        user="teacher_a",
        expect=200,
        json={"q": ctx.no_evidence_query(), "mode": "keyword"},
    )
    no_evidence_payload = ctx.json(no_evidence)
    ctx.eq(no_evidence_payload.get("status"), "no_evidence", "search no-evidence status")
    ctx.eq(no_evidence_payload.get("hits"), [], "search no-evidence hits")

    beyond = ctx.post(
        "/api/search",
        user="teacher_a",
        expect=200,
        json={"q": "去括号", "mode": "keyword", "limit": 5, "offset": 999},
    )
    beyond_payload = ctx.json(beyond)
    ctx.eq(beyond_payload.get("status"), "ok", "search offset beyond range status")
    ctx.eq(beyond_payload.get("hits"), [], "search offset beyond range hits")

    # A term that exists only in class B must be invisible to class A.  Use
    # rare CJK characters so no incidental ngram can match class A material.
    term = ctx.no_evidence_query()
    b_title = ctx.unique("class-b-isolation")
    b_body = f"# {b_title}\n\n{term} 只出现在班级B的材料里。\n"
    b_material = None
    try:
        upload = ctx.post(
            "/api/materials",
            user="teacher_b",
            expect=201,
            files={"file": (f"{b_title}.md", b_body.encode("utf-8"), "text/markdown")},
            data={"title": b_title},
        )
        b_material = ctx.json(upload)["id"]

        isolated = ctx.post(
            "/api/search",
            user="teacher_a",
            expect=200,
            json={"q": term, "mode": "keyword"},
        )
        isolated_payload = ctx.json(isolated)
        ctx.eq(isolated_payload.get("hits"), [], "class A cannot see class B term")
        ctx.check(
            isolated_payload.get("status") in ("ok", "no_evidence"),
            "class A isolation response is a successful empty result",
            isolated_payload,
        )

        visible = ctx.post(
            "/api/search",
            user="teacher_b",
            expect=200,
            json={"q": term, "mode": "keyword"},
        )
        visible_payload = ctx.json(visible)
        ctx.eq(visible_payload.get("status"), "ok", "class B sees its own term")
        ctx.check(
            any(hit["material_id"] == b_material for hit in visible_payload.get("hits", [])),
            "class B hit points at B material",
            visible_payload,
        )
    finally:
        if b_material is not None:
            ctx.remove_material(b_material)

    unauth = ctx.post("/api/search", expect=401, json={"q": "去括号"})
    ctx.assert_error(unauth, 401, "unauthorized", "search without token")
