package httpapi

import (
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/qbj1122/campusclaw-zww/internal/model"
	"github.com/qbj1122/campusclaw-zww/internal/service"
)

func (s *Server) listPromptTemplates(w http.ResponseWriter, r *http.Request) {
	items, err := s.Store.ListPromptTemplates(r.Context(), user(r).ClassID)
	if err != nil {
		s.fail(w, fmt.Errorf("prompt templates: %w", err))
		return
	}
	if items == nil {
		items = []model.PromptTemplate{}
	}
	for i := range items {
		if vars, err := service.PromptTemplateVars(items[i].Content); err == nil {
			items[i].Variables = vars
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) getPromptTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	t, err := s.Store.PromptTemplate(r.Context(), user(r).ClassID, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	t.Variables, _ = service.PromptTemplateVars(t.Content)
	writeJSON(w, http.StatusOK, t)
}

type promptTemplateInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"content"`
}

func (s *Server) createPromptTemplate(w http.ResponseWriter, r *http.Request) {
	var in promptTemplateInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "请求体必须是合法 JSON")
		return
	}
	vars, err := service.ValidatePromptTemplate(in.Name, in.Description, in.Content)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	t := &model.PromptTemplate{ClassID: user(r).ClassID, Name: strings.TrimSpace(in.Name), Description: strings.TrimSpace(in.Description), Content: in.Content, Variables: vars}
	id, err := s.Store.CreatePromptTemplate(r.Context(), t)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			writeError(w, http.StatusBadRequest, "bad_request", "班内模板名称必须唯一")
			return
		}
		s.fail(w, err)
		return
	}
	t.ID = id
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) updatePromptTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	old, err := s.Store.PromptTemplate(r.Context(), user(r).ClassID, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	var in promptTemplateInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "请求体必须是合法 JSON")
		return
	}
	vars, err := service.ValidatePromptTemplate(in.Name, in.Description, in.Content)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	old.Name = strings.TrimSpace(in.Name)
	old.Description = strings.TrimSpace(in.Description)
	old.Content = in.Content
	old.Variables = vars
	if err := s.Store.UpdatePromptTemplate(r.Context(), old); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			writeError(w, http.StatusBadRequest, "bad_request", "班内模板名称必须唯一")
			return
		}
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, old)
}

func (s *Server) deletePromptTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	if _, err := s.Store.PromptTemplate(r.Context(), user(r).ClassID, id); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	if err := s.Store.DeletePromptTemplate(r.Context(), user(r).ClassID, id); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) promptTemplateVariables(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": service.PromptVariableList()})
}

func (s *Server) previewPromptTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	t, err := s.Store.PromptTemplate(r.Context(), user(r).ClassID, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	u := user(r)
	rendered := service.RenderPromptTemplate(t.Content, map[string]string{
		"class_name": u.ClassName, "agent_name": "示例智能体", "student_name": u.DisplayName,
		"skill_name": "数学解题引导", "subject": "数学", "date": time.Now().Format("2006-01-02"),
		"materials": "数学自学笔记、语文自学笔记",
	})
	if utf8.RuneCountInString(rendered) > 8000 {
		rendered = string([]rune(rendered)[:8000])
	}
	writeJSON(w, http.StatusOK, map[string]any{"content": t.Content, "rendered": rendered})
}
