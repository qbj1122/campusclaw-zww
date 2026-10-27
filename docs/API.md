# CampusClaw API 契约（第 1–6 课汇总）

所有业务接口以 `Authorization: Bearer <JWT>` 鉴权（登录接口除外）。错误统一为：

```json
{"error":{"code":"bad_request|unauthorized|forbidden|not_found|conflict|too_large|unavailable|internal|bad_gateway","message":"可读中文"}}
```

班级一律取自登录用户；请求体中的 `class_id` 不被采信。跨班与不存在对按编号访问的资源返回一致的 404。

## 认证

| 方法 | 路径 | 角色 | 说明 |
|---|---|---|---|
| POST | `/api/login` | 公开 | `{username,password}` → `{token,expires_at,user:{id,username,role,class_id,class_name}}` |
| POST | `/api/logout` | 登录 | 吊销当前 JWT，204 |
| GET | `/api/me` | 登录 | `{user:{...}}` |
| GET | `/health` | 公开 | `{"status":"ok"}`，不查数据库 |

## 材料与检索

| 方法 | 路径 | 角色 | 说明 |
|---|---|---|---|
| GET | `/api/materials?q=` | 登录 | `{items:[{id,title,class_id,uploaded_by,created_at,size_bytes,chunk_count,index_status}]}` |
| GET | `/api/materials/{id}` | 登录 | 材料详情，含 `body_text`、`chunk_count`、`index_status` |
| GET | `/api/materials/{id}/file` | 登录 | 原文下载；跨班 404 |
| GET | `/api/materials/{id}/vectors` | 教师 | 向量视图：`{material_id,chunks:[{chunk_id,chunk_index,index_status,strategy,chars}]}`，不暴露向量分量 |
| POST | `/api/materials` | 教师 | multipart：`file`(.txt/.md)、`title?`、`strategy?(auto/custom/hierarchy)`、`chunk_max_chars?`、`chunk_overlap?`、`strip_urls?=1`、`collapse_whitespace?=1` → 201 `{id,title,chunks,index_error?}` |
| POST | `/api/materials/{id}/reindex` | 教师 | `{strategy,chunk_max_chars,chunk_overlap,strip_urls,collapse_whitespace}` → `{chunks,strategy}` |
| POST | `/api/search` | 登录 | `{q,mode:keyword|vector|hybrid,limit,offset}` → `{status:ok|no_evidence,mode,hits:[{material_id,material_title,chunk_id,chunk_index,start_offset,end_offset,excerpt,score,rank,keyword_rank?,vector_rank?}]}` |
| POST | `/api/ask` | 登录 | `{q,history:[{role,content}]}` → `{status:ok|no_evidence,answer,citations:[{index,material_id,material_title,chunk_id,chunk_index}]}`；无依据时回答固定文案且不调用模型 |

## 提示词、智能体、技能

| 方法 | 路径 | 角色 | 说明 |
|---|---|---|---|
| GET/PUT | `/api/prompt` | 登录/教师 | `{prompt}`，班级默认提示词 |
| GET | `/api/prompt-templates` | 教师 | `{items:[{id,name,description,content,variables:[],...}]}` |
| GET | `/api/prompt-templates/{id}` | 教师 | 模板详情 |
| POST | `/api/prompt-templates` | 教师 | `{name,description,content}` → 201；变量只允许 `class_name/agent_name/student_name/skill_name/subject/date/materials` |
| PUT | `/api/prompt-templates/{id}` | 教师 | 同上 |
| DELETE | `/api/prompt-templates/{id}` | 教师 | 204；引用该模板的智能体回退到 `prompt` |
| GET | `/api/prompt-variables` | 教师 | `{items:[{name,description}]}` |
| POST | `/api/prompt-templates/{id}/preview` | 教师 | `{content,rendered}`，用示例值渲染 |
| GET | `/api/skills` | 登录 | `{items:[{id,name,trigger_text,description,tools:[],subject_text,enabled,source_text,...}]}` |
| POST | `/api/skills` | 教师 | `{source_text}`；元数据空行 + 正文；工具仅允许 `knowledge_search` 或 `工具名@服务器名`，保存前校验服务器已连接且工具在发现清单 |
| POST | `/api/skills/upload` | 教师 | multipart `file`（`.md/.txt`，≤64KB）→ 201 技能 |
| PUT | `/api/skills/{id}/upload` | 教师 | multipart `file`，替换现有技能 |
| PUT | `/api/skills/{id}` | 教师 | 同上 |
| DELETE | `/api/skills/{id}` | 教师 | 204 |
| GET | `/api/agents` | 登录 | 学生只见启用；`{items:[{id,name,prompt,enabled,skill_ids,material_ids,...}]}` |
| GET | `/api/agents/{id}` | 登录 | 学生访问停用/他班 = 404 |
| POST/PUT | `/api/agents[/{id}]` | 教师 | `{name,prompt,prompt_template_id,enabled,skill_ids,material_ids}`；班内名称唯一，绑定只接受本班；模板非空时按模板渲染，`prompt` 为回退 |
| DELETE | `/api/agents/{id}` | 教师 | 204 |

