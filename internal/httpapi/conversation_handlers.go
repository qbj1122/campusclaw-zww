package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/qbj1122/campusclaw-zww/internal/model"
	"github.com/qbj1122/campusclaw-zww/internal/service"
)

func (s *Server) listConversations(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	items, err := s.Store.ListConversations(r.Context(), u.ClassID, u.ID)
	if err != nil {
		s.fail(w, fmt.Errorf("conversations: %w", err))
		return
	}
	if items == nil {
		items = []model.Conversation{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) createConversation(w http.ResponseWriter, r *http.Request) {
	var in struct {
		AgentID int64 `json:"agent_id"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "请求体必须是合法 JSON")
		return
	}
	u := user(r)
	if in.AgentID <= 0 {
		writeError(w, http.StatusBadRequest, "bad_request", "本班暂无可用的智能体")
		return
	}
	a, err := s.Store.Agent(r.Context(), u.ClassID, in.AgentID)
	if err != nil || !a.Enabled {
		// If there is no enabled agent at all, this is a 400 configuration
		// state; otherwise the requested agent is simply not found.
		enabled, _ := s.Store.ListAgents(r.Context(), u.ClassID, false)
		if len(enabled) == 0 {
			writeError(w, http.StatusBadRequest, "bad_request", "本班暂无可用的智能体")
			return
		}
		writeError(w, http.StatusNotFound, "not_found", "智能体不存在或未启用")
		return
	}
	id, err := s.Store.CreateConversation(r.Context(), u.ClassID, u.ID, a.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	conv, err := s.Store.Conversation(r.Context(), u.ClassID, u.ID, id)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, conv)
}

func (s *Server) getConversation(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	u := user(r)
	conv, err := s.Store.Conversation(r.Context(), u.ClassID, u.ID, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	msgs, err := s.Store.Messages(r.Context(), conv.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	if msgs == nil {
		msgs = []model.Message{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversation": conv, "messages": msgs})
}

func (s *Server) streamMessage(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	var in struct {
		Content string `json:"content"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "请求体必须是合法 JSON")
		return
	}
	flusher, _ := w.(http.Flusher)
	started := false
	send := func(event string, v any) error {
		b, _ := json.Marshal(v)
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b); err != nil {
			return err
		}
		if flusher != nil {
			flusher.Flush()
		}
		return nil
	}
	metaFn := func(m service.Meta) error {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		started = true
		if m.Citations == nil {
			m.Citations = []model.Citation{}
		}
		return send("meta", m)
	}
	deltaFn := func(text string) error { return send("delta", map[string]string{"text": text}) }

	outcome, err := s.Assistant.Chat(r.Context(), user(r), id, in.Content, metaFn, deltaFn)
	if err != nil {
		if !started {
			s.fail(w, err)
			return
		}
		msg := "生成中断"
		if errors.Is(err, service.ErrUnavailable) {
			msg = "模型服务暂时不可用"
		}
		_ = send("error", map[string]string{"message": msg})
		return
	}
	_ = send("done", map[string]any{"message_id": outcome.MessageID})
}

var _ = strings.TrimSpace
