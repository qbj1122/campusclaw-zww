package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/qbj1122/campusclaw-zww/internal/model"
)

func (s *Store) UserByUsername(ctx context.Context, username string) (*model.User, string, error) {
	var u model.User
	var hash string
	err := s.DB.QueryRowContext(ctx, `SELECT u.id,u.username,u.display_name,u.role,u.class_id,c.name,u.password_hash
		FROM users u JOIN classes c ON c.id=u.class_id WHERE u.username=?`, username).
		Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role, &u.ClassID, &u.ClassName, &hash)
	if err != nil {
		return nil, "", err
	}
	return &u, hash, nil
}

func (s *Store) UserByID(ctx context.Context, id int64) (*model.User, error) {
	var u model.User
	err := s.DB.QueryRowContext(ctx, `SELECT u.id,u.username,u.display_name,u.role,u.class_id,c.name
		FROM users u JOIN classes c ON c.id=u.class_id WHERE u.id=?`, id).
		Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role, &u.ClassID, &u.ClassName)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Store) RevokeToken(ctx context.Context, jti string, expiresAt time.Time) error {
	_, err := s.DB.ExecContext(ctx, `INSERT IGNORE INTO token_revocations(jti,expires_at) VALUES(?,?)`, jti, expiresAt)
	return err
}

func (s *Store) TokenRevoked(ctx context.Context, jti string) (bool, error) {
	var one int
	err := s.DB.QueryRowContext(ctx, `SELECT 1 FROM token_revocations WHERE jti=? AND expires_at > NOW()`, jti).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) CleanupRevocations(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM token_revocations WHERE expires_at < NOW()`)
	return err
}
