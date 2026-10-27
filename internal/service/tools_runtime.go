package service

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/qbj1122/campusclaw-zww/internal/model"
)

const rejectedToolOutput = "（工具返回内容因安全检查被丢弃，未作为指令使用。）"

var instructionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)忽略(前述|以上|之前|前面)`),
	regexp.MustCompile(`(?i)ignore\s+(all\s+)?previous`),
	regexp.MustCompile(`(?i)你现在是`),
	regexp.MustCompile(`(?i)system\s*:`),
	regexp.MustCompile(`(?i)系统提示`),
	regexp.MustCompile(`(?i)请(调用|执行)`),
	regexp.MustCompile(`(?i)(发送|上传|转发).{0,12}(地址|邮箱|链接)`),
}

func sanitizeToolOutput(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "（工具没有返回文本内容。）", false
	}
	if utf8.RuneCountInString(s) > 1200 {
		return rejectedToolOutput, true
	}
	for _, re := range instructionPatterns {
		if re.MatchString(s) {
			return rejectedToolOutput, true
		}
	}
	return s, false
}

// runTools is the deterministic "call first, generate later" orchestration.
// The model never decides which tool runs. Each call is audited unless the
// server itself cannot be found (renamed/deleted), in which case there is no
// server_id to attach the record to.
func (a *Assistant) runTools(ctx context.Context, user *model.User, convID int64, question string, sk *model.Skill) ([]model.ToolCall, []string) {
	if sk == nil || len(sk.Tools) == 0 {
		return nil, nil
	}
	var calls []model.ToolCall
	var texts []string
	limit := 8
	if a.Cfg != nil && a.Cfg.MaxToolCalls > 0 {
		limit = a.Cfg.MaxToolCalls
	}
	invoked := 0
	for _, ref := range sk.Tools {
		if ref == "knowledge_search" {
			continue
		}
		if invoked >= limit {
			calls = append(calls, model.ToolCall{Ref: ref, Status: "denied", Detail: "超过单次提问的工具调用上限，已跳过"})
			continue
		}
		invoked++
		tool, server, err := ParseToolRef(ref)
		if err != nil {
			calls = append(calls, model.ToolCall{Ref: ref, Status: "denied", Detail: err.Error()})
			continue
		}
		srv, token, err := a.Store.ServerByName(ctx, user.ClassID, server)
		if err == sql.ErrNoRows {
			calls = append(calls, model.ToolCall{Ref: ref, Status: "denied", Detail: "服务器不存在"})
			continue
		}
		if err != nil {
			calls = append(calls, model.ToolCall{Ref: ref, Status: "error", Detail: "服务端查询失败"})
			continue
		}
		if !srv.Connected {
			calls = append(calls, model.ToolCall{Ref: ref, Status: "denied", Detail: "服务器未连接"})
			_ = a.Store.InsertToolCall(ctx, user.ClassID, ref, srv.ID, "denied", "服务器未连接", user.ID, &convID)
			continue
		}
		args := map[string]any{"query": question}
		if tool == "unit_convert" {
			args = map[string]any{"value": 1, "from": "m", "to": "cm"}
		}
		result, err := a.MCP.Call(ctx, srv.BaseURL, token, tool, args)
		if err != nil {
			detail := "调用失败"
			if err != nil {
				detail = truncateRunes(err.Error(), 200)
			}
			calls = append(calls, model.ToolCall{Ref: ref, Status: "error", Detail: detail})
			_ = a.Store.InsertToolCall(ctx, user.ClassID, ref, srv.ID, "error", detail, user.ID, &convID)
			continue
		}
		if result.IsError {
			detail := truncateRunes(result.Text, 200)
			if detail == "" {
				detail = "工具自报错误"
			}
			calls = append(calls, model.ToolCall{Ref: ref, Status: "error", Detail: detail})
			_ = a.Store.InsertToolCall(ctx, user.ClassID, ref, srv.ID, "error", detail, user.ID, &convID)
			continue
		}
		safe, rejected := sanitizeToolOutput(result.Text)
		call := model.ToolCall{Ref: ref, Status: "ok", Text: safe}
		if rejected {
			call.Detail = "结果因安全检查被整条丢弃"
		}
		calls = append(calls, call)
		texts = append(texts, fmt.Sprintf("[工具 %s 返回的数据]\n%s", ref, safe))
		_ = a.Store.InsertToolCall(ctx, user.ClassID, ref, srv.ID, "ok", truncateRunes(result.Text, 200), user.ID, &convID)
	}
	return calls, texts
}
