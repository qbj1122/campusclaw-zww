package store

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/qbj1122/campusclaw-zww/internal/model"
)

func encodeTools(tools []string) string {
	if tools == nil {
		tools = []string{}
	}
	b, _ := json.Marshal(tools)
	return string(b)
}

func decodeTools(s string) []string {
	var out []string
	_ = json.Unmarshal([]byte(s), &out)
	if out == nil {
		out = []string{}
	}
	return out
}

func scanSkill(sc interface{ Scan(...any) error }) (model.Skill, error) {
	var sk model.Skill
	var tools string
	err := sc.Scan(&sk.ID, &sk.ClassID, &sk.Name, &sk.TriggerText, &sk.Description, &tools, &sk.SourceText, &sk.SubjectText, &sk.Enabled, &sk.CreatedAt, &sk.UpdatedAt)
	sk.Tools = decodeTools(tools)
	return sk, err
}

const skillSelect = `SELECT id,class_id,name,trigger_text,description,tools_json,source_text,subject_text,enabled,created_at,updated_at FROM skills `

func (s *Store) ListSkills(ctx context.Context, classID int64) ([]model.Skill, error) {
	rows, err := s.DB.QueryContext(ctx, skillSelect+`WHERE class_id=? ORDER BY id`, classID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Skill
	for rows.Next() {
		sk, err := scanSkill(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sk)
	}
	return out, rows.Err()
}

func (s *Store) Skill(ctx context.Context, classID, id int64) (*model.Skill, error) {
	sk, err := scanSkill(s.DB.QueryRowContext(ctx, skillSelect+`WHERE class_id=? AND id=?`, classID, id))
	if err != nil {
		return nil, err
	}
	return &sk, nil
}

func (s *Store) CreateSkill(ctx context.Context, sk *model.Skill) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO skills(class_id,name,trigger_text,description,tools_json,source_text,subject_text,enabled)
		VALUES(?,?,?,?,?,?,?,?)`, sk.ClassID, sk.Name, sk.TriggerText, sk.Description, encodeTools(sk.Tools), sk.SourceText, sk.SubjectText, sk.Enabled)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateSkill(ctx context.Context, sk *model.Skill) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE skills SET name=?,trigger_text=?,description=?,tools_json=?,source_text=?,subject_text=?,enabled=? WHERE class_id=? AND id=?`,
		sk.Name, sk.TriggerText, sk.Description, encodeTools(sk.Tools), sk.SourceText, sk.SubjectText, sk.Enabled, sk.ClassID, sk.ID)
	return err
}

func (s *Store) DeleteSkill(ctx context.Context, classID, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM skills WHERE class_id=? AND id=?`, classID, id)
	return err
}

func (s *Store) SkillsExist(ctx context.Context, classID int64, ids []int64) (bool, error) {
	if len(ids) == 0 {
		return true, nil
	}
	ph, args := inPlaceholders(ids)
	args = append([]any{classID}, args...)
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM skills WHERE class_id=? AND id IN (`+ph+`)`, args...).Scan(&n)
	return n == len(unique(ids)), err
}

