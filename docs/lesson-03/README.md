# 第 3 课验收记录：认证授权与知识库入库

## 交付物

- Go 服务：`cmd/server/`，分层 `httpapi → service → store → MySQL/Qdrant/模型网关`。
- MySQL 8：`classes`、`users`、`sessions` 对应的 JWT 吊销表、`materials`、`knowledge_entries`，以及第 2 课要求的 `handouts`、`assignments` 占位表。
- 上传目录只挂载给 API 容器，不经 Nginx 静态暴露；文件名由服务端随机生成。
- `docker-compose.yml`：`web`（唯一映射端口）→ `api` → `db`/`qdrant`/`mcp-demo`；数据卷持久化。
- `/health` 免认证，只做存活判定，不查数据库。

## 与第 2 课规约的对应

| 规约条款 | 实现 | 证据 |
|---|---|---|
| 教师/学生登录 | `POST /api/login`，bcrypt 校验，签发 HS256 JWT | 登录成功返回 `token` 与 `user`；错误口令统一 401 |
| 未登录访问受保护接口 | Bearer 中间件 | `/api/me` 无令牌返回 401，响应不含材料标题 |
| 学生不能上传 | `RequireTeacher` | 学生 `POST /api/materials` 返回 403，数据库无新行 |
| 班级是数据边界 | 查询一律带 `user.ClassID` | A 班读取 B 班材料详情/下载均 404，与不存在一致 |
| 上传入库 | 文件落盘 + `materials` + `knowledge_entries` 同事务 | 上传后 `GET /api/materials` 可见，`body_text` 可查 |
| 预置数据 | `internal/db/seed.go` 幂等种子 | A/B 班、教师/学生账号、两班可区分材料 |
| 密码哈希 | bcrypt cost 12 | `users.password_hash` 以 `$2a$` 开头，无明文 |
| 密钥只从环境变量 | `internal/config` 校验 | 缺 `JWT_SECRET`、`DB_PASSWORD` 或 `MCP_AUTH_TOKEN` 时启动失败 |
| Compose 与 `/health` | Compose + 存活探针 | `docker compose config` 通过；`GET /health` 返回 `{"status":"ok"}` |

## 认证方案冲突处理

第 3 课要求服务端会话 Cookie，并明确“本迭代不使用 JWT”。第 5 课把同一系统迭代为 Bearer JWT（HS256 + 吊销表），任务清单也要求“登录后用 Bearer 令牌访问接口”。本仓库按“以后面的课为准”实现 JWT，但保留第 3 课的安全目标：

- 角色与班级不写入令牌声明，每次请求回库 `JOIN users/classes` 读取；
- 登出把 `jti` 写入 `token_revocations`，旧令牌立即 401；
- 令牌由前端显式放入 `Authorization`，不存 Cookie；
- 密码仍是 bcrypt，密钥仍只从环境变量读取。

## 本地验收命令与结果

```bash
# 登录
curl -s -X POST http://127.0.0.1:18081/api/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"teacher_a","password":"Campus@2026"}'
# -> 200，返回 token、expires_at、user{role:teacher,class_id:1}

# 未登录
curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:18081/api/me
# -> 401

# 学生上传
curl -s -o /dev/null -w '%{http_code}' -X POST http://127.0.0.1:18081/api/materials \
  -H "Authorization: Bearer $STUDENT_TOKEN" -F file=@/tmp/hi.txt
# -> 403

# 跨班详情
curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:18081/api/materials/3 \
  -H "Authorization: Bearer $A_TOKEN"
# -> 404
```

数据库核对：

```sql
SELECT id,class_id,title FROM materials ORDER BY id;
SELECT material_id,class_id,LEFT(body_text,40) FROM knowledge_entries ORDER BY id;
SELECT username,role,class_id,LEFT(password_hash,4) FROM users;
```

结果：A 班材料只属于 `class_id=1`，B 班材料只属于 `class_id=2`；`knowledge_entries` 与 `materials` 一一对应；`password_hash` 均为 `$2a$` 前缀，无明文。

## 失败路径

- 学生上传：403，且数据库无新 `materials` 行、上传目录无新文件。
- 跨班详情/下载：404，与不存在完全一致。
- 未登录：401，不返回任何材料字段。
- 错误口令：401，提示与“用户不存在”一致；5 次/300 秒后仍返回 401，不暴露限流状态。
- 缺少密钥：进程启动即失败，不回落默认值。

## 自动化验证

`apitest/run_all.py` 在 2026-10-27 全量运行：55 个用例通过，0 失败；报告写入 `apitest/report.html`（不入库）。
