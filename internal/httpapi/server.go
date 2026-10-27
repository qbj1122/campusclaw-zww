package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/qbj1122/campusclaw-zww/internal/auth"
	"github.com/qbj1122/campusclaw-zww/internal/config"
	"github.com/qbj1122/campusclaw-zww/internal/model"
	"github.com/qbj1122/campusclaw-zww/internal/service"
	"github.com/qbj1122/campusclaw-zww/internal/store"
)

type Server struct {
	Cfg       *config.Config
	Store     *store.Store
	Auth      *auth.Manager
	Retrieval *service.Retrieval
	Assistant *service.Assistant
	MCP       *service.MCPClient
	Log       *slog.Logger
	Limiter   *LoginLimiter
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("POST /api/login", s.login)
	mux.HandleFunc("GET /openapi.json", s.openapi)
	mux.HandleFunc("GET /swagger", s.swaggerRedirect)
	mux.HandleFunc("GET /swagger/", s.swagger)
	mux.HandleFunc("GET /", s.index)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))

	mux.Handle("GET /api/me", s.authed(s.me))
	mux.Handle("POST /api/logout", s.authed(s.logout))

	mux.Handle("GET /api/materials", s.authed(s.listMaterials))
	mux.Handle("GET /api/materials/{id}", s.authed(s.materialDetail))
	mux.Handle("GET /api/materials/{id}/file", s.authed(s.materialFile))
	mux.Handle("GET /api/materials/{id}/vectors", s.teacher(s.materialVectors))
	mux.Handle("POST /api/materials", s.teacher(s.uploadMaterial))
	mux.Handle("POST /api/materials/{id}/reindex", s.teacher(s.reindexMaterial))

	mux.Handle("POST /api/search", s.authed(s.search))
	mux.Handle("POST /api/ask", s.authed(s.ask))

	mux.Handle("GET /api/prompt", s.authed(s.getPrompt))
	mux.Handle("PUT /api/prompt", s.teacher(s.putPrompt))
	mux.Handle("GET /api/skills", s.authed(s.listSkills))
	mux.Handle("POST /api/skills", s.teacher(s.createSkill))
	mux.Handle("POST /api/skills/upload", s.teacher(s.uploadSkill))
	mux.Handle("PUT /api/skills/{id}", s.teacher(s.updateSkill))
	mux.Handle("PUT /api/skills/{id}/upload", s.teacher(s.uploadSkillUpdate))
	mux.Handle("DELETE /api/skills/{id}", s.teacher(s.deleteSkill))
	mux.Handle("GET /api/prompt-templates", s.teacher(s.listPromptTemplates))
	mux.Handle("POST /api/prompt-templates", s.teacher(s.createPromptTemplate))
	mux.Handle("GET /api/prompt-templates/{id}", s.teacher(s.getPromptTemplate))
	mux.Handle("PUT /api/prompt-templates/{id}", s.teacher(s.updatePromptTemplate))
	mux.Handle("DELETE /api/prompt-templates/{id}", s.teacher(s.deletePromptTemplate))
	mux.Handle("POST /api/prompt-templates/{id}/preview", s.teacher(s.previewPromptTemplate))
	mux.Handle("GET /api/prompt-variables", s.teacher(s.promptTemplateVariables))

	mux.Handle("GET /api/apps", s.authed(s.listApps))
	mux.Handle("GET /api/apps/{id}", s.authed(s.getApp))
	mux.Handle("POST /api/apps", s.teacher(s.createApp))
	mux.Handle("PUT /api/apps/{id}", s.teacher(s.updateApp))
	mux.Handle("DELETE /api/apps/{id}", s.teacher(s.deleteApp))
	mux.Handle("POST /api/apps/{id}/instantiate", s.teacher(s.instantiateApp))

	mux.Handle("GET /api/agents", s.authed(s.listAgents))
	mux.Handle("GET /api/agents/{id}", s.authed(s.getAgent))
	mux.Handle("POST /api/agents", s.teacher(s.createAgent))
	mux.Handle("PUT /api/agents/{id}", s.teacher(s.updateAgent))
	mux.Handle("DELETE /api/agents/{id}", s.teacher(s.deleteAgent))

	mux.Handle("GET /api/conversations", s.authed(s.listConversations))
	mux.Handle("POST /api/conversations", s.authed(s.createConversation))
	mux.Handle("GET /api/conversations/{id}", s.authed(s.getConversation))
	mux.Handle("POST /api/conversations/{id}/messages/stream", s.authed(s.streamMessage))

	mux.Handle("GET /api/mcp/servers", s.teacher(s.listServers))
	mux.Handle("POST /api/mcp/servers", s.teacher(s.createServer))
	mux.Handle("PUT /api/mcp/servers/{id}", s.teacher(s.updateServer))
	mux.Handle("DELETE /api/mcp/servers/{id}", s.teacher(s.deleteServer))
	mux.Handle("POST /api/mcp/servers/{id}/connect", s.teacher(s.connectServer))
	mux.Handle("POST /api/mcp/servers/{id}/disconnect", s.teacher(s.disconnectServer))
	mux.Handle("GET /api/mcp/servers/{id}/tools", s.teacher(s.serverTools))
	mux.Handle("GET /api/mcp/tool-calls", s.teacher(s.toolCalls))

	return s.withLogging(mux)
}

func (s *Server) authed(h http.HandlerFunc) http.Handler { return s.Auth.Authenticate(h) }
func (s *Server) teacher(h http.HandlerFunc) http.Handler {
	return s.Auth.Authenticate(auth.RequireTeacher(h))
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ---- middleware ----

type ctxKey int

const reqIDKey ctxKey = 1

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *Server) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rid := requestID()
		w.Header().Set("X-Request-ID", rid)
		sw := &statusWriter{ResponseWriter: w}
		ctx := auth.WithAudit(context.WithValue(r.Context(), reqIDKey, rid))
		defer func() {
			if rec := recover(); rec != nil {
				s.Log.Error("panic recovered", "request_id", rid, "error", rec, "stack", string(debug.Stack()))
				if sw.status == 0 {
					writeError(sw, http.StatusInternalServerError, "internal", "服务内部错误")
				}
			}
			uid := int64(0)
			cid := int64(0)
			if a := auth.Audit(ctx); a != nil {
				uid, cid = a.UserID, a.ClassID
			}
			s.Log.Info("request",
				"request_id", rid, "method", r.Method, "path", r.URL.Path,
				"status", sw.status, "duration_ms", time.Since(start).Milliseconds(),
				"bytes", sw.bytes, "user_id", uid, "class_id", cid, "remote", clientIP(r))
		}()
		next.ServeHTTP(sw, r.WithContext(ctx))
	})
}

func requestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ---- helpers ----

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": msg}})
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrBadRequest):
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
	case errors.Is(err, service.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
	case errors.Is(err, service.ErrUnavailable):
		writeError(w, http.StatusServiceUnavailable, "unavailable", "服务暂时不可用")
	default:
		s.Log.Error("request failed", "error", err.Error())
		writeError(w, http.StatusInternalServerError, "internal", "服务内部错误")
	}
}

func decodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 2<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func pathID(r *http.Request, key string) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue(key), 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid id")
	}
	return id, nil
}

func clientIP(r *http.Request) string {
	if x := r.Header.Get("X-Forwarded-For"); x != "" {
		return strings.TrimSpace(strings.Split(x, ",")[0])
	}
	return r.RemoteAddr
}

func user(r *http.Request) *model.User { return auth.UserFrom(r.Context()) }
