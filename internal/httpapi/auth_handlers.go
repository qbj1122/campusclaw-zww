package httpapi

import (
	"database/sql"
	"net/http"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/qbj1122/campusclaw-zww/internal/auth"
	"github.com/qbj1122/campusclaw-zww/internal/model"
)

type loginWindow struct {
	fails []time.Time
}

type LoginLimiter struct {
	mu     sync.Mutex
	items  map[string]*loginWindow
	Limit  int
	Window time.Duration
}

func NewLoginLimiter(limit int, window time.Duration) *LoginLimiter {
	return &LoginLimiter{items: map[string]*loginWindow{}, Limit: limit, Window: window}
}

func (l *LoginLimiter) Blocked(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	w := l.items[key]
	if w == nil {
		return false
	}
	cut := time.Now().Add(-l.Window)
	kept := w.fails[:0]
	for _, t := range w.fails {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	w.fails = kept
	return len(w.fails) >= l.Limit
}

func (l *LoginLimiter) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	w := l.items[key]
	if w == nil {
		w = &loginWindow{}
		l.items[key] = w
	}
	w.fails = append(w.fails, time.Now())
}

func (l *LoginLimiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.items, key)
}

var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("not-a-real-password"), 12)

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &in); err != nil || in.Username == "" || in.Password == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "用户名或密码错误")
		return
	}
	key := in.Username + "|" + clientIP(r)
	if s.Limiter != nil && s.Limiter.Blocked(key) {
		// Deliberately indistinguishable from a wrong password.
		writeError(w, http.StatusUnauthorized, "unauthorized", "用户名或密码错误")
		return
	}
	u, hash, err := s.Store.UserByUsername(r.Context(), in.Username)
	if err != nil {
		if err != sql.ErrNoRows {
			writeError(w, http.StatusServiceUnavailable, "unavailable", "服务暂时不可用")
			return
		}
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(in.Password))
		if s.Limiter != nil {
			s.Limiter.Fail(key)
		}
		writeError(w, http.StatusUnauthorized, "unauthorized", "用户名或密码错误")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)) != nil {
		if s.Limiter != nil {
			s.Limiter.Fail(key)
		}
		writeError(w, http.StatusUnauthorized, "unauthorized", "用户名或密码错误")
		return
	}
	token, exp, err := s.Auth.Issue(u)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "无法签发令牌")
		return
	}
	if s.Limiter != nil {
		s.Limiter.Reset(key)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token":      token,
		"expires_at": exp.UTC().Format(time.RFC3339),
		"user":       u,
	})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"user": user(r)})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	raw := ""
	if h := r.Header.Get("Authorization"); len(h) > 7 {
		raw = h[7:]
	}
	if raw != "" {
		if claims, err := s.Auth.Parse(raw); err == nil {
			exp := time.Now().Add(s.Cfg.JWTTTL)
			if claims.ExpiresAt != nil {
				exp = claims.ExpiresAt.Time
			}
			_ = s.Store.RevokeToken(r.Context(), claims.ID, exp)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

var _ = model.User{}
var _ = auth.UserFrom
