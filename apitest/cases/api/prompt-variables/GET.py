"""GET /api/prompt-variables — allowed template variable catalog."""

from lib import Ctx


ALLOWED = [
    "class_name",
    "agent_name",
    "student_name",
    "skill_name",
    "subject",
    "date",
    "materials",
]


def run(ctx: Ctx) -> None:
    response = ctx.get("/api/prompt-variables", user="teacher_a", expect=200)
    payload = ctx.json(response)
    items = payload.get("items")
    ctx.is_instance(items, list, "prompt-variables.items")
    names = [item.get("name") for item in items]
    ctx.eq(names, ALLOWED, "prompt variable names/order")
    ctx.not_contains(names, "question", "question is not an allowed variable")
    for item in items:
        ctx.is_instance(item.get("description"), str, f"variable {item.get('name')} description")
        ctx.check(
            bool(item.get("description")),
            f"variable {item.get('name')} has a description",
            item,
        )

    student = ctx.get("/api/prompt-variables", user="student_a1", expect=403)
    ctx.assert_error(student, 403, "forbidden", "student prompt variables")
    unauth = ctx.get("/api/prompt-variables", expect=401)
    ctx.assert_error(unauth, 401, "unauthorized", "prompt variables without token")
