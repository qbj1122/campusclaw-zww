package httpapi

import (
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/qbj1122/campusclaw-zww/internal/model"
	"github.com/qbj1122/campusclaw-zww/internal/service"
)

const maxSkillFileBytes = 64 << 10

var errUploadHandled = errors.New("upload already handled")

func (s *Server) uploadSkill(w http.ResponseWriter, r *http.Request) {
	sk, err := s.skillFromUpload(w, r)
	if err != nil {
		return
	}
	sk.ClassID = user(r).ClassID
	id, err := s.Store.CreateSkill(r.Context(), sk)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			writeError(w, http.StatusBadRequest, "bad_request", "班内技能名称必须唯一")
			return
		}
		s.fail(w, err)
		return
	}
	sk.ID = id
	writeJSON(w, http.StatusCreated, sk)
}

func (s *Server) uploadSkillUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	old, err := s.Store.Skill(r.Context(), user(r).ClassID, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	sk, err := s.skillFromUpload(w, r)
	if err != nil {
		return
	}
	sk.ID = old.ID
	sk.ClassID = old.ClassID
	if err := s.Store.UpdateSkill(r.Context(), sk); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			writeError(w, http.StatusBadRequest, "bad_request", "班内技能名称必须唯一")
			return
		}
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sk)
}

func (s *Server) skillFromUpload(w http.ResponseWriter, r *http.Request) (*model.Skill, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxSkillFileBytes+64<<10)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "无法解析上传表单")
		return nil, errUploadHandled
	}
	f, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "缺少文件字段 file")
		return nil, errUploadHandled
	}
	defer f.Close()
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".md" && ext != ".txt" {
		writeError(w, http.StatusBadRequest, "bad_request", "技能文件只接受 .md 或 .txt")
		return nil, errUploadHandled
	}
	data, err := io.ReadAll(io.LimitReader(f, maxSkillFileBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "读取文件失败")
		return nil, errUploadHandled
	}
	if len(data) > maxSkillFileBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "too_large", "技能文件不能超过 64KB")
		return nil, errUploadHandled
	}
	if len(data) == 0 || !utf8.Valid(data) {
		writeError(w, http.StatusBadRequest, "bad_request", "技能文件必须是非空 UTF-8 文本")
		return nil, errUploadHandled
	}
	sk, err := service.ParseSkillFile(string(data))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return nil, errUploadHandled
	}
	if err := s.validateSkillTools(r.Context(), user(r).ClassID, sk.Tools); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return nil, errUploadHandled
	}
	return sk, nil
}
