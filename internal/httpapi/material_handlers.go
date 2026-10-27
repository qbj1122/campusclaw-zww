package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/qbj1122/campusclaw-zww/internal/chunk"
	"github.com/qbj1122/campusclaw-zww/internal/model"
	"github.com/qbj1122/campusclaw-zww/internal/store"
)

func (s *Server) listMaterials(w http.ResponseWriter, r *http.Request) {
	items, err := s.Store.ListMaterials(r.Context(), user(r).ClassID)
	if err != nil {
		s.fail(w, fmt.Errorf("list materials: %w", err))
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q != "" {
		filtered := items[:0]
		for _, m := range items {
			if strings.Contains(m.Title, q) {
				filtered = append(filtered, m)
			}
		}
		items = filtered
	}
	if items == nil {
		items = []model.Material{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) materialDetail(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	m, err := s.Store.Material(r.Context(), user(r).ClassID, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	body, _ := s.Store.MaterialBody(r.Context(), user(r).ClassID, id)
	writeJSON(w, http.StatusOK, map[string]any{
		"id": m.ID, "class_id": m.ClassID, "title": m.Title, "uploaded_by": m.UploadedBy,
		"created_at": m.CreatedAt, "body_text": body, "chunk_count": m.ChunkCount, "index_status": m.IndexStatus,
	})
}

func (s *Server) materialFile(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	m, err := s.Store.Material(r.Context(), user(r).ClassID, id)
	if err != nil || m.FilePath == "" {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	if _, err := os.Stat(m.FilePath); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	w.Header().Set("Content-Disposition", "inline; filename*=UTF-8''"+urlEscape(m.Title))
	http.ServeFile(w, r, m.FilePath)
}

func (s *Server) uploadMaterial(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, s.Cfg.MaxUploadBytes+2<<20)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "无法解析上传表单")
		return
	}
	f, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "缺少文件字段 file")
		return
	}
	defer f.Close()
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".txt" && ext != ".md" {
		writeError(w, http.StatusBadRequest, "bad_request", "只接受 .txt 或 .md 文本材料")
		return
	}
	data, err := io.ReadAll(io.LimitReader(f, s.Cfg.MaxUploadBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "读取文件失败")
		return
	}
	if int64(len(data)) > s.Cfg.MaxUploadBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "too_large", "文件超过大小限制")
		return
	}
	if len(data) == 0 || !utf8.Valid(data) {
		writeError(w, http.StatusBadRequest, "bad_request", "文件必须是非空 UTF-8 文本")
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		title = strings.TrimSpace(strings.TrimSuffix(filepath.Base(header.Filename), filepath.Ext(header.Filename)))
	}
	if title == "" || utf8.RuneCountInString(title) > 255 {
		writeError(w, http.StatusBadRequest, "bad_request", "标题不能为空且不超过 255 字")
		return
	}
	if err := os.MkdirAll(s.Cfg.UploadDir, 0o750); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "无法创建上传目录")
		return
	}
	raw := make([]byte, 16)
	_, _ = rand.Read(raw)
	stored := filepath.Join(s.Cfg.UploadDir, hex.EncodeToString(raw)+ext)
	if err := os.WriteFile(stored, data, 0o640); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "无法保存文件")
		return
	}
	u := user(r)
	materialID, err := s.Store.CreateMaterial(r.Context(), u.ClassID, u.ID, title, stored, mimeOf(ext), int64(len(data)), string(data))
	if err != nil {
		_ = os.Remove(stored)
		s.fail(w, fmt.Errorf("create material: %w", err))
		return
	}
	strategy := r.FormValue("strategy")
	opts := chunk.Options{
		Strategy:     strategy,
		MaxChars:     formInt(r, "chunk_max_chars", s.Cfg.ChunkMaxChars),
		OverlapChars: formInt(r, "chunk_overlap", s.Cfg.ChunkOverlapChars),
		StripURLs:    r.FormValue("strip_urls") == "1",
		CollapseWS:   r.FormValue("collapse_whitespace") == "1",
	}
	chunks, indexErr := s.Retrieval.IndexMaterial(r.Context(), materialID, strategy, opts)
	resp := map[string]any{"id": materialID, "title": title, "chunks": chunks}
	if indexErr != nil {
		s.Log.Warn("index after upload failed", "material_id", materialID, "error", indexErr.Error())
		resp["index_error"] = indexErr.Error()
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) materialVectors(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	if _, err := s.Store.Material(r.Context(), user(r).ClassID, id); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	chunks, err := s.Store.ChunksForMaterial(r.Context(), user(r).ClassID, id)
	if err != nil {
		s.fail(w, err)
		return
	}
	if chunks == nil {
		chunks = []store.ChunkInfo{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"material_id": id, "chunks": chunks})
}

func (s *Server) reindexMaterial(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	if _, err := s.Store.Material(r.Context(), user(r).ClassID, id); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	var in struct {
		Strategy      string `json:"strategy"`
		ChunkMaxChars int    `json:"chunk_max_chars"`
		ChunkOverlap  int    `json:"chunk_overlap"`
		StripURLs     bool   `json:"strip_urls"`
		CollapseWS    bool   `json:"collapse_whitespace"`
	}
	_ = decodeJSON(r, &in)
	entryID, _, _, err := s.Store.EntryForMaterial(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	_ = s.Store.ClearIndexState(r.Context(), entryID)
	if in.Strategy != "custom" && in.ChunkOverlap == 0 {
		in.ChunkOverlap = s.Cfg.ChunkOverlapChars
	}
	n, err := s.Retrieval.IndexMaterial(r.Context(), id, in.Strategy, chunk.Options{
		Strategy: in.Strategy, MaxChars: nonZero(in.ChunkMaxChars, s.Cfg.ChunkMaxChars),
		OverlapChars: in.ChunkOverlap, StripURLs: in.StripURLs, CollapseWS: in.CollapseWS,
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"chunks": n, "strategy": nonEmpty(in.Strategy, "auto")})
}

func formInt(r *http.Request, key string, def int) int {
	if v := r.FormValue(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func nonZero(v, def int) int {
	if v > 0 {
		return v
	}
	return def
}

func nonEmpty(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func mimeOf(ext string) string {
	if ext == ".md" {
		return "text/markdown"
	}
	return "text/plain"
}

func urlEscape(s string) string {
	return url.PathEscape(s)
}

var _ = multipart.ErrMessageTooLarge
