package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/qbj1122/campusclaw-zww/internal/chunk"
	"github.com/qbj1122/campusclaw-zww/internal/model"
)

func (s *Store) ReplaceChunks(ctx context.Context, classID, materialID, entryID int64, strategy string, chunks []chunk.Chunk) ([]int64, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM knowledge_chunks WHERE knowledge_entry_id=?`, entryID); err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(chunks))
	for _, c := range chunks {
		res, err := tx.ExecContext(ctx, `INSERT INTO knowledge_chunks
			(class_id,material_id,knowledge_entry_id,chunk_index,chunk_text,start_offset,end_offset,index_status,chunk_strategy)
			VALUES(?,?,?,?,?,?,?,'pending',?)`,
			classID, materialID, entryID, c.Index, c.Text, c.StartOffset, c.EndOffset, strategy)
		if err != nil {
			return nil, err
		}
		id, _ := res.LastInsertId()
		ids = append(ids, id)
	}
	return ids, tx.Commit()
}

func (s *Store) MarkChunks(ctx context.Context, ids []int64, status string) error {
	if len(ids) == 0 {
		return nil
	}
	ph, args := inPlaceholders(ids)
	args = append([]any{status}, args...)
	_, err := s.DB.ExecContext(ctx, `UPDATE knowledge_chunks SET index_status=? WHERE id IN (`+ph+`)`, args...)
	return err
}

func (s *Store) IndexState(ctx context.Context, entryID int64) (hash, strategy string, err error) {
	err = s.DB.QueryRowContext(ctx, `SELECT body_hash,strategy FROM knowledge_index_state WHERE knowledge_entry_id=?`, entryID).Scan(&hash, &strategy)
	return
}

func (s *Store) SetIndexState(ctx context.Context, entryID int64, hash, strategy string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO knowledge_index_state(knowledge_entry_id,body_hash,strategy) VALUES(?,?,?)
		ON DUPLICATE KEY UPDATE body_hash=VALUES(body_hash),strategy=VALUES(strategy)`, entryID, hash, strategy)
	return err
}

func (s *Store) ChunkFullTexts(ctx context.Context, classID int64, ids []int64) (map[int64]model.Hit, error) {
	out := map[int64]model.Hit{}
	uniq := unique(ids)
	if len(uniq) == 0 {
		return out, nil
	}
	ph, args := inPlaceholders(uniq)
	args = append([]any{classID}, args...)
	rows, err := s.DB.QueryContext(ctx, `SELECT kc.id,kc.material_id,m.title,kc.chunk_index,kc.start_offset,kc.end_offset,kc.chunk_text
		FROM knowledge_chunks kc JOIN materials m ON m.id=kc.material_id
		WHERE kc.class_id=? AND kc.index_status='ready' AND kc.id IN (`+ph+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var h model.Hit
		if err := rows.Scan(&h.ChunkID, &h.MaterialID, &h.MaterialTitle, &h.ChunkIndex, &h.StartOffset, &h.EndOffset, &h.Excerpt); err != nil {
			return nil, err
		}
		out[h.ChunkID] = h
	}
	return out, rows.Err()
}

func (s *Store) KeywordSearch(ctx context.Context, classID int64, q string, limit int) ([]model.Hit, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT kc.id,kc.material_id,m.title,kc.chunk_index,kc.start_offset,kc.end_offset,kc.chunk_text,
		MATCH(kc.chunk_text) AGAINST(? IN NATURAL LANGUAGE MODE) AS score
		FROM knowledge_chunks kc JOIN materials m ON m.id=kc.material_id
		WHERE kc.class_id=? AND kc.index_status='ready' AND MATCH(kc.chunk_text) AGAINST(? IN NATURAL LANGUAGE MODE)
		ORDER BY score DESC,kc.id ASC LIMIT ?`, q, classID, q, limit)
	if err == nil {
		defer rows.Close()
		return scanHits(rows)
	}
	// Portability fallback for servers without the ngram parser. It is less
	// precise but keeps the keyword path usable in local environments.
	return s.keywordSearchLike(ctx, classID, q, limit)
}

func (s *Store) keywordSearchLike(ctx context.Context, classID int64, q string, limit int) ([]model.Hit, error) {
	terms := strings.Fields(q)
	if len(terms) == 0 {
		return nil, nil
	}
	conds := make([]string, 0, len(terms))
	args := []any{classID}
	for _, t := range terms {
		conds = append(conds, "kc.chunk_text LIKE ?")
		args = append(args, "%"+t+"%")
	}
	args = append(args, limit)
	rows, err := s.DB.QueryContext(ctx, `SELECT kc.id,kc.material_id,m.title,kc.chunk_index,kc.start_offset,kc.end_offset,kc.chunk_text,0 AS score
		FROM knowledge_chunks kc JOIN materials m ON m.id=kc.material_id
		WHERE kc.class_id=? AND kc.index_status='ready' AND (`+strings.Join(conds, " OR ")+`)
		ORDER BY kc.id ASC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanHits(rows)
}

func scanHits(rows *sql.Rows) ([]model.Hit, error) {
	var out []model.Hit
	for rows.Next() {
		var h model.Hit
		if err := rows.Scan(&h.ChunkID, &h.MaterialID, &h.MaterialTitle, &h.ChunkIndex, &h.StartOffset, &h.EndOffset, &h.Excerpt, &h.Score); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (s *Store) EntriesForReindex(ctx context.Context) ([]struct {
	EntryID    int64
	ClassID    int64
	MaterialID int64
	Strategy   string
	Body       string
}, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT ke.id,ke.class_id,ke.material_id,COALESCE(kis.strategy,'auto'),ke.body_text
		FROM knowledge_entries ke LEFT JOIN knowledge_index_state kis ON kis.knowledge_entry_id=ke.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []struct {
		EntryID    int64
		ClassID    int64
		MaterialID int64
		Strategy   string
		Body       string
	}
	for rows.Next() {
		var e struct {
			EntryID    int64
			ClassID    int64
			MaterialID int64
			Strategy   string
			Body       string
		}
		if err := rows.Scan(&e.EntryID, &e.ClassID, &e.MaterialID, &e.Strategy, &e.Body); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) DeleteChunksForEntry(ctx context.Context, entryID int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM knowledge_chunks WHERE knowledge_entry_id=?`, entryID)
	if err != nil {
		return fmt.Errorf("delete chunks: %w", err)
	}
	return nil
}

func (s *Store) ClearIndexState(ctx context.Context, entryID int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM knowledge_index_state WHERE knowledge_entry_id=?`, entryID)
	return err
}

type ChunkInfo struct {
	ID          int64  `json:"chunk_id"`
	ChunkIndex  int    `json:"chunk_index"`
	IndexStatus string `json:"index_status"`
	Strategy    string `json:"strategy"`
	Chars       int    `json:"chars"`
}

func (s *Store) ChunksForMaterial(ctx context.Context, classID, materialID int64) ([]ChunkInfo, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,chunk_index,index_status,chunk_strategy,CHAR_LENGTH(chunk_text)
		FROM knowledge_chunks WHERE class_id=? AND material_id=? ORDER BY chunk_index`, classID, materialID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ChunkInfo
	for rows.Next() {
		var c ChunkInfo
		if err := rows.Scan(&c.ID, &c.ChunkIndex, &c.IndexStatus, &c.Strategy, &c.Chars); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
