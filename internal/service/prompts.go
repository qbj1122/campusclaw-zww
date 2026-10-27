package service

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// PromptTemplateVariables are trusted request/config values. User-authored
// question text is deliberately not a template variable: it stays in the user
// message so it never gains instruction weight in the system message.
var PromptTemplateVariables = []string{
	"class_name", "agent_name", "student_name", "skill_name", "subject", "date", "materials",
}

var promptVarRe = regexp.MustCompile(`\{\{\s*([a-z_]+)\s*\}\}`)

var promptVarDesc = map[string]string{
	"class_name":   "当前班级名称",
	"agent_name":   "当前智能体名称",
	"student_name": "当前学生/教师显示名",
	"skill_name":   "本次命中的技能名称，未命中时为空",
	"subject":      "本次技能的资料范围，未命中时为空",
	"date":         "回答当天日期，格式 YYYY-MM-DD",
	"materials":    "该智能体绑定的材料标题，使用顿号连接",
}

func PromptVariableList() []map[string]string {
	out := make([]map[string]string, 0, len(PromptTemplateVariables))
	for _, name := range PromptTemplateVariables {
		out = append(out, map[string]string{"name": name, "description": promptVarDesc[name]})
	}
	return out
}

func PromptTemplateVars(content string) ([]string, error) {
	matches := promptVarRe.FindAllStringSubmatch(content, -1)
	seen := map[string]bool{}
	var out []string
	for _, m := range matches {
		name := m[1]
		if !validPromptVar(name) {
			return nil, fmt.Errorf("不支持的模板变量：{{%s}}（可用变量：%s）", name, strings.Join(PromptTemplateVariables, "、"))
		}
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	if strings.Contains(promptVarRe.ReplaceAllString(content, ""), "{{") {
		return nil, fmt.Errorf("模板中存在无法解析的变量写法，请使用 {{变量名}}")
	}
	return out, nil
}

func validPromptVar(name string) bool {
	for _, v := range PromptTemplateVariables {
		if v == name {
			return true
		}
	}
	return false
}

func ValidatePromptTemplate(name, description, content string) ([]string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 64 {
		return nil, fmt.Errorf("模板名称不能为空且不超过 64 字")
	}
	if utf8.RuneCountInString(description) > 255 {
		return nil, fmt.Errorf("模板说明不能超过 255 字")
	}
	if strings.TrimSpace(content) == "" || utf8.RuneCountInString(content) > 4000 {
		return nil, fmt.Errorf("模板内容不能为空且不超过 4000 字")
	}
	return PromptTemplateVars(content)
}

// RenderPromptTemplate replaces only known variables. User-controlled question
// text is intentionally not part of this map.
func RenderPromptTemplate(content string, values map[string]string) string {
	return promptVarRe.ReplaceAllStringFunc(content, func(m string) string {
		name := promptVarRe.FindStringSubmatch(m)[1]
		if v, ok := values[name]; ok {
			return v
		}
		return ""
	})
}

func SortVariables(vars []string) []string {
	out := append([]string(nil), vars...)
	sort.Strings(out)
	return out
}
