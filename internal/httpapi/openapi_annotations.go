package httpapi

// @Summary 健康检查
// @Description 返回服务存活状态，不访问数据库。
// @Tags system
// @Produce json
// @Success 200 {object} Health
// @Router /health [get]

// @Summary 用户登录
// @Description 校验用户名与密码并签发 JWT；凭据错误与登录限流统一返回 401。
// @Tags auth
// @Accept json
// @Produce json
// @Param body body LoginRequest true "用户名与密码"
// @Success 200 {object} LoginResponse
// @Failure 401 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Router /api/login [post]

// @Summary 退出登录
// @Description 吊销当前请求携带的 JWT。
// @Tags auth
// @Produce json
// @Security Bearer
// @Success 204 "已吊销令牌"
// @Failure 401 {object} ErrorResponse
// @Router /api/logout [post]

// @Summary 当前用户
// @Description 返回当前登录用户及其班级信息。
// @Tags auth
// @Produce json
// @Security Bearer
// @Success 200 {object} MeResponse
// @Failure 401 {object} ErrorResponse
// @Router /api/me [get]

// @Summary 材料列表
// @Description 只返回当前登录用户所在班级的材料，q 按标题模糊筛选。
// @Tags materials
// @Produce json
// @Security Bearer
// @Param q query string false "标题关键词"
// @Success 200 {object} MaterialList
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/materials [get]

// @Summary 材料详情
// @Description 返回材料正文与索引状态；跨班或不存在统一返回 404。
// @Tags materials
// @Produce json
// @Security Bearer
// @Param id path int true "编号"
// @Success 200 {object} MaterialDetail
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/materials/{id} [get]

// @Summary 下载材料原文
// @Description 返回材料原始文本；跨班或不存在统一返回 404。
// @Tags materials
// @Produce text/plain, application/octet-stream
// @Security Bearer
// @Param id path int true "编号"
// @Success 200 {file} string "材料原文"
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/materials/{id}/file [get]

// @Summary 材料向量视图
// @Description 仅教师
// @Description 返回材料切片的索引状态与字符数，不暴露向量分量。
// @Tags materials
// @Produce json
// @Security Bearer
// @Param id path int true "编号"
// @Success 200 {object} MaterialVectorsResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/materials/{id}/vectors [get]

// @Summary 上传材料
// @Description 仅教师
// @Description 接收 multipart 表单上传 .txt 或 .md 文本材料，保存后立即建立索引。
// @Tags materials
// @Accept multipart/form-data
// @Produce json
// @Security Bearer
// @Param body body UploadRequest true "上传表单"
// @Success 201 {object} UploadResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 413 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/materials [post]

// @Summary 重建材料索引
// @Description 仅教师
// @Description 使用新的切分参数清除并重建材料索引。
// @Tags materials
// @Accept json
// @Produce json
// @Security Bearer
// @Param id path int true "编号"
// @Param body body ReindexRequest true "切分参数"
// @Success 200 {object} ReindexResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/materials/{id}/reindex [post]

// @Summary 检索材料
// @Description 按关键词、向量或混合模式检索本班材料。
// @Tags search
// @Accept json
// @Produce json
// @Security Bearer
// @Param body body SearchRequest true "检索条件"
// @Success 200 {object} SearchResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/search [post]

// @Summary 基于材料问答
// @Description 基于本班材料回答问题并返回引用；无依据时不调用模型。
// @Tags search
// @Accept json
// @Produce json
// @Security Bearer
// @Param body body AskRequest true "问题与历史"
// @Success 200 {object} AskResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Router /api/ask [post]

// @Summary 读取默认提示词
// @Description 返回本班默认系统提示词。
// @Tags prompt
// @Produce json
// @Security Bearer
// @Success 200 {object} Prompt
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/prompt [get]

// @Summary 更新默认提示词
// @Description 仅教师
// @Description 更新本班默认系统提示词，最长 2000 字。
// @Tags prompt
// @Accept json
// @Produce json
// @Security Bearer
// @Param body body Prompt true "提示词"
// @Success 200 {object} Prompt
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/prompt [put]