func (s *Store) AgentSkills(ctx context.Context, agentID, classID int64) ([]model.Skill, error) {
	rows, err := s.DB.QueryContext(ctx, skillSelect+`JOIN agent_skills ags ON ags.skill_id=skills.id
		WHERE ags.agent_id=? AND skills.class_id=? AND skills.enabled=1 ORDER BY skills.id`, agentID, classID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Skill
	for rows.Next() {
		sk, err := scanSkill(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sk)
	}
	return out, rows.Err()
}

const agentSelect = `SELECT a.id,a.class_id,a.name,a.prompt,a.prompt_template_id,COALESCE(pt.name,''),
	a.app_id,COALESCE(app.name,''),a.enabled,a.created_at,a.updated_at
	FROM agents a LEFT JOIN prompt_templates pt ON pt.id=a.prompt_template_id
	LEFT JOIN ai_apps app ON app.id=a.app_id `

func (s *Store) ListAgents(ctx context.Context, classID int64, includeDisabled bool) ([]model.Agent, error) {
	q := agentSelect + `WHERE a.class_id=?`
	if !includeDisabled {
		q += ` AND a.enabled=1`
	}
	q += ` ORDER BY a.id`
	rows, err := s.DB.QueryContext(ctx, q, classID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Agent
	for rows.Next() {
		a, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if err := s.loadAgentBindings(ctx, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func scanAgent(sc interface{ Scan(...any) error }) (model.Agent, error) {
	var a model.Agent
	err := sc.Scan(&a.ID, &a.ClassID, &a.Name, &a.Prompt, &a.PromptTemplateID, &a.PromptTemplateName,
		&a.AppID, &a.AppName, &a.Enabled, &a.CreatedAt, &a.UpdatedAt)
	return a, err
}

func (s *Store) Agent(ctx context.Context, classID, id int64) (*model.Agent, error) {
	a, err := scanAgent(s.DB.QueryRowContext(ctx, agentSelect+`WHERE a.class_id=? AND a.id=?`, classID, id))
	if err != nil {
		return nil, err
	}
	if err := s.loadAgentBindings(ctx, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Store) loadAgentBindings(ctx context.Context, a *model.Agent) error {
	if a.SkillIDs == nil {
		a.SkillIDs = []int64{}
	}
	if a.MaterialIDs == nil {
		a.MaterialIDs = []int64{}
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT skill_id FROM agent_skills WHERE agent_id=? ORDER BY skill_id`, a.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		a.SkillIDs = append(a.SkillIDs, id)
	}
	rows.Close()
	rows, err = s.DB.QueryContext(ctx, `SELECT material_id FROM agent_materials WHERE agent_id=? ORDER BY material_id`, a.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
		a.MaterialIDs = append(a.MaterialIDs, id)
	}
	return rows.Err()
}

func (s *Store) CreateAgent(ctx context.Context, a *model.Agent) (int64, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO agents(class_id,name,prompt,prompt_template_id,app_id,enabled) VALUES(?,?,?,?,?,?)`,
		a.ClassID, a.Name, a.Prompt, a.PromptTemplateID, a.AppID, a.Enabled)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	if err := replaceBindings(ctx, tx, id, a.SkillIDs, a.MaterialIDs); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

func (s *Store) UpdateAgent(ctx context.Context, a *model.Agent) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE agents SET name=?,prompt=?,prompt_template_id=?,app_id=?,enabled=? WHERE class_id=? AND id=?`,
		a.Name, a.Prompt, a.PromptTemplateID, a.AppID, a.Enabled, a.ClassID, a.ID); err != nil {
		return err
	}
	if err := replaceBindings(ctx, tx, a.ID, a.SkillIDs, a.MaterialIDs); err != nil {
		return err
	}
	return tx.Commit()
}

func replaceBindings(ctx context.Context, tx *sql.Tx, agentID int64, skillIDs, materialIDs []int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM agent_skills WHERE agent_id=?`, agentID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM agent_materials WHERE agent_id=?`, agentID); err != nil {
		return err
	}
	for _, id := range unique(skillIDs) {
		if _, err := tx.ExecContext(ctx, `INSERT INTO agent_skills(agent_id,skill_id) VALUES(?,?)`, agentID, id); err != nil {
			return err
		}
	}
	for _, id := range unique(materialIDs) {
		if _, err := tx.ExecContext(ctx, `INSERT INTO agent_materials(agent_id,material_id) VALUES(?,?)`, agentID, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) DeleteAgent(ctx context.Context, classID, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM agents WHERE class_id=? AND id=?`, classID, id)
	return err
}

func (s *Store) ClassPrompt(ctx context.Context, classID int64) (string, error) {
	var p string
	err := s.DB.QueryRowContext(ctx, `SELECT prompt FROM class_settings WHERE class_id=?`, classID).Scan(&p)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return p, err
}

func (s *Store) SetClassPrompt(ctx context.Context, classID int64, prompt string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO class_settings(class_id,prompt) VALUES(?,?)
		ON DUPLICATE KEY UPDATE prompt=VALUES(prompt)`, classID, prompt)
	return err
}
