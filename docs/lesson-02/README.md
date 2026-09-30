# 第 2 课验收记录：SDD 与 CampusClaw 规约

## 交付物

- 仓库根：`README.md`（定位与本机运行）、`AGENTS.md`（无规约不写代码与分层/安全规则）、`openspec/config.yaml`。
- `openspec/changes/archive/*-add-auth-rbac-class-knowledge/`：
  - `proposal.md`：Why / What Changes / Capabilities / Impact / Non-goals。
  - `design.md`：Go + MySQL 8 + Qdrant + Nginx + Compose 的选型与取舍、JWT 认证、班级隔离、上传数据流、数据模型。
  - `specs/auth-upload/spec.md`：登录、角色、隔离、上传、种子、密码哈希、Compose/health 等 Requirement 与 Scenario。
  - `tasks.md`：按“骨架 → 种子 → 登录 → 隔离 → 上传 → Compose → validate”排序，每项带 `verify`，归档时已勾选。
- `openspec/specs/auth-upload/spec.md`：归档后合并出的长期能力规约。
- 后续三个变更同样以四件套归档：`traceable-retrieval`、`assistant-streaming`、`mcp-tools`。

## 规约验收

```bash
openspec validate --specs --strict
openspec list --specs
openspec validate --archived
```

要求：

- 每条能力至少一组 Scenario；失败路径不少于四条（未登录、学生上传、跨班访问、密钥缺失、无依据、断开 MCP 等）。
- Scenario 写成“前置条件 / 操作 / 可观测结果”，数值具体（800 字、80 字重叠、0.35、6 条、500 字、k=60、50/100 条）。
- 新开一个不带历史上下文的 Harness 对话，只给仓库路径与变更名，能按 `tasks.md` 复现骨架与主路径。

## 四件套职责

| 文件 | 回答的问题 | 本仓示例 |
|---|---|---|
| `proposal.md` | 为什么做、做什么、明确不做什么 | 登录/隔离/上传；Non-goals 排除 Homework、SSO、生产 HA |
| `design.md` | 怎么做、为什么这样选 | Go/MySQL 而非 Flask/SQLite；JWT 迭代；Qdrant 只存向量 |
| `specs/<能力>/spec.md` | 做到什么算合格 | 跨班 404、学生 403、上传后列表可见、无依据固定文案 |
| `tasks.md` | 按什么顺序做、如何验证 | 每项 `verify:`；骨架在前，业务在后 |

## 冲突与修订记录

- 第 2 课初稿允许跨班 403 或 404；第 3 课要求“与不存在一致”，归档规约统一为 404。
- 第 2 课 design 示例为 Flask/SQLite；第 3 课明确 Go + MySQL，归档 design 记录“技术栈变更需先改规约”。
- 第 3 课要求 Cookie + Session；第 5 课迭代为 Bearer JWT。归档中保留两个决策的上下文，最终实现以 JWT 为准。
