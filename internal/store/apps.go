package store

import (
	"context"
	"encoding/json"

	"github.com/qbj1122/campusclaw-zww/internal/model"
)

func encodeIDs(ids []int64) string {
	if ids == nil {
		ids = []int64{}
	}
	b, _ := json.Marshal(ids)
	return string(b)
}

func decodeIDs(s string) []int64 {
	var out []int64
	_ = json.Unmarshal([]byte(s), &out)
	if out == nil {
		out = []int64{}
	}
	return out
}

const appSelect = `SELECT a.id,a.class_id,a.name,a.description,a.category,a.icon,a.prompt_template_id,
	COALESCE(pt.name,''),a.default_prompt,a.default_skill_ids_json,a.default_material_ids_json,a.enabled,
	(SELECT COUNT(*) FROM agents ag WHERE ag.app_id=a.id) AS agent_count,a.created_at,a.updated_at
	FROM ai_apps a LEFT JOIN prompt_templates pt ON pt.id=a.prompt_template_id `

func scanApp(sc interface{ Scan(...any) error }) (model.AIApp, error) {
	var a model.AIApp
	var skills, materials string
	err := sc.Scan(&a.ID, &a.ClassID, &a.Name, &a.Description, &a.Category, &a.Icon, &a.PromptTemplateID,
		&a.PromptTemplateName, &a.DefaultPrompt, &skills, &materials, &a.Enabled, &a.AgentCount, &a.CreatedAt, &a.UpdatedAt)
	a.DefaultSkillIDs = decodeIDs(skills)
	a.DefaultMaterialIDs = decodeIDs(materials)
	return a, err
}

func (s *Store) ListAIApps(ctx context.Context, classID int64, includeDisabled bool) ([]model.AIApp, error) {
	q := appSelect + `WHERE a.class_id=?`
	if !includeDisabled {
		q += ` AND a.enabled=1`
	}
	q += ` ORDER BY a.id`
	rows, err := s.DB.QueryContext(ctx, q, classID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.AIApp
	for rows.Next() {
		a, err := scanApp(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) AIApp(ctx context.Context, classID, id int64) (*model.AIApp, error) {
	a, err := scanApp(s.DB.QueryRowContext(ctx, appSelect+`WHERE a.class_id=? AND a.id=?`, classID, id))
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Store) CreateAIApp(ctx context.Context, a *model.AIApp) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO ai_apps
		(class_id,name,description,category,icon,prompt_template_id,default_prompt,default_skill_ids_json,default_material_ids_json,enabled)
		VALUES(?,?,?,?,?,?,?,?,?,?)`,
		a.ClassID, a.Name, a.Description, a.Category, a.Icon, a.PromptTemplateID, a.DefaultPrompt,
		encodeIDs(a.DefaultSkillIDs), encodeIDs(a.DefaultMaterialIDs), a.Enabled)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateAIApp(ctx context.Context, a *model.AIApp) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE ai_apps SET name=?,description=?,category=?,icon=?,prompt_template_id=?,
		default_prompt=?,default_skill_ids_json=?,default_material_ids_json=?,enabled=? WHERE class_id=? AND id=?`,
		a.Name, a.Description, a.Category, a.Icon, a.PromptTemplateID, a.DefaultPrompt,
		encodeIDs(a.DefaultSkillIDs), encodeIDs(a.DefaultMaterialIDs), a.Enabled, a.ClassID, a.ID)
	return err
}

func (s *Store) DeleteAIApp(ctx context.Context, classID, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM ai_apps WHERE class_id=? AND id=?`, classID, id)
	return err
}
