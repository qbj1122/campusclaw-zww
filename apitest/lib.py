"""Shared helpers for the CampusClaw API test suite.

The suite is deliberately dependency-light: ``requests`` for HTTP, ``pymysql``
for the database assertions and nothing else.  Credentials are read from the
environment; the defaults below are the local demo values documented in
``apitest/README.md``.
"""

from __future__ import annotations

import json
import os
import re
import secrets
import string
import time
from pathlib import Path
from dataclasses import dataclass
from typing import Any, Iterable

import pymysql
import requests
from pymysql.cursors import DictCursor


# ---------------------------------------------------------------------------
# configuration
# ---------------------------------------------------------------------------


def _load_dotenv() -> None:
    """Load the repo-root .env if present. Real values stay out of the repo."""
    path = Path(__file__).resolve().parent.parent / ".env"
    if not path.is_file():
        return
    for raw in path.read_text(encoding="utf-8").splitlines():
        line = raw.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        key, value = key.strip(), value.strip()
        if value[:1] == value[-1:] and value[:1] in ("'", '"'):
            value = value[1:-1]
        os.environ.setdefault(key, value)


_load_dotenv()


def _env(name: str, default: str) -> str:
    value = os.environ.get(name)
    return value if value not in (None, "") else default


DEFAULT_BASE_URL = "http://127.0.0.1:18080"
DEFAULT_PASSWORD = _env("SEED_PASSWORD", "Campus@2026")
DEFAULT_DB = {
    "host": "127.0.0.1",
    "port": 3307,
    "user": "campusclaw",
    "password": "CampusDB@2026",
    "database": "campusclaw",
}
DEFAULT_MCP_BASE_URL = "http://127.0.0.1:8090"
DEFAULT_MCP_TOKEN = "test-mcp-token-0123456789"
DEFAULT_QDRANT_URL = "http://127.0.0.1:6333"
DEFAULT_QDRANT_COLLECTION = "campusclaw_chunks"

ACCOUNTS = ("teacher_a", "student_a1", "teacher_b", "student_b1")


def load_config() -> dict[str, Any]:
    return {
        "base_url": _env("BASE_URL", DEFAULT_BASE_URL).rstrip("/"),
        "password": _env("APITEST_PASSWORD", DEFAULT_PASSWORD),
        "db": {
            "host": _env("DB_HOST", DEFAULT_DB["host"]),
            "port": int(_env("DB_PORT", str(DEFAULT_DB["port"]))),
            "user": _env("DB_USER", DEFAULT_DB["user"]),
            "password": _env("DB_PASSWORD", DEFAULT_DB["password"]),
            "database": _env("DB_NAME", DEFAULT_DB["database"]),
        },
        "mcp_base_url": _env("MCP_BASE_URL", DEFAULT_MCP_BASE_URL).rstrip("/"),
        "mcp_token": _env("MCP_AUTH_TOKEN", DEFAULT_MCP_TOKEN),
        "qdrant_url": _env("QDRANT_URL", DEFAULT_QDRANT_URL).rstrip("/"),
        "qdrant_collection": _env("QDRANT_COLLECTION", DEFAULT_QDRANT_COLLECTION),
    }


# ---------------------------------------------------------------------------
# failures and SSE
# ---------------------------------------------------------------------------


class CaseFailure(AssertionError):
    """A test assertion failed; the message is printed by run_all."""


@dataclass
class SSEEvent:
    event: str
    data: Any
    raw: str = ""


def parse_sse(lines: Iterable[str]) -> list[SSEEvent]:
    """Parse an SSE byte/line stream into events.

    Handles comments, multi-line ``data:`` fields and a missing trailing blank
    line (the latter is useful when a connection closes right after ``done``).
    """

    events: list[SSEEvent] = []
    event_name = ""
    data_lines: list[str] = []

    def flush() -> None:
        nonlocal event_name, data_lines
        if not event_name and not data_lines:
            return
        raw = "\n".join(data_lines)
        data: Any = raw
        if raw:
            try:
                data = json.loads(raw)
            except (TypeError, ValueError):
                data = raw
        events.append(SSEEvent(event_name or "message", data, raw))
        event_name = ""
        data_lines = []

    for raw_line in lines:
        line = raw_line.decode("utf-8") if isinstance(raw_line, bytes) else raw_line
        line = line.rstrip("\r")
        if line == "":
            flush()
            continue
        if line.startswith(":"):
            continue
        field, _, value = line.partition(":")
        if value.startswith(" "):
            value = value[1:]
        if field == "event":
            event_name = value
        elif field == "data":
            data_lines.append(value)
    flush()
    return events


