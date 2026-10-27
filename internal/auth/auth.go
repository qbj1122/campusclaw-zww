package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/qbj1122/campusclaw-zww/internal/model"
	"github.com/qbj1122/campusclaw-zww/internal/store"
)

type key int

const userKey key = 1
const auditKey key = 2

// AuditInfo is filled by Authenticate so the outer request logger can emit the
// authenticated user and class without parsing the token twice.
type AuditInfo struct {
	UserID  int64
	ClassID int64
}

func WithAudit(ctx context.Context) context.Context {
	return context.WithValue(ctx, auditKey, &AuditInfo{})
}

func Audit(ctx context.Context) *AuditInfo {
	v, _ := ctx.Value(auditKey).(*AuditInfo)
	return v
}

type Manager struct {
	Secret []byte
	TTL    time.Duration
	Store  *store.Store
}

type Claims struct {
	jwt.RegisteredClaims
}

func (m *Manager) Issue(u *model.User) (string, time.Time, error) {
	now := time.Now()
	exp := now.Add(m.TTL)
	jtiRaw := make([]byte, 16)
	if _, err := rand.Read(jtiRaw); err != nil {
		return "", time.Time{}, err
	}
	claims := Claims{RegisteredClaims: jwt.RegisteredClaims{
		Subject:   strconv.FormatInt(u.ID, 10),
		ID:        hex.EncodeToString(jtiRaw),
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(exp),
	}}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.Secret)
	return tok, exp, err
}

func (m *Manager) Parse(token string) (*Claims, error) {
	claims := &Claims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, errors.New("unexpected signing method")
		}
		return m.Secret, nil
	}, jwt.WithExpirationRequired(), jwt.WithIssuedAt(), jwt.WithValidMethods([]string{"HS256"}))
	if err != nil || !parsed.Valid {
		return nil, errors.New("invalid token")
	}
	if claims.Subject == "" || claims.ID == "" {
		return nil, errors.New("invalid token claims")
	}
	return claims, nil
}

func (m *Manager) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := bearer(r)
		if raw == "" {
			writeErr(w, http.StatusUnauthorized, "unauthorized", "缺少或格式不正确的 Bearer 令牌")
			return
		}
		claims, err := m.Parse(raw)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "unauthorized", "令牌无效或已过期")
			return
		}
		revoked, err := m.Store.TokenRevoked(r.Context(), claims.ID)
		if err != nil {
			writeErr(w, http.StatusServiceUnavailable, "unavailable", "服务暂时不可用")
			return
		}
		if revoked {
			writeErr(w, http.StatusUnauthorized, "unauthorized", "令牌已失效")
			return
		}
		uid, err := strconv.ParseInt(claims.Subject, 10, 64)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "unauthorized", "令牌无效")
			return
		}
		u, err := m.Store.UserByID(r.Context(), uid)
		if err == sql.ErrNoRows {
			writeErr(w, http.StatusUnauthorized, "unauthorized", "用户不存在")
			return
		}
		if err != nil {
			writeErr(w, http.StatusServiceUnavailable, "unavailable", "服务暂时不可用")
			return
		}
		ctx := context.WithValue(r.Context(), userKey, u)
		if a := Audit(ctx); a != nil {
			a.UserID, a.ClassID = u.ID, u.ClassID
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func RequireTeacher(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := UserFrom(r.Context())
		if u == nil || u.Role != "teacher" {
			writeErr(w, http.StatusForbidden, "forbidden", "仅教师可执行此操作")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func UserFrom(ctx context.Context) *model.User {
	u, _ := ctx.Value(userKey).(*model.User)
	return u
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if h == "" {
		return ""
	}
	parts := strings.SplitN(h, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.ContainsAny(parts[1], " \t") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func writeErr(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": msg}})
}
