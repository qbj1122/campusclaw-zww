package httpapi

import (
	"net/http"

	"github.com/qbj1122/campusclaw-zww/internal/llm"
	"github.com/qbj1122/campusclaw-zww/internal/model"
	"github.com/qbj1122/campusclaw-zww/internal/service"
)

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Q      string `json:"q"`
		Mode   string `json:"mode"`
		Limit  int    `json:"limit"`
		Offset int    `json:"offset"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "请求体必须是合法 JSON")
		return
	}
	res, err := s.Retrieval.Search(r.Context(), user(r).ClassID, in.Q, service.SearchOptions{Mode: in.Mode, Limit: in.Limit, Offset: in.Offset})
	if err != nil {
		s.fail(w, err)
		return
	}
	if res.Hits == nil {
		res.Hits = []model.Hit{}
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) ask(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Q       string        `json:"q"`
		History []llm.Message `json:"history"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "请求体必须是合法 JSON")
		return
	}
	status, answer, citations, err := s.Assistant.Ask(r.Context(), user(r).ClassID, in.Q, in.History)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": status, "answer": answer, "citations": citations})
}
