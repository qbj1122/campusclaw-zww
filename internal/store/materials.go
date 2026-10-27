package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/qbj1122/campusclaw-zww/internal/model"
)

const materialSelect = `SELECT m.id,m.class_id,m.uploaded_by,m.title,m.file_path,m.mime_type,m.size_bytes,m.created_at,
	COALESCE((SELECT COUNT(*) FROM knowledge_chunks kc WHERE kc.material_id=m.id),0) AS chunk_count,
	COALESCE((SELECT CASE WHEN SUM(kc.index_status='failed')>0 THEN 'failed' ELSE 'ready' END
		FROM knowledge_chunks kc WHERE kc.material_id=m.id),'pending') AS index_status
	FROM materials m `

func scanMaterial(sc interface{ Scan(...any) error }) (model.Material, error) {
	var m model.Material
	err := sc.Scan(&m.ID, &m.ClassID, &m.UploadedBy, &m.Title, &m.FilePath, &m.MimeType, &m.SizeBytes, &m.CreatedAt, &m.ChunkCount, &m.IndexStatus)
	return m, err
}

func (s *Store) ListMaterials(ctx context.Context, classID int64) ([]model.Material, error) {
	rows, err := s.DB.QueryContext(ctx, materialSelect+`WHERE m.class_id=? ORDER BY m.created_at DESC,m.id DESC`, classID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Material
	for rows.Next() {
		m, err := scanMaterial(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) Material(ctx context.Context, classID, id int64) (*model.Material, error) {
	row := s.DB.QueryRowContext(ctx, materialSelect+`WHERE m.class_id=? AND m.id=?`, classID, id)
	m, err := scanMaterial(row)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *Store) MaterialBody(ctx context.Context, classID, id int64) (string, error) {
	var body string
	err := s.DB.QueryRowContext(ctx, `SELECT ke.body_text FROM knowledge_entries ke
		JOIN materials m ON m.id=ke.material_id WHERE m.class_id=? AND m.id=?`, classID, id).Scan(&body)
	return body, err
}

func (s *Store) CreateMaterial(ctx context.Context, classID, uploadedBy int64, title, path, mime string, size int64, body string) (int64, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO materials(class_id,uploaded_by,title,file_path,mime_type,size_bytes) VALUES(?,?,?,?,?,?)`,
		classID, uploadedBy, title, path, mime, size)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	if _, err := tx.ExecContext(ctx, `INSERT INTO knowledge_entries(material_id,class_id,body_text) VALUES(?,?,?)`, id, classID, body); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

func (s *Store) EntryForMaterial(ctx context.Context, materialID int64) (entryID, classID int64, body string, err error) {
	err = s.DB.QueryRowContext(ctx, `SELECT id,class_id,body_text FROM knowledge_entries WHERE material_id=?`, materialID).Scan(&entryID, &classID, &body)
	return
}

func (s *Store) MaterialIDsExist(ctx context.Context, classID int64, ids []int64) (bool, error) {
	if len(ids) == 0 {
		return true, nil
	}
	ph, args := inPlaceholders(ids)
	args = append([]any{classID}, args...)
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM materials WHERE class_id=? AND id IN (`+ph+`)`, args...).Scan(&n)
	return n == len(unique(ids)), err
}

func (s *Store) AgentMaterials(ctx context.Context, agentID, classID int64) ([]model.Material, error) {
	rows, err := s.DB.QueryContext(ctx, materialSelect+`JOIN agent_materials am ON am.material_id=m.id
		WHERE am.agent_id=? AND m.class_id=? ORDER BY m.id`, agentID, classID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Material
	for rows.Next() {
		m, err := scanMaterial(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func inPlaceholders(ids []int64) (string, []any) {
	parts := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		parts[i] = "?"
		args[i] = id
	}
	return strings.Join(parts, ","), args
}

func unique(ids []int64) []int64 {
	seen := map[int64]bool{}
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func (s *Store) MustMaterial(ctx context.Context, classID, id int64) (*model.Material, error) {
	m, err := s.Material(ctx, classID, id)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("material not found")
	}
	return m, err
}