# ---------------------------------------------------------------------------
# context
# ---------------------------------------------------------------------------


class Ctx:
    """Per-run context shared by all case scripts."""

    def __init__(self, config: dict[str, Any] | None = None) -> None:
        self.config = config or load_config()
        self.base_url: str = self.config["base_url"]
        self.password: str = self.config["password"]
        self.mcp_base_url: str = self.config["mcp_base_url"]
        self.mcp_token: str = self.config["mcp_token"]
        self.qdrant_url: str = self.config["qdrant_url"]
        self.qdrant_collection: str = self.config["qdrant_collection"]
        self.http = requests.Session()
        self.state: dict[str, Any] = {}
        self.checks = 0
        self.warnings: list[str] = []
        self.run_id = f"{int(time.time())}-{secrets.token_hex(3)}"

        self._tokens: dict[str, str] = {}
        self._users: dict[str, dict[str, Any]] = {}
        self._db: pymysql.connections.Connection | None = None
        self._tracked: dict[str, dict[int, Any]] = {
            "conversation": {},
            "agent": {},
            "skill": {},
            "material": {},
            "server": {},
        }

    # -- assertions ---------------------------------------------------------

    def check(self, condition: bool, message: str, detail: Any = None) -> None:
        self.checks += 1
        if not condition:
            suffix = f"\n  detail: {detail!r}" if detail is not None else ""
            raise CaseFailure(message + suffix)

    def eq(self, actual: Any, expected: Any, label: str) -> None:
        self.check(actual == expected, f"{label}: expected {expected!r}, got {actual!r}")

    def ne(self, actual: Any, unexpected: Any, label: str) -> None:
        self.check(actual != unexpected, f"{label}: did not expect {unexpected!r}")

    def contains(self, haystack: Any, needle: Any, label: str) -> None:
        self.check(needle in haystack, f"{label}: expected to find {needle!r}", haystack)

    def not_contains(self, haystack: Any, needle: Any, label: str) -> None:
        self.check(needle not in haystack, f"{label}: did not expect {needle!r}", haystack)

    def is_instance(self, value: Any, expected_type: type, label: str) -> None:
        self.check(
            isinstance(value, expected_type),
            f"{label}: expected {expected_type.__name__}, got {type(value).__name__}",
            value,
        )

    def warn(self, message: str) -> None:
        self.warnings.append(message)

    # -- HTTP ---------------------------------------------------------------

    def request(
        self,
        method: str,
        path: str,
        *,
        user: str | None = None,
        token: str | None = None,
        expect: int | Iterable[int] | None = None,
        timeout: float = 60,
        headers: dict[str, str] | None = None,
        **kwargs: Any,
    ) -> requests.Response:
        request_headers = dict(headers or {})
        if token is None and user is not None:
            token = self.token(user)
        if token:
            request_headers["Authorization"] = f"Bearer {token}"
        url = path if path.startswith("http") else self.base_url + path
        try:
            response = self.http.request(
                method, url, headers=request_headers, timeout=timeout, **kwargs
            )
        except requests.RequestException as exc:
            raise CaseFailure(f"{method} {path}: request failed: {exc}") from exc
        if expect is not None:
            self.expect_status(response, expect, f"{method} {path}")
        return response

    def get(self, path: str, **kwargs: Any) -> requests.Response:
        return self.request("GET", path, **kwargs)

    def post(self, path: str, **kwargs: Any) -> requests.Response:
        return self.request("POST", path, **kwargs)

    def put(self, path: str, **kwargs: Any) -> requests.Response:
        return self.request("PUT", path, **kwargs)

    def delete(self, path: str, **kwargs: Any) -> requests.Response:
        return self.request("DELETE", path, **kwargs)

    def expect_status(
        self, response: requests.Response, expected: int | Iterable[int], label: str
    ) -> requests.Response:
        allowed = {expected} if isinstance(expected, int) else set(expected)
        self.checks += 1
        if response.status_code not in allowed:
            body = self.safe_text(response)
            raise CaseFailure(
                f"{label}: expected HTTP {sorted(allowed)}, got {response.status_code}; "
                f"body={body}"
            )
        return response

    def safe_text(self, response: requests.Response, limit: int = 4000) -> str:
        try:
            text = response.text
        except Exception:  # pragma: no cover - defensive
            return "<unreadable body>"
        return text if len(text) <= limit else text[:limit] + "…"

    def json(self, response: requests.Response) -> Any:
        try:
            return response.json()
        except ValueError as exc:
            raise CaseFailure(
                f"response is not valid JSON: status={response.status_code} "
                f"body={self.safe_text(response)}"
            ) from exc

    def assert_error(
        self,
        response: requests.Response,
        status: int,
        code: str | None = None,
        label: str = "error response",
    ) -> dict[str, Any]:
        self.expect_status(response, status, label)
        payload = self.json(response)
        self.is_instance(payload, dict, f"{label} body")
        error = payload.get("error")
        self.is_instance(error, dict, f"{label}.error")
        self.check(bool(error.get("message")), f"{label}: missing error message", payload)
        if code is not None:
            self.eq(error.get("code"), code, f"{label}.error.code")
        return error

    # -- auth ---------------------------------------------------------------

    def login(
        self,
        username: str,
        password: str | None = None,
        *,
        use_cache: bool = True,
    ) -> str:
        if use_cache and username in self._tokens:
            return self._tokens[username]
        response = self.post(
            "/api/login",
            json={"username": username, "password": password or self.password},
            expect=200,
        )
        payload = self.json(response)
        token = payload.get("token")
        self.check(isinstance(token, str) and token, f"login {username}: missing token", payload)
        self._tokens[username] = token
        self._users[username] = payload.get("user") or {}
        return token

    def fresh_login(self, username: str, password: str | None = None) -> str:
        response = self.post(
            "/api/login",
            json={"username": username, "password": password or self.password},
            expect=200,
        )
        payload = self.json(response)
        token = payload.get("token")
        self.check(isinstance(token, str) and token, f"fresh login {username}: missing token", payload)
        return token

    def token(self, username: str) -> str:
        return self.login(username)

    def user(self, username: str) -> dict[str, Any]:
        if username not in self._users:
            self.login(username)
        return self._users[username]

    def user_id(self, username: str) -> int:
        return int(self.user(username)["id"])

    def class_id(self, username: str) -> int:
        return int(self.user(username)["class_id"])

    # -- SSE ----------------------------------------------------------------

    def stream(
        self,
        path: str,
        *,
        user: str,
        content: str | None = None,
        payload: dict[str, Any] | None = None,
        expect: int = 200,
        timeout: float = 180,
    ) -> tuple[requests.Response, list[SSEEvent]]:
        body = payload if payload is not None else {"content": content}
        headers = {"Authorization": f"Bearer {self.token(user)}"}
        url = self.base_url + path
        try:
            response = self.http.post(
                url, headers=headers, json=body, stream=True, timeout=timeout
            )
        except requests.RequestException as exc:
            raise CaseFailure(f"POST {path}: stream request failed: {exc}") from exc

        if response.status_code != expect:
            text = self.safe_text(response)
            response.close()
            self.checks += 1
            raise CaseFailure(
                f"POST {path}: expected HTTP {expect}, got {response.status_code}; "
                f"body={text}"
            )

        if expect != 200:
            response.close()
            return response, []

        content_type = response.headers.get("Content-Type", "")
        self.checks += 1
        if "text/event-stream" not in content_type:
            text = self.safe_text(response)
            response.close()
            raise CaseFailure(
                f"POST {path}: expected text/event-stream, got {content_type!r}; "
                f"body={text}"
            )
        try:
            events = parse_sse(response.iter_lines(decode_unicode=True))
        finally:
            response.close()
        self.check(events, f"POST {path}: SSE stream contained no events")
        return response, events

    @staticmethod
    def event_names(events: list[SSEEvent]) -> list[str]:
        return [event.event for event in events]

    @staticmethod
    def deltas(events: list[SSEEvent]) -> list[str]:
        parts = []
        for event in events:
            if event.event == "delta":
                self_data = event.data
                if isinstance(self_data, dict):
                    parts.append(str(self_data.get("text", "")))
        return parts

    # -- fixtures / unique names -------------------------------------------

    def unique(self, prefix: str, max_len: int = 32) -> str:
        cleaned = re.sub(r"[^a-z0-9_-]+", "-", prefix.lower()).strip("-_") or "t"
        suffix = secrets.token_hex(4)
        room = max(1, max_len - len(suffix) - 1)
        return f"{cleaned[:room]}-{suffix}"[:max_len]

    def unique_cn(self, prefix: str) -> str:
        return f"{prefix}{secrets.randbelow(900000) + 100000}"

    def no_evidence_query(self) -> str:
        """A query built from rare CJK characters absent from the demo data.

        Random ASCII is not safe for the ngram FULLTEXT index: a query such as
        ``not-in-any-material`` can match incidental bigrams in a test title.
        """

        alphabet = "龘靐齉爩麤龗灪籲纞虋魑魅魍魉饕餮"
        return "".join(secrets.choice(alphabet) for _ in range(8))

    def _fixtures(self) -> dict[str, Any]:
        return self.state.setdefault("fixtures", {})

    def ensure_material(self, key: str = "default", body: str | None = None) -> dict[str, Any]:
        fixtures = self._fixtures()
        cache_key = f"material:{key}"
        if cache_key in fixtures:
            return fixtures[cache_key]
        title = self.unique(f"mat-{key}")
        marker = self.unique("mk", max_len=20)
        text = body or (
            f"# {title}\n\n"
            f"## 自动化测试材料\n\n"
            f"{marker} 这是一份用于接口测试的数学材料。\n\n"
            "一元一次方程去括号时，括号外的系数要乘到括号内的每一项，"
            "括号前面是负号时，去掉括号后每一项都要改变符号。\n\n"
            "行程问题中，路程等于速度乘以时间。画线段图可以避免把相遇与追及混淆。\n\n"
            "分数的分母不同时先通分再计算，通分的关键是找分母的最小公倍数。\n"
        )
        response = self.post(
            "/api/materials",
            user="teacher_a",
            expect=201,
            files={"file": (f"{title}.md", text.encode("utf-8"), "text/markdown")},
            data={"title": title},
        )
        payload = self.json(response)
        material_id = payload.get("id")
        self.check(isinstance(material_id, int) and material_id > 0, "fixture material id", payload)
        material = {"id": material_id, "title": title, "body": text, "marker": marker}
        fixtures[cache_key] = material
        self.track("material", material_id, material)
        return material

    @staticmethod
    def skill_source(
        name: str,
        trigger: str = "方程,去括号",
        tools: list[str] | None = None,
        subject: str = "",
        description: str = "这是自动化测试技能的引导正文。先提示下一步，不直接给出最终答案。",
        enabled: bool = True,
    ) -> str:
        lines = [f"名称：{name}", f"适用条件：{trigger}"]
        if subject:
            lines.append(f"资料：{subject}")
        lines.append("工具：" + (",".join(tools) if tools else ""))
        lines.append("启用：" + ("是" if enabled else "否"))
        return "\n".join(lines) + "\n\n" + description

    def create_skill(
        self,
        source_text: str,
        *,
        user: str = "teacher_a",
        expect: int = 201,
        track: bool = True,
    ) -> dict[str, Any]:
        response = self.post(
            "/api/skills", user=user, expect=expect, json={"source_text": source_text}
        )
        if expect != 201:
            return {}
        payload = self.json(response)
        if track and isinstance(payload.get("id"), int):
            self.track("skill", payload["id"], payload)
        return payload

    def create_agent(
        self,
        name: str,
        *,
        prompt: str = "你是自动化测试智能体，依据本班材料回答。",
        enabled: bool = True,
        skill_ids: list[int] | None = None,
        material_ids: list[int] | None = None,
        user: str = "teacher_a",
        expect: int = 201,
        track: bool = True,
    ) -> dict[str, Any]:
        response = self.post(
            "/api/agents",
            user=user,
            expect=expect,
            json={
                "name": name,
                "prompt": prompt,
                "enabled": enabled,
                "skill_ids": skill_ids or [],
                "material_ids": material_ids or [],
            },
        )
        if expect != 201:
            return {}
        payload = self.json(response)
        if track and isinstance(payload.get("id"), int):
            self.track("agent", payload["id"], payload)
        return payload

    def create_conversation(
        self,
        agent_id: int,
        *,
        user: str,
        expect: int = 201,
        track: bool = True,
    ) -> dict[str, Any]:
        response = self.post(
            "/api/conversations", user=user, expect=expect, json={"agent_id": agent_id}
        )
        if expect != 201:
            return {}
        payload = self.json(response)
        if track and isinstance(payload.get("id"), int):
            self.track("conversation", payload["id"], payload)
        return payload

    def create_server(
        self,
        name: str | None = None,
        *,
        token: str | None = None,
        base_url: str | None = None,
        user: str = "teacher_a",
        expect: int = 201,
        track: bool = True,
    ) -> dict[str, Any]:
        name = name or self.unique("mcp-tools")
        response = self.post(
            "/api/mcp/servers",
            user=user,
            expect=expect,
            json={
                "name": name,
                "base_url": base_url or self.mcp_base_url,
                "auth_token": self.mcp_token if token is None else token,
            },
        )
        if expect != 201:
            return {}
        payload = self.json(response)
        if track and isinstance(payload.get("id"), int):
            self.track("server", payload["id"], payload)
        return payload

    def connected_server(self) -> dict[str, Any]:
        fixtures = self._fixtures()
        if "connected_server" in fixtures:
            return fixtures["connected_server"]
        server = self.create_server()
        response = self.post(
            f"/api/mcp/servers/{server['id']}/connect", user="teacher_a", expect=200
        )
        payload = self.json(response)
        server = payload.get("server", server)
        fixtures["connected_server"] = server
        return server

    # -- database -----------------------------------------------------------

    def db(self) -> pymysql.connections.Connection:
        if self._db is None:
            cfg = self.config["db"]
            self._db = pymysql.connect(
                host=cfg["host"],
                port=cfg["port"],
                user=cfg["user"],
                password=cfg["password"],
                database=cfg["database"],
                charset="utf8mb4",
                cursorclass=DictCursor,
                autocommit=True,
            )
        return self._db

    def query(self, sql: str, params: tuple | list = ()) -> list[dict[str, Any]]:
        with self.db().cursor() as cursor:
            cursor.execute(sql, params)
            return list(cursor.fetchall())

    def one(self, sql: str, params: tuple | list = ()) -> dict[str, Any] | None:
        rows = self.query(sql, params)
        return rows[0] if rows else None

    def scalar(self, sql: str, params: tuple | list = ()) -> Any:
        row = self.one(sql, params)
        if row is None:
            return None
        return next(iter(row.values()))

    def execute(self, sql: str, params: tuple | list = ()) -> int:
        with self.db().cursor() as cursor:
            cursor.execute(sql, params)
            return cursor.rowcount

    def insert(self, sql: str, params: tuple | list = ()) -> int:
        with self.db().cursor() as cursor:
            cursor.execute(sql, params)
            return int(cursor.lastrowid or 0)

    # -- tracking / cleanup -------------------------------------------------

    def track(self, kind: str, resource_id: int, meta: Any = None) -> None:
        self._tracked[kind][int(resource_id)] = meta

    def untrack(self, kind: str, resource_id: int) -> None:
        self._tracked[kind].pop(int(resource_id), None)

    def _delete_vector_points(self, point_ids: list[int]) -> None:
        """Best-effort Qdrant cleanup so deleted test chunks do not crowd search."""

        if not point_ids:
            return
        try:
            self.http.post(
                f"{self.qdrant_url}/collections/{self.qdrant_collection}/points/delete",
                params={"wait": "true"},
                json={"points": point_ids},
                timeout=10,
            )
        except requests.RequestException:
            pass

    def prune_orphan_vectors(self) -> int:
        """Delete Qdrant points whose MySQL chunk row no longer exists."""

        try:
            point_ids: list[int] = []
            offset = None
            while True:
                body: dict[str, Any] = {
                    "limit": 1000,
                    "with_payload": False,
                    "with_vector": False,
                }
                if offset is not None:
                    body["offset"] = offset
                response = self.http.post(
                    f"{self.qdrant_url}/collections/{self.qdrant_collection}/points/scroll",
                    json=body,
                    timeout=10,
                )
                if response.status_code != 200:
                    return 0
                result = response.json().get("result") or {}
                point_ids.extend(int(point["id"]) for point in result.get("points", []))
                offset = result.get("next_page_offset")
                if offset is None:
                    break
            db_ids = {int(row["id"]) for row in self.query("SELECT id FROM knowledge_chunks")}
            orphans = [point_id for point_id in point_ids if point_id not in db_ids]
            self._delete_vector_points(orphans)
            return len(orphans)
        except Exception:  # pragma: no cover - optional auxiliary store
            return 0

    def remove_material(self, material_id: int) -> None:
        chunk_ids = [
            int(row["id"])
            for row in self.query(
                "SELECT id FROM knowledge_chunks WHERE material_id=%s", (material_id,)
            )
        ]
        self._delete_vector_points(chunk_ids)
        row = self.one("SELECT file_path FROM materials WHERE id=%s", (material_id,))
        self.execute("DELETE FROM materials WHERE id=%s", (material_id,))
        if row and row.get("file_path") and os.path.exists(row["file_path"]):
            try:
                os.remove(row["file_path"])
            except OSError:
                pass
        self.untrack("material", material_id)

    def remove_conversation(self, conversation_id: int) -> None:
        self.execute("DELETE FROM conversations WHERE id=%s", (conversation_id,))
        self.untrack("conversation", conversation_id)

    def remove_agent(self, agent_id: int) -> None:
        self.execute("DELETE FROM agents WHERE id=%s", (agent_id,))
        self.untrack("agent", agent_id)

    def remove_skill(self, skill_id: int) -> None:
        self.execute("DELETE FROM skills WHERE id=%s", (skill_id,))
        self.untrack("skill", skill_id)

    def remove_server(self, server_id: int) -> None:
        # Test cleanup only: real product deletions are exercised through the API.
        self.execute("DELETE FROM mcp_tool_calls WHERE server_id=%s", (server_id,))
        self.execute("DELETE FROM mcp_tools WHERE server_id=%s", (server_id,))
        self.execute("DELETE FROM mcp_servers WHERE id=%s", (server_id,))
        self.untrack("server", server_id)

    def cleanup(self) -> list[str]:
        errors: list[str] = []
        for conversation_id in list(self._tracked["conversation"]):
            try:
                self.remove_conversation(conversation_id)
            except Exception as exc:  # pragma: no cover - best effort
                errors.append(f"conversation {conversation_id}: {exc}")
        for agent_id in list(self._tracked["agent"]):
            try:
                self.remove_agent(agent_id)
            except Exception as exc:  # pragma: no cover - best effort
                errors.append(f"agent {agent_id}: {exc}")
        for skill_id in list(self._tracked["skill"]):
            try:
                self.remove_skill(skill_id)
            except Exception as exc:  # pragma: no cover - best effort
                errors.append(f"skill {skill_id}: {exc}")
        for material_id in list(self._tracked["material"]):
            try:
                self.remove_material(material_id)
            except Exception as exc:  # pragma: no cover - best effort
                errors.append(f"material {material_id}: {exc}")
        for server_id in list(self._tracked["server"]):
            try:
                self.remove_server(server_id)
            except Exception as exc:  # pragma: no cover - best effort
                errors.append(f"server {server_id}: {exc}")
        self.prune_orphan_vectors()
        return errors

    def close(self) -> None:
        if self._db is not None:
            try:
                self._db.close()
            finally:
                self._db = None
        self.http.close()
