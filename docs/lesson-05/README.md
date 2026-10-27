# 第 5 课验收记录：提示词、AI 技能与流式会话

## 交付物

- `agents`：名称、提示词、启用状态；`agent_skills`、`agent_materials` 两张绑定表。
- `skills`：单文件定义；元数据（名称、适用条件、资料、工具、启用）+ 空行 + 引导正文；教师可增删改，班内名称唯一。
- `conversations`、`messages`：会话归属用户与班级，创建时锁定本班已启用智能体；消息保存引用、技能名与工具调用。
- `POST /api/conversations/{id}/messages/stream`：SSE `meta → delta × n → done`，失败补 `error`。
- `/api/prompt` 班级默认提示词；`/api/agents` 每个智能体的角色提示词。
- Swagger：`/swagger/` 可对照 `/openapi.json` 试调。
- `apitest/` 一个接口一个脚本，`run_all.py` 生成 HTML 报告。

## 一次回答的链路

1. 按会话编号 + 当前用户 + 班级读取会话；不满足即 404。
2. 读取会话锁定的智能体；停用的智能体仍可供已有会话使用，只拦截新建。
3. 候选技能 = 该智能体绑定且启用的技能；按适用条件做小写词法计分，至多一个，同分取编号小者。
4. 提问先落库；首轮用前 24 字改写标题。
5. 只在该智能体绑定的材料内做混合检索（技能有资料范围时再按标题过滤），取前 4 条。
6. 推 `meta`：状态、技能名、引用列表；引用编号从 1 连续。
7. 系统消息只放提示词、技能说明、边界句；编号资料与工具返回放用户消息。
8. 追加最近 6 条历史（每条 500 字）。
9. 模型逐段生成；无依据时不写 `[n]`，并在推送前剥掉编号。
10. 助手消息落库（含 citations、skill_name、tool_calls），推 `done`。

## 本地验收命令与结果

```bash
# 新建会话（选定数学解题助手）
POST /api/conversations {"agent_id":1}
# -> 201 {"id":...,"agent_id":1,"title":"新的提问"}

# 流式提问
curl -N -X POST /api/conversations/1/messages/stream \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"content":"这道方程去括号怎么去"}'
```

实际帧（节选）：

```text
event: meta
data: {"status":"ok","skill":{"name":"数学解题引导"},"citations":[{"index":1,"material_id":1,"material_title":"数学自学笔记","chunk_id":4,"chunk_index":0}]}

event: delta
data: {"text":"先别急着算最终结"}

event: delta
data: {"text":"果。根据本班材料"}

...

event: done
data: {"message_id":3}
```

- `meta` 先到；多个 `delta` 每次只含新增文本；拼接后等于最终回答。
- `done` 返回助手消息编号；数据库 `messages` 中该行含 `citations_json` 与 `skill_name`。
- 无依据问题（如“请讲一个和课程无关的笑话”）：`meta.citations=[]`，正文无 `[n]`，回答不整段等于“资料中未找到相关内容”。
- 技能停用/解绑后，同一句提问不再命中该技能，系统消息改为“没有匹配到已启用的学科技能”，行为立即变化。
- 跨班/他人会话：404，且与不存在一致；学生写技能/智能体：403。

## SSE 工程细节

- Nginx：`proxy_buffering off`、`proxy_cache off`、读超时 120s。
- Go：`Content-Type: text/event-stream`、`Cache-Control: no-cache`、`X-Accel-Buffering: no`，每帧 `Flush`。
- 第一个事件产生前可返回 401/404/400/503；流开始后状态码固定 200，后续失败只能发 `error`。
- 浏览器用 `fetch` POST 读取 `response.body` 并自行按空行解帧（`EventSource` 只能 GET）。

## 边界与失败路径

| 情形 | 结果 |
|---|---|
| 提问 <2 或 >500 字 | 400 |
| 会话不存在/他人/跨班 | 404 |
| 新建会话指定停用/他班/不存在智能体 | 404；本班一个启用智能体都没有时 400 |
| 学生调用智能体/技能写接口 | 403 |
| 提示词为空 | 回退默认角色，不拒绝回答 |
| 检索落空 | 仍作答，citations 为空，正文无编号 |
| 模型网关失败 | 离线回退或 `error` 事件；不泄露上游响应 |

## 自动化验证

`apitest/run_all.py` 在 2026-10-27 全量运行：35 个用例通过，0 失败；报告写入 `apitest/report.html`（不入库）。
