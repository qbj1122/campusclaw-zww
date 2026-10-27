package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/qbj1122/campusclaw-zww/internal/model"
	"github.com/qbj1122/campusclaw-zww/internal/service"
)

func (s *Server) getPrompt(w http.ResponseWriter, r *http.Request) {
	p, err := s.Store.ClassPrompt(r.Context(), user(r).ClassID)
	if err != nil {
		s.fail(w, fmt.Errorf("prompt: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"prompt": p})
}

func (s *Server) putPrompt(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Prompt string `json:"prompt"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "请求体必须是合法 JSON")
		return
	}
	if utf8.RuneCountInString(in.Prompt) > 2000 {
		writeError(w, http.StatusBadRequest, "bad_request", "提示词不能超过 2000 字")
		return
	}
	if err := s.Store.SetClassPrompt(r.Context(), user(r).ClassID, in.Prompt); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"prompt": in.Prompt})
}

func (s *Server) listSkills(w http.ResponseWriter, r *http.Request) {
	items, err := s.Store.ListSkills(r.Context(), user(r).ClassID)
	if err != nil {
		s.fail(w, fmt.Errorf("skills: %w", err))
		return
	}
	if items == nil {
		items = []model.Skill{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) createSkill(w http.ResponseWriter, r *http.Request) {
	var in struct {
		SourceText string `json:"source_text"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "请求体必须是合法 JSON")
		return
	}
	sk, err := service.ParseSkillFile(in.SourceText)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if err := s.validateSkillTools(r.Context(), user(r).ClassID, sk.Tools); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	sk.ClassID = user(r).ClassID
	id, err := s.Store.CreateSkill(r.Context(), sk)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			writeError(w, http.StatusBadRequest, "bad_request", "班内技能名称必须唯一")
			return
		}
		s.fail(w, err)
		return
	}
	sk.ID = id
	writeJSON(w, http.StatusCreated, sk)
}

func (s *Server) updateSkill(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	old, err := s.Store.Skill(r.Context(), user(r).ClassID, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	var in struct {
		SourceText string `json:"source_text"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "请求体必须是合法 JSON")
		return
	}
	sk, err := service.ParseSkillFile(in.SourceText)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if err := s.validateSkillTools(r.Context(), user(r).ClassID, sk.Tools); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	sk.ID = old.ID
	sk.ClassID = old.ClassID
	if err := s.Store.UpdateSkill(r.Context(), sk); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			writeError(w, http.StatusBadRequest, "bad_request", "班内技能名称必须唯一")
			return
		}
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sk)
}

func (s *Server) deleteSkill(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	if _, err := s.Store.Skill(r.Context(), user(r).ClassID, id); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	if err := s.Store.DeleteSkill(r.Context(), user(r).ClassID, id); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) validateSkillTools(ctx context.Context, classID int64, tools []string) error {
	for _, ref := range tools {
		if ref == "knowledge_search" {
			continue
		}
		tool, server, err := service.ParseToolRef(ref)
		if err != nil {
			return err
		}
		srv, _, err := s.Store.ServerByName(ctx, classID, server)
		if err != nil {
			return fmt.Errorf("服务器不存在或不属于本班：%s", server)
		}
		if !srv.Connected {
			return fmt.Errorf("服务器 %s 尚未连接，请先连接并发现工具", server)
		}
		known, err := s.Store.ToolKnown(ctx, srv.ID, tool)
		if err != nil {
			return fmt.Errorf("校验工具失败：%v", err)
		}
		if !known {
			return fmt.Errorf("服务器 %s 的发现清单中没有工具 %s", server, tool)
		}
	}
	return nil
}

func (s *Server) listAgents(w http.ResponseWriter, r *http.Request) {
	includeDisabled := user(r).Role == "teacher"
	items, err := s.Store.ListAgents(r.Context(), user(r).ClassID, includeDisabled)
	if err != nil {
		s.fail(w, fmt.Errorf("agents: %w", err))
		return
	}
	if items == nil {
		items = []model.Agent{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) getAgent(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	a, err := s.Store.Agent(r.Context(), user(r).ClassID, id)
	if err != nil || (user(r).Role != "teacher" && !a.Enabled) {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	writeJSON(w, http.StatusOK, a)
}

type agentInput struct {
	Name        string  `json:"name"`
	Prompt      string  `json:"prompt"`
	Enabled     *bool   `json:"enabled"`
	SkillIDs    []int64 `json:"skill_ids"`
	MaterialIDs []int64 `json:"material_ids"`
}

func (s *Server) createAgent(w http.ResponseWriter, r *http.Request) {
	var in agentInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "请求体必须是合法 JSON")
		return
	}
	a, err := s.agentFromInput(r.Context(), user(r).ClassID, in)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	id, err := s.Store.CreateAgent(r.Context(), a)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			writeError(w, http.StatusBadRequest, "bad_request", "班内智能体名称必须唯一")
			return
		}
		s.fail(w, err)
		return
	}
	a.ID = id
	writeJSON(w, http.StatusCreated, a)
}

func (s *Server) updateAgent(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	old, err := s.Store.Agent(r.Context(), user(r).ClassID, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	var in agentInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "请求体必须是合法 JSON")
		return
	}
	a, err := s.agentFromInput(r.Context(), old.ClassID, in)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	a.ID = old.ID
	if err := s.Store.UpdateAgent(r.Context(), a); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			writeError(w, http.StatusBadRequest, "bad_request", "班内智能体名称必须唯一")
			return
		}
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (s *Server) deleteAgent(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	if _, err := s.Store.Agent(r.Context(), user(r).ClassID, id); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	if err := s.Store.DeleteAgent(r.Context(), user(r).ClassID, id); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "foreign key") {
			writeError(w, http.StatusConflict, "conflict", "已有会话使用该智能体，无法删除")
			return
		}
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) agentFromInput(ctx context.Context, classID int64, in agentInput) (*model.Agent, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || utf8.RuneCountInString(name) > 64 {
		return nil, fmt.Errorf("名称不能为空且不超过 64 字")
	}
	if utf8.RuneCountInString(in.Prompt) > 2000 {
		return nil, fmt.Errorf("提示词不能超过 2000 字")
	}
	ok, err := s.Store.SkillsExist(ctx, classID, in.SkillIDs)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("绑定了他班或不存在的技能")
	}
	ok, err = s.Store.MaterialIDsExist(ctx, classID, in.MaterialIDs)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("绑定了他班或不存在的材料")
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	return &model.Agent{ClassID: classID, Name: name, Prompt: in.Prompt, Enabled: enabled, SkillIDs: in.SkillIDs, MaterialIDs: in.MaterialIDs}, nil
}
