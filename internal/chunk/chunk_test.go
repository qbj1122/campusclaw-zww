package chunk

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestAutoWindowBoundsAndOverlap(t *testing.T) {
	body := strings.Repeat("这是一个用于测试切分的中文句子。", 120)
	parts := Split(body, Options{Strategy: "auto", MaxChars: 300, OverlapChars: 30})
	if len(parts) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(parts))
	}
	for i, p := range parts {
		if utf8.RuneCountInString(p.Text) > 300 {
			t.Fatalf("chunk %d too long: %d", i, utf8.RuneCountInString(p.Text))
		}
		if p.StartOffset < 0 || p.EndOffset <= p.StartOffset {
			t.Fatalf("bad offsets on chunk %d: %+v", i, p)
		}
	}
}

func TestHierarchyKeepsHeadings(t *testing.T) {
	body := "# 第一章\n内容甲。\n\n## 第一节\n内容乙。\n\n## 第二节\n内容丙。"
	parts := Split(body, Options{Strategy: "hierarchy", MaxChars: 800, OverlapChars: 80})
	if len(parts) != 3 {
		t.Fatalf("expected 3 sections, got %d", len(parts))
	}
	if !strings.HasPrefix(parts[0].Text, "# 第一章") || !strings.Contains(parts[1].Text, "## 第一节") {
		t.Fatalf("headings not kept with section: %+v", parts)
	}
}

func TestCustomPreprocessKeepsBodyUntouched(t *testing.T) {
	body := "第一段  有  \n\n链接 https://example.com 与邮箱 a@b.com。"
	parts := Split(body, Options{Strategy: "custom", MaxChars: 2000, OverlapChars: 0, StripURLs: true, CollapseWS: true})
	if len(parts) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(parts))
	}
	if strings.Contains(parts[0].Text, "https://") || strings.Contains(parts[0].Text, "a@b.com") {
		t.Fatalf("preprocess did not remove url/email: %q", parts[0].Text)
	}
	if !strings.Contains(body, "https://example.com") {
		t.Fatal("original body was mutated")
	}
}

func TestCustomClamps(t *testing.T) {
	parts := Split("短文本", Options{Strategy: "custom", MaxChars: 10, OverlapChars: 999})
	if len(parts) != 1 || parts[0].Text != "短文本" {
		t.Fatalf("unexpected chunks: %+v", parts)
	}
}
