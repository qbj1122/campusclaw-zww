package service

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/qbj1122/campusclaw-zww/internal/model"
)

var (
	toolNameRe   = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
	serverNameRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
)

// ParseSkillFile implements the lesson-5 single-file skill format:
// metadata above the first blank line, guidance below it.
func ParseSkillFile(text string) (*model.Skill, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("技能文件不能为空")
	}
	if utf8.RuneCountInString(text) > 8000 {
		return nil, fmt.Errorf("技能文件不能超过 8000 字")
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	parts := strings.SplitN(text, "\n\n", 2)
	meta := parts[0]
	desc := ""
	if len(parts) > 1 {
		desc = strings.TrimSpace(parts[1])
	}
	if desc == "" {
		return nil, fmt.Errorf("空行以下缺少技能说明正文")
	}
	if utf8.RuneCountInString(desc) > 2000 {
		return nil, fmt.Errorf("技能说明不能超过 2000 字")
	}
	sk := &model.Skill{Description: desc, SourceText: text, Enabled: true}
	seen := map[string]bool{}
	for _, raw := range strings.Split(meta, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "：")
		if !ok {
			k, v, ok = strings.Cut(line, ":")
		}
		if !ok {
			return nil, fmt.Errorf("无法识别的元数据行：%s", line)
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		switch k {
		case "名称", "适用条件", "资料", "工具", "启用":
		default:
			return nil, fmt.Errorf("未定义字段：%s", k)
		}
		if seen[k] {
			return nil, fmt.Errorf("字段重复：%s", k)
		}
		seen[k] = true
		switch k {
		case "名称":
			sk.Name = v
		case "适用条件":
			sk.TriggerText = v
		case "资料":
			sk.SubjectText = v
		case "工具":
			for _, item := range SplitList(v) {
				item = strings.TrimSpace(item)
				if item == "" {
					continue
				}
				if item == "knowledge_search" {
					sk.Tools = append(sk.Tools, item)
					continue
				}
				if _, _, err := ParseToolRef(item); err != nil {
					return nil, err
				}
				sk.Tools = append(sk.Tools, item)
			}
		case "启用":
			sk.Enabled = v == "是" || strings.EqualFold(v, "true")
		}
	}
	if sk.Name == "" || sk.TriggerText == "" || !seen["启用"] {
		return nil, fmt.Errorf("名称、适用条件与启用为必填字段")
	}
	if utf8.RuneCountInString(sk.Name) > 32 {
		return nil, fmt.Errorf("名称不能超过 32 字")
	}
	if utf8.RuneCountInString(sk.TriggerText) > 120 {
		return nil, fmt.Errorf("适用条件不能超过 120 字")
	}
	if utf8.RuneCountInString(sk.SubjectText) > 32 {
		return nil, fmt.Errorf("资料范围不能超过 32 字")
	}
	if sk.Tools == nil {
		sk.Tools = []string{}
	}
	return sk, nil
}

func SplitList(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == '，' || r == '、' || r == ';' || r == '；' || r == '|'
	})
}

func ParseToolRef(ref string) (tool, server string, err error) {
	parts := strings.Split(ref, "@")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("工具引用格式错误：%s（应写成 工具名@服务器名）", ref)
	}
	tool, server = parts[0], parts[1]
	if !toolNameRe.MatchString(tool) {
		return "", "", fmt.Errorf("工具名格式错误：%s（小写字母开头，只含小写字母、数字与下划线）", tool)
	}
	if !serverNameRe.MatchString(server) {
		return "", "", fmt.Errorf("服务器名格式错误：%s（小写字母开头，可含小写字母、数字、下划线与连字符）", server)
	}
	return tool, server, nil
}

// RouteSkill is a deterministic lexical score over the enabled skills already
// bound to the current agent. Zero matches means no skill.
func RouteSkill(question string, skills []model.Skill) *model.Skill {
	q := strings.ToLower(question)
	bestScore := 0
	var best *model.Skill
	for i := range skills {
		score := 0
		for _, kw := range SplitList(skills[i].TriggerText) {
			kw = strings.ToLower(strings.TrimSpace(kw))
			if kw != "" && strings.Contains(q, kw) {
				score++
			}
		}
		if score > bestScore || (score == bestScore && score > 0 && best != nil && skills[i].ID < best.ID) {
			bestScore = score
			best = &skills[i]
		}
	}
	return best
}