// @Summary 技能列表
// @Description 列出本班技能及其工具绑定。
// @Tags skills
// @Produce json
// @Security Bearer
// @Success 200 {object} SkillList
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/skills [get]

// @Summary 创建技能
// @Description 仅教师
// @Description 从技能文件源文本创建技能；工具必须已连接且出现在发现清单中。
// @Tags skills
// @Accept json
// @Produce json
// @Security Bearer
// @Param body body SkillInput true "技能文件源文本"
// @Success 201 {object} Skill
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/skills [post]

// @Summary 更新技能
// @Description 仅教师
// @Description 用新的技能文件源文本替换技能。
// @Tags skills
// @Accept json
// @Produce json
// @Security Bearer
// @Param id path int true "编号"
// @Param body body SkillInput true "技能文件源文本"
// @Success 200 {object} Skill
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/skills/{id} [put]

// @Summary 删除技能
// @Description 仅教师
// @Description 删除本班技能。
// @Tags skills
// @Produce json
// @Security Bearer
// @Param id path int true "编号"
// @Success 204 "已删除"
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/skills/{id} [delete]

// @Summary 智能体列表
// @Description 学生只看到启用智能体，教师可见全部。
// @Tags agents
// @Produce json
// @Security Bearer
// @Success 200 {object} AgentList
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/agents [get]

// @Summary 智能体详情
// @Description 学生访问停用或他班智能体统一返回 404。
// @Tags agents
// @Produce json
// @Security Bearer
// @Param id path int true "编号"
// @Success 200 {object} Agent
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/agents/{id} [get]

// @Summary 创建智能体
// @Description 仅教师
// @Description 创建智能体；名称班内唯一，技能与材料绑定只接受本班资源。
// @Tags agents
// @Accept json
// @Produce json
// @Security Bearer
// @Param body body AgentInput true "智能体配置"
// @Success 201 {object} Agent
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/agents [post]

// @Summary 更新智能体
// @Description 仅教师
// @Description 更新智能体配置；名称班内唯一。
// @Tags agents
// @Accept json
// @Produce json
// @Security Bearer
// @Param id path int true "编号"
// @Param body body AgentInput true "智能体配置"
// @Success 200 {object} Agent
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/agents/{id} [put]

// @Summary 删除智能体
// @Description 仅教师
// @Description 删除智能体；已有会话引用时返回 409。
// @Tags agents
// @Produce json
// @Security Bearer
// @Param id path int true "编号"
// @Success 204 "已删除"
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/agents/{id} [delete]

// @Summary 会话列表
// @Description 只列出当前用户自己的会话。
// @Tags conversations
// @Produce json
// @Security Bearer
// @Success 200 {object} ConversationList
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/conversations [get]

// @Summary 创建会话
// @Description 创建会话；智能体必须属于本班且已启用。
// @Tags conversations
// @Accept json
// @Produce json
// @Security Bearer
// @Param body body ConversationCreateRequest true "智能体编号"
// @Success 201 {object} Conversation
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/conversations [post]

// @Summary 会话详情
// @Description 返回会话及其消息；跨班或他人会话统一返回 404。
// @Tags conversations
// @Produce json
// @Security Bearer
// @Param id path int true "编号"
// @Success 200 {object} ConversationDetail
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/conversations/{id} [get]

// @Summary 流式对话
// @Description 向会话追加用户消息，以 SSE 返回 meta、delta、done 事件；失败时返回 error 事件。
// @Tags conversations
// @Accept json
// @Produce text/event-stream
// @Security Bearer
// @Param id path int true "编号"
// @Param body body StreamRequest true "用户消息"
// @Success 200 {string} string "SSE 事件流（meta/delta/done）"
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Router /api/conversations/{id}/messages/stream [post]

