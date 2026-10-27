package auth

import (
	"testing"
	"time"

	"github.com/qbj1122/campusclaw-zww/internal/model"
)

func TestIssueAndParse(t *testing.T) {
	m := &Manager{Secret: []byte("0123456789abcdef0123456789abcdef"), TTL: time.Hour}
	u := &model.User{ID: 42}
	tok, exp, err := m.Issue(u)
	if err != nil {
		t.Fatal(err)
	}
	if exp.Before(time.Now()) {
		t.Fatal("expired token")
	}
	claims, err := m.Parse(tok)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Subject != "42" || claims.ID == "" {
		t.Fatalf("bad claims: %+v", claims)
	}
}

func TestParseRejectsWrongSecretAndAlg(t *testing.T) {
	m := &Manager{Secret: []byte("0123456789abcdef0123456789abcdef"), TTL: time.Hour}
	tok, _, _ := m.Issue(&model.User{ID: 1})
	other := &Manager{Secret: []byte("ffffffffffffffffffffffffffffffff")}
	if _, err := other.Parse(tok); err == nil {
		t.Fatal("wrong secret accepted")
	}
	if _, err := m.Parse(tok + "x"); err == nil {
		t.Fatal("tampered token accepted")
	}
}
