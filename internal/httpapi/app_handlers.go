package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/qbj1122/campusclaw-zww/internal/model"
)

func (s *Server) listApps(w http.ResponseWriter, r *http.Request) {
	includeDisabled := user(r).Role == "teacher"
	items, err := s.Store.ListAIApps(r.Context(), user(r).ClassID, includeDisabled)
	if err != nil {
		s.fail(w, fmt.Errorf("apps: %w", err))
		return
	}
	if items == nil {
		items = []model.AIApp{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) getApp(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	a, err := s.Store.AIApp(r.Context(), user(r).ClassID, id)
	if err != nil || (user(r).Role != "teacher" && !a.Enabled) {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	writeJSON(w, http.StatusOK, a)
}

type appInput struct {
	Name             string  `json:"name"`
	Description      string  `json:"description"`
	Category         string  `json:"category"`
	Icon             string  `json:"icon"`
	PromptTemplateID *int64  `json:"prompt_template_id"`
	DefaultPrompt    string  `json:"default_prompt"`
	SkillIDs         []int64 `json:"skill_ids"`
	MaterialIDs      []int64 `json:"material_ids"`
	Enabled          *bool   `json:"enabled"`
}

func (s *Server) createApp(w http.ResponseWriter, r *http.Request) {
	var in appInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "请求体必须是合法 JSON")
		return
	}
	a, err := s.appFromInput(r.Context(), user(r).ClassID, in)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	id, err := s.Store.CreateAIApp(r.Context(), a)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			writeError(w, http.StatusBadRequest, "bad_request", "班内应用名称必须唯一")
			return
		}
		s.fail(w, err)
		return
	}
	a.ID = id
	if created, err := s.Store.AIApp(r.Context(), user(r).ClassID, id); err == nil {
		a = created
	}
	writeJSON(w, http.StatusCreated, a)
}

func (s *Server) updateApp(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	old, err := s.Store.AIApp(r.Context(), user(r).ClassID, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	var in appInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "请求体必须是合法 JSON")
		return
	}
	a, err := s.appFromInput(r.Context(), old.ClassID, in)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	a.ID = old.ID
	if err := s.Store.UpdateAIApp(r.Context(), a); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			writeError(w, http.StatusBadRequest, "bad_request", "班内应用名称必须唯一")
			return
		}
		s.fail(w, err)
		return
	}
	if updated, err := s.Store.AIApp(r.Context(), user(r).ClassID, a.ID); err == nil {
		a = updated
	}
	writeJSON(w, http.StatusOK, a)
}

func (s *Server) deleteApp(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	if _, err := s.Store.AIApp(r.Context(), user(r).ClassID, id); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	if err := s.Store.DeleteAIApp(r.Context(), user(r).ClassID, id); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) appFromInput(ctx context.Context, classID int64, in appInput) (*model.AIApp, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || utf8.RuneCountInString(name) > 64 {
		return nil, fmt.Errorf("应用名称不能为空且不超过 64 字")
	}
	if utf8.RuneCountInString(in.Description) > 255 {
		return nil, fmt.Errorf("应用说明不能超过 255 字")
	}
	category := strings.TrimSpace(in.Category)
	if category == "" {
		category = "通用"
	}
	if utf8.RuneCountInString(category) > 32 {
		return nil, fmt.Errorf("分类不能超过 32 字")
	}
	icon := strings.TrimSpace(in.Icon)
	if icon == "" {
		icon = "✨"
	}
	if utf8.RuneCountInString(icon) > 8 {
		return nil, fmt.Errorf("图标不能超过 8 个字符")
	}
	if utf8.RuneCountInString(in.DefaultPrompt) > 2000 {
		return nil, fmt.Errorf("默认提示词不能超过 2000 字")
	}
	templateID := in.PromptTemplateID
	if templateID != nil && *templateID <= 0 {
		templateID = nil
	}
	if templateID != nil {
		ok, err := s.Store.PromptTemplateExists(ctx, classID, *templateID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("提示词模板不存在或不属于本班")
		}
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
	skillIDs := in.SkillIDs
	if skillIDs == nil {
		skillIDs = []int64{}
	}
	materialIDs := in.MaterialIDs
	if materialIDs == nil {
		materialIDs = []int64{}
	}
	return &model.AIApp{
		ClassID: classID, Name: name, Description: strings.TrimSpace(in.Description),
		Category: category, Icon: icon, PromptTemplateID: templateID,
		DefaultPrompt: in.DefaultPrompt, DefaultSkillIDs: skillIDs,
		DefaultMaterialIDs: materialIDs, Enabled: enabled,
	}, nil
}

func (s *Server) instantiateApp(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	app, err := s.Store.AIApp(r.Context(), user(r).ClassID, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	if !app.Enabled {
		writeError(w, http.StatusBadRequest, "bad_request", "该应用已停用，无法创建助手")
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	_ = decodeJSON(r, &in)
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = app.Name + "助手"
	}
	if utf8.RuneCountInString(name) > 64 {
		writeError(w, http.StatusBadRequest, "bad_request", "助手名称不能超过 64 字")
		return
	}
	ok, err := s.Store.SkillsExist(r.Context(), user(r).ClassID, app.DefaultSkillIDs)
	if err != nil || !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "应用绑定的技能已不存在，请先修改应用")
		return
	}
	ok, err = s.Store.MaterialIDsExist(r.Context(), user(r).ClassID, app.DefaultMaterialIDs)
	if err != nil || !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "应用绑定的材料已不存在，请先修改应用")
		return
	}
	agent := &model.Agent{
		ClassID: user(r).ClassID, Name: name, Prompt: app.DefaultPrompt,
		PromptTemplateID: app.PromptTemplateID, AppID: &app.ID, Enabled: true,
		SkillIDs: app.DefaultSkillIDs, MaterialIDs: app.DefaultMaterialIDs,
	}
	agentID, err := s.Store.CreateAgent(r.Context(), agent)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			writeError(w, http.StatusBadRequest, "bad_request", "助手名称已存在，请换一个名称")
			return
		}
		s.fail(w, err)
		return
	}
	agent.ID = agentID
	if created, err := s.Store.Agent(r.Context(), user(r).ClassID, agentID); err == nil {
		agent = created
	}
	writeJSON(w, http.StatusCreated, agent)
}
