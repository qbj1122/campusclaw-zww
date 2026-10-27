# CampusClaw

面向中小学教研场景的智能互联网应用：登录与班级隔离、教研材料入库、可追溯混合检索、提示词与技能驱动的流式解题助手，以及带鉴权与审计的 MCP 工具调用。同一仓库按第 1–6 课增量演进，最终形态是一套可部署、可演示、可验收的系统。

## 快速开始（Docker Compose）

```bash
cp .env.example .env
# 至少修改 .env 中的 DB_PASSWORD、MYSQL_ROOT_PASSWORD、JWT_SECRET、MCP_AUTH_TOKEN
docker compose up -d --build
open http://localhost:8080
```

首次启动会建表并写入种子数据。`docker compose down` 不会删除数据卷；只有加 `-v` 才会清空。

演示账号（密码均为 `SEED_PASSWORD`，默认 `Campus@2026`）：

| 账号 | 角色 | 班级 |
|---|---|---|
| `teacher_a` | 教师 | 班级A |
| `student_a1` | 学生 | 班级A |
| `teacher_b` | 教师 | 班级B |
| `student_b1` | 学生 | 班级B |

班级A预置《数学自学笔记》《语文自学笔记》与两个解题助手；班级B预置《B班物理笔记》，其中含仅用于隔离验证的独有词“量子纠缠草莓”。

## 本地开发

依赖：Go 1.24、MySQL 8、Qdrant，可选 Docker。模型网关不是启动必需项。

```bash
cp .env.example .env
# 把 DB_HOST 改为 127.0.0.1，DB_PORT 改为你的 MySQL 端口，QDRANT_URL 改为本地地址
set -a; . ./.env; set +a
go run ./cmd/server
```

- API 默认 `http://127.0.0.1:8080`，静态页面与 Swagger 也从同一进程提供。
- MCP 工具服务：`go run ./cmd/mcp-server`（需要 `MCP_AUTH_TOKEN`，默认端口 8090）。
- `GET /health` 只表示进程存活，不检查数据库。
- `CHAT_API_KEY` 为空时使用确定性的离线演示回答；`EMBEDDING_API_KEY` 为空时使用字符二元组哈希向量，并在向量路径额外做词面重合校验。配置网关后自动切换为 OpenAI 兼容的 `/v1/chat/completions` 与 `/v1/embeddings`。

## 功能与课程对应

| 课次 | 交付 |
|---|---|
| 第 1 课 | `prototype/lesson01-timetable/` 氛围编程小练习；记录“能运行 ≠ 可验收”的对照 |
| 第 2 课 | OpenSpec 规约四件套；`openspec/specs/` 中的长期能力规约 |
| 第 3 课 | Go + MySQL 骨架、JWT 登录、角色权限、服务端班级隔离、教师上传入库、种子数据、Compose 与 `/health` |
| 第 4 课 | 三种切分策略、MySQL 切片与全文索引、Qdrant 向量、keyword/vector/hybrid+RRF、引用标注、无依据不调用模型 |
| 第 5 课 | 智能体与绑定、提示词、技能文件解析与确定性路由、会话与消息、SSE 流式回答、Swagger |
| 第 6 课 | 自建 `mcp-server/`、服务器登记/连接/发现/断开、`tool@server` 技能绑定、确定性工具调用、输出安全检查、调用审计 |

关键行为：

- **班级只来自登录身份**。请求体里的 `class_id` 不生效；跨班检索返回空结果，按编号访问跨班资源与不存在一致返回 404。
- **认证**采用 HS256 Bearer JWT；角色与班级每次回库读取，不信任令牌声明。登出写入吊销表。
- **检索**先查 MySQL 切片正文，Qdrant 只存向量与标识；向量主键等于 `knowledge_chunks.id`。混合检索按名次做 RRF（k=60），向量余弦阈值默认 0.35。
- **流式**使用 SSE：`meta` 先带技能、引用与本次 `tool_calls`，随后多个 `delta`，最后 `done`。Nginx 显式关闭缓冲。
- **MCP 工具**先调用、再生成，调用由技能白名单决定而非模型自行决定；工具返回与资料一样进入用户消息，并先过指令式句式与长度检查。令牌只在服务端，接口只返回 `has_auth`。

## 接口与文档

- 接口契约：`docs/API.md`
- OpenAPI：`GET /openapi.json`，由 `internal/httpapi/openapi_annotations.go` 的注解生成（`make openapi`）
- Swagger UI：`GET /swagger/`
- 规约：`openspec/specs/`；历史变更与验收记录：`openspec/changes/archive/`、`docs/lesson-0*/`

## 测试

```bash
make test                 # Go 单元测试
go vet ./...
python3 apitest/run_all.py # 需要先启动 API/MySQL/Qdrant/MCP，报告写入 apitest/report.html
```

`apitest/` 一个接口一个脚本，按 URL 组织，例如 `apitest/cases/api/login/POST.py` 对应 `POST /api/login`。`run_all.py` 一键执行并生成 HTML 报告；报告不随仓库提交。

## 安全与运维

- `.env`、上传目录、构建产物、测试报告均在 `.gitignore` 中；仓库只保留 `.env.example`。
- 密码使用 bcrypt（cost 12）；JWT 密钥、数据库口令、MCP 令牌只从环境变量读取，缺失即启动失败。
- 日志为一行一条 JSON，带 `request_id`、`user_id`、`class_id`、状态与耗时；不记录令牌、口令与模型密钥。
- 删除已有调用审计的 MCP 服务器会被拒绝（503），以保留证据；改名或删除后，旧技能引用在调用时记为 `denied`，且因找不到服务器不写审计。

## 技术栈与课程冲突处理

采用课程后段明确使用的 **Go + MySQL 8 + Qdrant + Nginx + Docker Compose**。第 3 课曾要求 Cookie + Session 且明确不引入 JWT，第 5 课把认证迭代为 Bearer JWT；按“以后面的课为准”，本仓库最终实现 Bearer JWT，角色与班级仍回库读取。第 2 课的 Flask/SQLite 示例与跨班可用 403/404 的选择，分别被第 3 课的 Go/MySQL 与“跨班与不存在一致 404”取代。所有取舍写入 `docs/lesson-0*/` 与 OpenSpec `design.md`。

## 目录

```
cmd/server/            API 入口
cmd/mcp-server/        自建 MCP 工具服务
internal/              分层：httpapi → service → store → MySQL/Qdrant/模型网关
web/                   静态前端与 Swagger
nginx/                 反向代理（SSE 关闭缓冲）
mcp-server/            MCP 服务 Dockerfile 与说明
openspec/              规约与归档变更
apitest/               接口测试与 HTML 报告
docs/                  接口契约、各课验收记录
prototype/             第 1 课氛围编程练习
```