## AI 应用目录

| 方法 | 路径 | 角色 | 说明 |
|---|---|---|---|
| GET | `/api/apps` | 登录 | 学生仅看已启用；`{items:[{id,name,description,category,icon,prompt_template_id,prompt_template_name,default_prompt,default_skill_ids,default_material_ids,enabled,agent_count,...}]}` |
| GET | `/api/apps/{id}` | 登录 | 学生访问停用/他班 = 404 |
| POST/PUT | `/api/apps[/{id}]` | 教师 | `{name,description,category,icon,prompt_template_id,default_prompt,skill_ids,material_ids,enabled}`；班内名称唯一 |
| DELETE | `/api/apps/{id}` | 教师 | 204；已有智能体保留，仅解除 `app_id` 关联 |
| POST | `/api/apps/{id}/instantiate` | 教师 | `{name?}` → 201 智能体；默认名 `<应用名>助手`，重名时需换名 |

技能文件格式：

```
名称：数学解题引导
适用条件：方程,去括号,分数,通分
资料：数学
工具：knowledge_search, formula_hint@campus-tools
启用：是

（空行以下为引导正文）
```

## 会话与流式回答

| 方法 | 路径 | 角色 | 说明 |
|---|---|---|---|
| GET | `/api/conversations` | 登录 | 只列本人 `{items:[...]}` |
| POST | `/api/conversations` | 登录 | `{agent_id}` → 201 会话；必须是本班已启用智能体 |
| GET | `/api/conversations/{id}` | 登录 | `{conversation,messages:[...]}`；跨班/他人 404 |
| POST | `/api/conversations/{id}/messages/stream` | 登录 | `{content}` → `text/event-stream` |

SSE 帧：

```
event: meta
data: {"status":"ok","skill":{"name":"数学解题引导"},"citations":[...],"tool_calls":[{"ref":"formula_hint@campus-tools","status":"ok","text":"..."}]}

event: delta
data: {"text":"新增片段"}

event: done
data: {"message_id":123}
```

`tool_calls` 只在确有调用时出现；`citations` 始终为数组。检索落空时仍生成回答、`citations` 为空、正文不得含 `[n]`。

## MCP（全部仅教师）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/mcp/servers` | `{items:[{id,name,base_url,has_auth,connected,tool_count,...}]}`，绝不回传令牌 |
| POST | `/api/mcp/servers` | `{name,base_url,auth_token}`；名称 `^[a-z][a-z0-9_-]{0,63}$`；同班同名 400 |
| PUT | `/api/mcp/servers/{id}` | `{name,base_url,auth_token?}`；省略令牌=不修改；修改清空连接与缓存 |
| DELETE | `/api/mcp/servers/{id}` | 有审计记录时 503，保留证据 |
| POST | `/api/mcp/servers/{id}/connect` | 握手 + `tools/list`，覆盖缓存；失败 502 且保持未连接 |
| POST | `/api/mcp/servers/{id}/disconnect` | 清空工具缓存并标记未连接，204 |
| GET | `/api/mcp/servers/{id}/tools` | `{server,tools:[{name,description,input_schema}]}` |
| GET | `/api/mcp/tool-calls?limit=` | 本班审计，默认 50，上限 100；`{items:[{tool_ref,server_name,status,detail,username,conversation_id,created_at}]}` |

## 文档

- `GET /openapi.json`
- `GET /swagger/`（Swagger UI，可试调）
