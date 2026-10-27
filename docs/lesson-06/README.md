# 第 6 课验收记录：MCP 与工具接入

## 交付物

- `cmd/mcp-server/`：自建 MCP 工具服务，`POST /mcp` JSON-RPC 2.0，`GET /health`；Bearer 鉴权；`MCP_AUTH_TOKEN` 必填。
- 两个工具：
  - `formula_hint`：根据问题给公式/步骤提示，不给最终数值答案；inputSchema `{query:string}`。
  - `unit_convert`：米/厘米、千克/克换算；inputSchema `{value:number,from:string,to:string}`。
- 主机侧三张表：`mcp_servers`（配置）、`mcp_tools`（发现缓存）、`mcp_tool_calls`（审计，只增不改）。
- 管理接口：列表、登记、修改、删除、连接、断开、工具清单、调用审计；全部仅教师。
- 技能“工具”栏支持 `工具名@服务器名`；保存前四道校验。
- 对话调用：技能白名单确定性编排，调用点在推 `meta` 之前；`meta.tool_calls` 可见。
- 工具输出安全检查：指令式句式命中或超过 1200 字整条丢弃，替换固定说明。
- Swagger 补齐 MCP 接口；apitest 覆盖管理与端到端。

## 令牌边界

| 位置 | 是否接触令牌原文 |
|---|---|
| `mcp_servers.auth_token` | 是（服务端数据库） |
| 发起 `tools/call` | 是（`Authorization: Bearer`） |
| 列表/详情接口响应 | 否，只返回 `has_auth: true/false` |
| 前端表单 | 仅输入，提交后清空、不回显 |
| 日志 | 否 |

`GET /api/mcp/servers` 的整份 JSON 中搜不到令牌字符串。

## 本地验收命令与结果

```bash
# 服务单独可验
curl http://127.0.0.1:8090/health
# -> {"status":"ok"}

curl -X POST http://127.0.0.1:8090/mcp \
  -H "Authorization: Bearer $MCP_AUTH_TOKEN" -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}'
# -> result.tools 含 formula_hint、unit_convert

curl -X POST http://127.0.0.1:8090/mcp \
  -H "Authorization: Bearer wrong" -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
# -> 401
```

```bash
# 登记并连接
POST /api/mcp/servers
{"name":"campus-tools","base_url":"http://127.0.0.1:8090","auth_token":"..."}
# -> 201，响应只有 has_auth=true

POST /api/mcp/servers/1/connect
# -> 200，connected=true，tool_count=2，tools 为上面两个工具

# 连续连接两次后工具表仍为 2 行（覆盖而不是追加）

# 保存带工具引用的技能
工具：knowledge_search, formula_hint@campus-tools
# -> 201

# 清单外的工具名
工具：formula_hint@campus-tools, unit_convert@campus-tools（已知）
工具：nope@campus-tools
# -> 400，提示“发现清单中没有工具 nope”

# 提问命中技能
POST /api/conversations/{id}/messages/stream
{"content":"这道方程去括号怎么去"}
```

实际 `meta`：

```json
{
  "status": "ok",
  "skill": {"name": "数学解题引导（含工具）"},
  "citations": [{"index":1,"material_id":1,"material_title":"数学自学笔记","chunk_id":4,"chunk_index":0}],
  "tool_calls": [{
    "ref": "formula_hint@campus-tools",
    "status": "ok",
    "text": "去括号时，要把括号外的系数乘到括号内的每一项，并注意符号变化。先检查这一步，不要急着算出最终数值。"
  }]
}
```

`meta` 先于第一个 `delta`。审计新增一行：

```json
{
  "tool_ref": "formula_hint@campus-tools",
  "server_name": "campus-tools",
  "status": "ok",
  "username": "teacher_a",
  "conversation_id": 3,
  "detail": "去括号时，要把括号外的系数乘到括号内的每一项……"
}
```

## 断开、删除与改名

| 服务器状态 | 技能行 | 下一次提问 |
|---|---|---|
| 已断开 | 不变 | `tool_calls` 中该引用 `denied`，审计新增 `denied` |
| 已改名 | 引用仍是旧名 | 按旧名找不到服务器，`denied`，**不写审计** |
| 已删除 | 引用照旧 | 找不到服务器，`denied`，**不写审计** |
| 有审计记录时删除 | — | 503，保留证据 |

## 工具输出信任边界

- 工具返回与检索资料一样放在**用户消息**，不进入系统消息。
- 命中“忽略前述 / ignore previous / 你现在是 / system: / 请调用 / 发到某地址”等句式，或超过 1200 字，整条丢弃，替换为固定说明。
- 调用参数只来自提问原文或代码给定值，模型不能自行生成参数；模型不持有工具开关。

## 与任务清单的取舍说明

第 6 课任务清单提到“把工具声明交给模型，由模型提出调用”，而同一课实现教程明确采用“先调工具、再生成，模型不参与决定”，并解释这是为了可预期、可测试、可审计与注入面收敛。本仓库采用同一课更详细的实现教程方案：

- 调用由技能白名单决定，模型只拿到已执行的结果；
- 单次提问工具调用上限 `MAX_TOOL_CALLS_PER_REQUEST`（默认 8），超限的引用记为 `denied` 并跳过，回答继续生成；
- 无模型驱动的多轮循环，因此不存在“循环无法终止”的风险；
- 若后续课程要求模型原生 function calling，可在 `service` 层替换编排，不影响审计、令牌与输出检查。

## 同伴交叉验证（待同伴复核）

以下四项需要同伴以自己的账号在同一套环境验证并留存截图/日志；本仓库已提供可复现步骤：

1. **服务器与工具隔离**：同伴列出服务器，看不到本班之外的；用本班服务器编号直接访问，与不存在编号返回一致 404。
2. **令牌不回传**：同伴检查列表响应，整份 JSON 搜不到令牌，只有 `has_auth=true`。
3. **断开即失效**：同伴断开服务器后，用绑定其工具的技能提问，`tool_calls` 状态为 `denied`，审计新增拒绝记录。
4. **审计只看本班**：同伴打开调用记录，只看到自己班级的调用。

## 自动化验证结果

2026-10-27 在本机 MySQL 8.0.40 + Qdrant 1.12.4 + 自建 MCP 服务上运行：

```text
$ python3 apitest/run_all.py
...
------------------------------------------------------------------------------
55 passed, 0 failed, 55 total in 21.71s
HTML report: apitest/report.html
```

覆盖了 MCP 服务的健康检查与鉴权、登记/连接/发现/断开、令牌不回传、重复连接不翻倍、
工具引用保存校验、断开后的 `denied` 与审计、学生 403、有审计删除 503，以及端到端
“连接 → 绑定 → 提问 → `meta.tool_calls=ok` → 审计成功”。
