package store

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/qbj1122/campusclaw-zww/internal/model"
)

func (s *Store) CreateConversation(ctx context.Context, classID, userID, agentID int64) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO conversations(class_id,user_id,agent_id,title) VALUES(?,?,?,?)`, classID, userID, agentID, "新的提问")
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) ListConversations(ctx context.Context, classID, userID int64) ([]model.Conversation, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,class_id,user_id,agent_id,title,created_at,updated_at
		FROM conversations WHERE user_id=? AND class_id=? ORDER BY updated_at DESC,id DESC`, userID, classID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Conversation
	for rows.Next() {
		var c model.Conversation
		if err := rows.Scan(&c.ID, &c.ClassID, &c.UserID, &c.AgentID, &c.Title, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) Conversation(ctx context.Context, classID, userID, id int64) (*model.Conversation, error) {
	var c model.Conversation
	err := s.DB.QueryRowContext(ctx, `SELECT id,class_id,user_id,agent_id,title,created_at,updated_at
		FROM conversations WHERE id=? AND user_id=? AND class_id=?`, id, userID, classID).
		Scan(&c.ID, &c.ClassID, &c.UserID, &c.AgentID, &c.Title, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Store) TouchConversation(ctx context.Context, id int64, titleIfPlaceholder string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE conversations SET
		title=CASE WHEN title='新的提问' AND ?<>'' THEN ? ELSE title END,
		updated_at=NOW() WHERE id=?`, titleIfPlaceholder, titleIfPlaceholder, id)
	return err
}

func (s *Store) AddMessage(ctx context.Context, m *model.Message) (int64, error) {
	cit, _ := json.Marshal(m.Citations)
	tools, _ := json.Marshal(m.ToolCalls)
	var citVal, toolsVal any
	if len(m.Citations) > 0 {
		citVal = string(cit)
	}
	if len(m.ToolCalls) > 0 {
		toolsVal = string(tools)
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO messages(conversation_id,role,content,citations_json,skill_name,tool_calls_json)
		VALUES(?,?,?,?,?,?)`, m.ConversationID, m.Role, m.Content, citVal, m.SkillName, toolsVal)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) Messages(ctx context.Context, conversationID int64) ([]model.Message, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,conversation_id,role,content,citations_json,skill_name,tool_calls_json,created_at
		FROM messages WHERE conversation_id=? ORDER BY id`, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Message
	for rows.Next() {
		var m model.Message
		var cit, tools sql.NullString
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &cit, &m.SkillName, &tools, &m.CreatedAt); err != nil {
			return nil, err
		}
		if cit.Valid {
			_ = json.Unmarshal([]byte(cit.String), &m.Citations)
		}
		if tools.Valid {
			_ = json.Unmarshal([]byte(tools.String), &m.ToolCalls)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// RecentMessages returns the last n messages in ascending order.
func (s *Store) RecentMessages(ctx context.Context, conversationID int64, n int) ([]model.Message, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,conversation_id,role,content,citations_json,skill_name,tool_calls_json,created_at
		FROM (SELECT * FROM messages WHERE conversation_id=? ORDER BY id DESC LIMIT ?) t
		ORDER BY id`, conversationID, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Message
	for rows.Next() {
		var m model.Message
		var cit, tools sql.NullString
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &cit, &m.SkillName, &tools, &m.CreatedAt); err != nil {
			return nil, err
		}
		if cit.Valid {
			_ = json.Unmarshal([]byte(cit.String), &m.Citations)
		}
		if tools.Valid {
			_ = json.Unmarshal([]byte(tools.String), &m.ToolCalls)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