// @Summary MCP 服务器列表
// @Description 仅教师
// @Description 列出本班 MCP 服务器；绝不回传令牌。
// @Tags mcp
// @Produce json
// @Security Bearer
// @Success 200 {object} MCPServerList
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/mcp/servers [get]

// @Summary 注册 MCP 服务器
// @Description 仅教师
// @Description 注册 MCP 服务器；名称需匹配 ^[a-z][a-z0-9_-]{0,63}$，同班同名返回 400。
// @Tags mcp
// @Accept json
// @Produce json
// @Security Bearer
// @Param body body MCPServerInput true "服务器配置"
// @Success 201 {object} MCPServer
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/mcp/servers [post]

// @Summary 更新 MCP 服务器
// @Description 仅教师
// @Description 更新服务器；省略 auth_token 表示不修改，修改会清空连接与工具缓存。
// @Tags mcp
// @Accept json
// @Produce json
// @Security Bearer
// @Param id path int true "编号"
// @Param body body MCPServerInput true "服务器配置"
// @Success 200 {object} MCPServer
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/mcp/servers/{id} [put]

// @Summary 删除 MCP 服务器
// @Description 仅教师
// @Description 删除服务器；存在审计记录时返回 503 以保留证据。
// @Tags mcp
// @Produce json
// @Security Bearer
// @Param id path int true "编号"
// @Success 204 "已删除"
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Router /api/mcp/servers/{id} [delete]

// @Summary 连接 MCP 服务器
// @Description 仅教师
// @Description 握手并拉取 tools/list，覆盖工具缓存；失败返回 502 且保持未连接。
// @Tags mcp
// @Produce json
// @Security Bearer
// @Param id path int true "编号"
// @Success 200 {object} MCPToolsResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Failure 502 {object} ErrorResponse
// @Router /api/mcp/servers/{id}/connect [post]

// @Summary 断开 MCP 服务器
// @Description 仅教师
// @Description 清空工具缓存并标记为未连接。
// @Tags mcp
// @Produce json
// @Security Bearer
// @Param id path int true "编号"
// @Success 204 "已断开"
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/mcp/servers/{id}/disconnect [post]

// @Summary MCP 工具清单
// @Description 仅教师
// @Description 返回服务器已发现的工具清单。
// @Tags mcp
// @Produce json
// @Security Bearer
// @Param id path int true "编号"
// @Success 200 {object} MCPToolsResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/mcp/servers/{id}/tools [get]

// @Summary MCP 工具调用审计
// @Description 仅教师
// @Description 返回本班 MCP 工具调用审计，默认 50 条、最多 100 条。
// @Tags mcp
// @Produce json
// @Security Bearer
// @Param limit query int false "返回条数，默认 50，上限 100"
// @Success 200 {object} ToolCallList
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/mcp/tool-calls [get]

// @Summary OpenAPI 文档
// @Description 返回生成的 OpenAPI 3.0.3 JSON 文档。
// @Tags docs
// @Produce json
// @Success 200 {object} OpenAPIDocument
// @Failure 404 {object} ErrorResponse
// @Router /openapi.json [get]

// @Summary Swagger UI 跳转
// @Description 重定向到 Swagger UI。
// @Tags docs
// @Success 302 "重定向到 /swagger/"
// @Router /swagger [get]

// @Summary Swagger UI
// @Description Swagger UI 页面，可直接试调接口。
// @Tags docs
// @Produce text/html
// @Success 200 {string} string "Swagger UI 页面"
// @Failure 404 {object} ErrorResponse
// @Router /swagger/ [get]

// @Summary Web 首页
// @Description CampusClaw Web 首页。
// @Tags system
// @Produce text/html
// @Success 200 {string} string "首页 HTML"
// @Failure 404 {object} ErrorResponse
// @Router / [get]

// @Summary 静态资源
// @Description 返回 web/static 下的静态资源文件。
// @Tags system
// @Produce application/octet-stream
// @Param path path string true "静态文件相对路径"
// @Success 200 {file} string "文件内容"
// @Failure 404 {object} ErrorResponse
// @Router /static/{path} [get]
