package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/qbj1122/campusclaw-zww/internal/model"
	"github.com/qbj1122/campusclaw-zww/internal/service"
)

var mcpNameRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

func (s *Server) listServers(w http.ResponseWriter, r *http.Request) {
	items, err := s.Store.ListServers(r.Context(), user(r).ClassID)
	if err != nil {
		s.fail(w, err)
		return
	}
	if items == nil {
		items = []model.MCPServer{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func validMCPInput(name, base string) error {
	if !mcpNameRe.MatchString(name) {
		return fmt.Errorf("服务器名须以小写字母开头，只含小写字母、数字、下划线与连字符")
	}
	if utf8.RuneCountInString(base) > 512 {
		return fmt.Errorf("Base URL 不能超过 512 字")
	}
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("Base URL 必须是合法的 http(s) 地址")
	}
	return nil
}

func (s *Server) createServer(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name      string `json:"name"`
		BaseURL   string `json:"base_url"`
		AuthToken string `json:"auth_token"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "请求体必须是合法 JSON")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.BaseURL = strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
	if err := validMCPInput(in.Name, in.BaseURL); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	id, err := s.Store.CreateServer(r.Context(), user(r).ClassID, in.Name, in.BaseURL, in.AuthToken)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			writeError(w, http.StatusBadRequest, "bad_request", "同班服务器名称必须唯一")
			return
		}
		s.fail(w, err)
		return
	}
	srv, err := s.Store.Server(r.Context(), user(r).ClassID, id)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, srv)
}

func (s *Server) updateServer(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	if _, err := s.Store.Server(r.Context(), user(r).ClassID, id); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	var in struct {
		Name      string  `json:"name"`
		BaseURL   string  `json:"base_url"`
		AuthToken *string `json:"auth_token"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "请求体必须是合法 JSON")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.BaseURL = strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
	if err := validMCPInput(in.Name, in.BaseURL); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	var token *string
	if in.AuthToken != nil && strings.TrimSpace(*in.AuthToken) != "" {
		t := strings.TrimSpace(*in.AuthToken)
		token = &t
	}
	if err := s.Store.UpdateServer(r.Context(), user(r).ClassID, id, in.Name, in.BaseURL, token); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			writeError(w, http.StatusBadRequest, "bad_request", "同班服务器名称必须唯一")
			return
		}
		s.fail(w, err)
		return
	}
	srv, err := s.Store.Server(r.Context(), user(r).ClassID, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	writeJSON(w, http.StatusOK, srv)
}

func (s *Server) deleteServer(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	if _, err := s.Store.Server(r.Context(), user(r).ClassID, id); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	if err := s.Store.DeleteServer(r.Context(), user(r).ClassID, id); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "foreign key") {
			writeError(w, http.StatusServiceUnavailable, "unavailable", "该服务器已有调用记录，按设计保留证据，无法删除")
			return
		}
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) connectServer(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	srv, token, err := s.Store.ServerWithToken(r.Context(), user(r).ClassID, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	tools, err := s.MCP.Connect(r.Context(), srv.BaseURL, token)
	if err != nil {
		_ = s.Store.SetConnected(r.Context(), srv.ID, false)
		msg := "连接失败，请检查地址与令牌"
		if errors.Is(err, service.ErrMCPCall) {
			msg += "：" + err.Error()
		}
		s.Log.Warn("mcp connect failed", "server_id", srv.ID, "error", err.Error())
		writeError(w, http.StatusBadGateway, "bad_gateway", msg)
		return
	}
	cache := make([]model.MCPTool, 0, len(tools))
	for _, t := range tools {
		cache = append(cache, model.MCPTool{ServerID: srv.ID, Name: t.Name, Description: t.Description, SchemaJSON: t.SchemaJSON})
	}
	if err := s.Store.ReplaceTools(r.Context(), srv.ID, cache); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.Store.SetConnected(r.Context(), srv.ID, true); err != nil {
		s.fail(w, err)
		return
	}
	updated, _ := s.Store.Server(r.Context(), user(r).ClassID, srv.ID)
	stored, _ := s.Store.Tools(r.Context(), srv.ID)
	writeJSON(w, http.StatusOK, map[string]any{"server": updated, "tools": stored})
}

func (s *Server) disconnectServer(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	srv, err := s.Store.Server(r.Context(), user(r).ClassID, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	if err := s.Store.ClearTools(r.Context(), srv.ID); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.Store.SetConnected(r.Context(), srv.ID, false); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) serverTools(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	srv, err := s.Store.Server(r.Context(), user(r).ClassID, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	tools, err := s.Store.Tools(r.Context(), srv.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	if tools == nil {
		tools = []model.MCPTool{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"server": srv, "tools": tools})
}

func (s *Server) toolCalls(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 100 {
		limit = 100
	}
	items, err := s.Store.ToolCalls(r.Context(), user(r).ClassID, limit)
	if err != nil {
		s.fail(w, err)
		return
	}
	if items == nil {
		items = []model.ToolCallRecord{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
