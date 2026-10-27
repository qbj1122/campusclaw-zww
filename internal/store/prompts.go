package store

import (
	"context"
	"database/sql"

	"github.com/qbj1122/campusclaw-zww/internal/model"
)

const templateSelect = `SELECT id,class_id,name,description,content,created_at,updated_at FROM prompt_templates `

func scanTemplate(sc interface{ Scan(...any) error }) (model.PromptTemplate, error) {
	var t model.PromptTemplate
	err := sc.Scan(&t.ID, &t.ClassID, &t.Name, &t.Description, &t.Content, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}

func (s *Store) ListPromptTemplates(ctx context.Context, classID int64) ([]model.PromptTemplate, error) {
	rows, err := s.DB.QueryContext(ctx, templateSelect+`WHERE class_id=? ORDER BY id`, classID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.PromptTemplate
	for rows.Next() {
		t, err := scanTemplate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) PromptTemplate(ctx context.Context, classID, id int64) (*model.PromptTemplate, error) {
	t, err := scanTemplate(s.DB.QueryRowContext(ctx, templateSelect+`WHERE class_id=? AND id=?`, classID, id))
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) CreatePromptTemplate(ctx context.Context, t *model.PromptTemplate) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO prompt_templates(class_id,name,description,content) VALUES(?,?,?,?)`,
		t.ClassID, t.Name, t.Description, t.Content)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdatePromptTemplate(ctx context.Context, t *model.PromptTemplate) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE prompt_templates SET name=?,description=?,content=? WHERE class_id=? AND id=?`,
		t.Name, t.Description, t.Content, t.ClassID, t.ID)
	return err
}

func (s *Store) DeletePromptTemplate(ctx context.Context, classID, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM prompt_templates WHERE class_id=? AND id=?`, classID, id)
	return err
}

func (s *Store) PromptTemplateExists(ctx context.Context, classID, id int64) (bool, error) {
	var one int
	err := s.DB.QueryRowContext(ctx, `SELECT 1 FROM prompt_templates WHERE class_id=? AND id=?`, classID, id).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}
