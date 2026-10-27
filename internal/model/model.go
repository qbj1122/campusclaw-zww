package model

import "time"

type User struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name,omitempty"`
	Role        string `json:"role"`
	ClassID     int64  `json:"class_id"`
	ClassName   string `json:"class_name,omitempty"`
}

type Material struct {
	ID          int64     `json:"id"`
	ClassID     int64     `json:"class_id"`
	UploadedBy  int64     `json:"uploaded_by"`
	Title       string    `json:"title"`
	FilePath    string    `json:"-"`
	MimeType    string    `json:"mime_type,omitempty"`
	SizeBytes   int64     `json:"size_bytes"`
	CreatedAt   time.Time `json:"created_at"`
	ChunkCount  int       `json:"chunk_count,omitempty"`
	IndexStatus string    `json:"index_status,omitempty"`
}

type KnowledgeEntry struct {
	ID         int64     `json:"id"`
	MaterialID int64     `json:"material_id"`
	ClassID    int64     `json:"class_id"`
	BodyText   string    `json:"body_text"`
	CreatedAt  time.Time `json:"created_at"`
}

type Chunk struct {
	ID               int64  `json:"id"`
	ClassID          int64  `json:"class_id"`
	MaterialID       int64  `json:"material_id"`
	KnowledgeEntryID int64  `json:"knowledge_entry_id"`
	ChunkIndex       int    `json:"chunk_index"`
	ChunkText        string `json:"chunk_text"`
	StartOffset      int    `json:"start_offset"`
	EndOffset        int    `json:"end_offset"`
	IndexStatus      string `json:"index_status"`
	ChunkStrategy    string `json:"chunk_strategy"`
}

type Hit struct {
	MaterialID    int64   `json:"material_id"`
	MaterialTitle string  `json:"material_title"`
	ChunkID       int64   `json:"chunk_id"`
	ChunkIndex    int     `json:"chunk_index"`
	StartOffset   int     `json:"start_offset"`
	EndOffset     int     `json:"end_offset"`
	Excerpt       string  `json:"excerpt"`
	Score         float64 `json:"score"`
	Rank          int     `json:"rank"`
	KeywordRank   int     `json:"keyword_rank,omitempty"`
	VectorRank    int     `json:"vector_rank,omitempty"`
}

type Citation struct {
	Index         int    `json:"index"`
	MaterialID    int64  `json:"material_id"`
	MaterialTitle string `json:"material_title"`
	ChunkID       int64  `json:"chunk_id"`
	ChunkIndex    int    `json:"chunk_index"`
}

type Skill struct {
	ID          int64     `json:"id"`
	ClassID     int64     `json:"class_id"`
	Name        string    `json:"name"`
	TriggerText string    `json:"trigger_text"`
	Description string    `json:"description"`
	Tools       []string  `json:"tools"`
	SourceText  string    `json:"source_text,omitempty"`
	SubjectText string    `json:"subject_text,omitempty"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type PromptTemplate struct {
	ID          int64     `json:"id"`
	ClassID     int64     `json:"class_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Content     string    `json:"content"`
	Variables   []string  `json:"variables"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type AIApp struct {
	ID                 int64     `json:"id"`
	ClassID            int64     `json:"class_id"`
	Name               string    `json:"name"`
	Description        string    `json:"description"`
	Category           string    `json:"category"`
	Icon               string    `json:"icon"`
	PromptTemplateID   *int64    `json:"prompt_template_id,omitempty"`
	PromptTemplateName string    `json:"prompt_template_name,omitempty"`
	DefaultPrompt      string    `json:"default_prompt"`
	DefaultSkillIDs    []int64   `json:"default_skill_ids"`
	DefaultMaterialIDs []int64   `json:"default_material_ids"`
	Enabled            bool      `json:"enabled"`
	AgentCount         int       `json:"agent_count"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type Agent struct {
	ID                 int64     `json:"id"`
	ClassID            int64     `json:"class_id"`
	Name               string    `json:"name"`
	Prompt             string    `json:"prompt"`
	PromptTemplateID   *int64    `json:"prompt_template_id,omitempty"`
	PromptTemplateName string    `json:"prompt_template_name,omitempty"`
	AppID              *int64    `json:"app_id,omitempty"`
	AppName            string    `json:"app_name,omitempty"`
	Enabled            bool      `json:"enabled"`
	SkillIDs           []int64   `json:"skill_ids"`
	MaterialIDs        []int64   `json:"material_ids"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type Conversation struct {
	ID        int64     `json:"id"`
	ClassID   int64     `json:"class_id"`
	UserID    int64     `json:"user_id"`
	AgentID   int64     `json:"agent_id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Message struct {
	ID             int64      `json:"id"`
	ConversationID int64      `json:"conversation_id"`
	Role           string     `json:"role"`
	Content        string     `json:"content"`
	Citations      []Citation `json:"citations,omitempty"`
	SkillName      string     `json:"skill_name,omitempty"`
	ToolCalls      []ToolCall `json:"tool_calls,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

type MCPServer struct {
	ID          int64      `json:"id"`
	ClassID     int64      `json:"class_id"`
	Name        string     `json:"name"`
	BaseURL     string     `json:"base_url"`
	HasAuth     bool       `json:"has_auth"`
	Connected   bool       `json:"connected"`
	ConnectedAt *time.Time `json:"connected_at,omitempty"`
	ToolCount   int        `json:"tool_count"`
	CreatedAt   time.Time  `json:"created_at"`
}

type MCPTool struct {
	ID          int64  `json:"id"`
	ServerID    int64  `json:"server_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	SchemaJSON  string `json:"input_schema"`
}

type ToolCall struct {
	Ref    string `json:"ref"`
	Status string `json:"status"`
	Text   string `json:"text,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type ToolCallRecord struct {
	ID             int64     `json:"id"`
	ClassID        int64     `json:"class_id"`
	ToolRef        string    `json:"tool_ref"`
	ServerID       int64     `json:"server_id"`
	ServerName     string    `json:"server_name,omitempty"`
	Status         string    `json:"status"`
	Detail         string    `json:"detail"`
	UserID         int64     `json:"user_id"`
	Username       string    `json:"username,omitempty"`
	ConversationID *int64    `json:"conversation_id,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}
