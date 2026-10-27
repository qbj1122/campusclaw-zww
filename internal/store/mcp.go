package store

import (
	"context"
	"database/sql"

	"github.com/qbj1122/campusclaw-zww/internal/model"
)

const serverSelect = `SELECT s.id,s.class_id,s.name,s.base_url,(s.auth_token<>'') AS has_auth,s.connected,s.connected_at,
	(SELECT COUNT(*) FROM mcp_tools t WHERE t.server_id=s.id) AS tool_count,s.created_at
	FROM mcp_servers s `

func scanServer(sc interface{ Scan(...any) error }) (model.MCPServer, error) {
	var s model.MCPServer
	err := sc.Scan(&s.ID, &s.ClassID, &s.Name, &s.BaseURL, &s.HasAuth, &s.Connected, &s.ConnectedAt, &s.ToolCount, &s.CreatedAt)
	return s, err
}

func (s *Store) ListServers(ctx context.Context, classID int64) ([]model.MCPServer, error) {
	rows, err := s.DB.QueryContext(ctx, serverSelect+`WHERE s.class_id=? ORDER BY s.id`, classID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.MCPServer
	for rows.Next() {
		srv, err := scanServer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, srv)
	}
	return out, rows.Err()
}

func (s *Store) Server(ctx context.Context, classID, id int64) (*model.MCPServer, error) {
	srv, err := scanServer(s.DB.QueryRowContext(ctx, serverSelect+`WHERE s.class_id=? AND s.id=?`, classID, id))
	if err != nil {
		return nil, err
	}
	return &srv, nil
}

func (s *Store) ServerWithToken(ctx context.Context, classID, id int64) (*model.MCPServer, string, error) {
	srv, err := scanServer(s.DB.QueryRowContext(ctx, serverSelect+`WHERE s.class_id=? AND s.id=?`, classID, id))
	if err != nil {
		return nil, "", err
	}
	var tok string
	if err := s.DB.QueryRowContext(ctx, `SELECT auth_token FROM mcp_servers WHERE id=?`, id).Scan(&tok); err != nil {
		return nil, "", err
	}
	return &srv, tok, nil
}

func (s *Store) ServerByName(ctx context.Context, classID int64, name string) (*model.MCPServer, string, error) {
	srv, err := scanServer(s.DB.QueryRowContext(ctx, serverSelect+`WHERE s.class_id=? AND s.name=?`, classID, name))
	if err != nil {
		return nil, "", err
	}
	var tok string
	if err := s.DB.QueryRowContext(ctx, `SELECT auth_token FROM mcp_servers WHERE id=?`, srv.ID).Scan(&tok); err != nil {
		return nil, "", err
	}
	return &srv, tok, nil
}

func (s *Store) CreateServer(ctx context.Context, classID int64, name, baseURL, token string) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO mcp_servers(class_id,name,base_url,auth_token) VALUES(?,?,?,?)`, classID, name, baseURL, token)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateServer(ctx context.Context, classID, id int64, name, baseURL string, token *string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if token == nil {
		_, err = tx.ExecContext(ctx, `UPDATE mcp_servers SET name=?,base_url=?,connected=0,connected_at=NULL WHERE class_id=? AND id=?`, name, baseURL, classID, id)
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE mcp_servers SET name=?,base_url=?,auth_token=?,connected=0,connected_at=NULL WHERE class_id=? AND id=?`, name, baseURL, *token, classID, id)
	}
	if err != nil {
		return err
	}
	// A modified address or token may point at a different server, so the
	// previous discovery result is no longer trustworthy.
	if _, err := tx.ExecContext(ctx, `DELETE FROM mcp_tools WHERE server_id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DeleteServer(ctx context.Context, classID, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM mcp_servers WHERE class_id=? AND id=?`, classID, id)
	return err
}

func (s *Store) SetConnected(ctx context.Context, id int64, connected bool) error {
	if connected {
		_, err := s.DB.ExecContext(ctx, `UPDATE mcp_servers SET connected=1,connected_at=NOW() WHERE id=?`, id)
		return err
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE mcp_servers SET connected=0,connected_at=NULL WHERE id=?`, id)
	return err
}

func (s *Store) ReplaceTools(ctx context.Context, serverID int64, tools []model.MCPTool) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM mcp_tools WHERE server_id=?`, serverID); err != nil {
		return err
	}
	for _, t := range tools {
		if _, err := tx.ExecContext(ctx, `INSERT INTO mcp_tools(server_id,tool_name,description,schema_json) VALUES(?,?,?,?)`,
			serverID, t.Name, t.Description, t.SchemaJSON); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ClearTools(ctx context.Context, serverID int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM mcp_tools WHERE server_id=?`, serverID)
	return err
}

func (s *Store) Tools(ctx context.Context, serverID int64) ([]model.MCPTool, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,server_id,tool_name,description,schema_json FROM mcp_tools WHERE server_id=? ORDER BY tool_name`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.MCPTool
	for rows.Next() {
		var t model.MCPTool
		if err := rows.Scan(&t.ID, &t.ServerID, &t.Name, &t.Description, &t.SchemaJSON); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) ToolKnown(ctx context.Context, serverID int64, name string) (bool, error) {
	var one int
	err := s.DB.QueryRowContext(ctx, `SELECT 1 FROM mcp_tools WHERE server_id=? AND tool_name=?`, serverID, name).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) InsertToolCall(ctx context.Context, classID int64, toolRef string, serverID int64, status, detail string, userID int64, conversationID *int64) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO mcp_tool_calls(class_id,tool_ref,server_id,status,detail,user_id,conversation_id)
		VALUES(?,?,?,?,?,?,?)`, classID, toolRef, serverID, status, truncRunes(detail, 512), userID, conversationID)
	return err
}

func truncRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func (s *Store) ToolCalls(ctx context.Context, classID int64, limit int) ([]model.ToolCallRecord, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT c.id,c.class_id,c.tool_ref,c.server_id,s.name,c.status,c.detail,c.user_id,u.username,c.conversation_id,c.created_at
		FROM mcp_tool_calls c JOIN mcp_servers s ON s.id=c.server_id JOIN users u ON u.id=c.user_id
		WHERE c.class_id=? ORDER BY c.created_at DESC,c.id DESC LIMIT ?`, classID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ToolCallRecord
	for rows.Next() {
		var r model.ToolCallRecord
		if err := rows.Scan(&r.ID, &r.ClassID, &r.ToolRef, &r.ServerID, &r.ServerName, &r.Status, &r.Detail, &r.UserID, &r.Username, &r.ConversationID, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
