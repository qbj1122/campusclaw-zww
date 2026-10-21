package chunk

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

type Chunk struct {
	Index       int
	Text        string
	StartOffset int
	EndOffset   int
}

type Options struct {
	Strategy     string // auto | custom | hierarchy
	MaxChars     int
	OverlapChars int
	StripURLs    bool
	CollapseWS   bool
}

func Normalize(opts Options) Options {
	if opts.Strategy == "" {
		opts.Strategy = "auto"
	}
	if opts.Strategy == "auto" || opts.Strategy == "hierarchy" {
		if opts.MaxChars < 100 {
			opts.MaxChars = 800
		}
		if opts.OverlapChars < 0 || opts.OverlapChars >= opts.MaxChars {
			opts.OverlapChars = opts.MaxChars / 10
		}
	}
	if opts.Strategy == "custom" {
		if opts.MaxChars < 100 {
			opts.MaxChars = 800
		}
		if opts.MaxChars > 2000 {
			opts.MaxChars = 2000
		}
		if opts.OverlapChars < 0 {
			opts.OverlapChars = 0
		}
		if opts.OverlapChars > opts.MaxChars/2 {
			opts.OverlapChars = opts.MaxChars / 2
		}
	}
	return opts
}

// Split returns chunks whose offsets refer to the text actually handed to this
// function. Preprocessing (URL stripping, whitespace collapsing) only affects
// that working copy; knowledge_entries.body_text remains untouched.
func Split(body string, opts Options) []Chunk {
	opts = Normalize(opts)
	text := body
	if opts.Strategy == "custom" {
		if opts.StripURLs {
			text = stripURLs(text)
		}
		if opts.CollapseWS {
			text = collapseWhitespace(text)
		}
	}
	switch opts.Strategy {
	case "hierarchy":
		out := splitHierarchy(text, opts)
		if len(out) > 0 {
			return out
		}
	}
	return splitWindow(text, opts.MaxChars, opts.OverlapChars, []string{"\n\n", "\n", "。", "！", "？", ". ", "! ", "? "})
}

func splitWindow(text string, max, overlap int, seps []string) []Chunk {
	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}
	var out []Chunk
	start := 0
	for start < len(runes) {
		end := start + max
		if end >= len(runes) {
			end = len(runes)
		} else {
			// Prefer a natural boundary in the last third of the window.
			low := start + max*2/3
			if low < start {
				low = start
			}
			best := -1
			for _, sep := range seps {
				sr := []rune(sep)
				for i := end; i >= low; i-- {
					if i+len(sr) <= len(runes) && string(runes[i:i+len(sr)]) == sep {
						if i > best {
							best = i + len(sr)
						}
						break
					}
				}
			}
			if best > start {
				end = best
			}
		}
		raw := string(runes[start:end])
		seg := strings.TrimSpace(raw)
		if seg != "" {
			// Trimmed text changes the true offsets slightly; shift the start
			// by the number of leading whitespace runes.
			lead := utf8.RuneCountInString(raw) - utf8.RuneCountInString(strings.TrimLeft(raw, " \t\r\n"))
			st := start + lead
			out = append(out, Chunk{Index: len(out), Text: seg, StartOffset: st, EndOffset: end})
		}
		if end >= len(runes) {
			break
		}
		next := end - overlap
		if next <= start {
			next = end
		}
		start = next
	}
	return out
}

var headingRe = regexp.MustCompile(`^(#{1,3})\s+(.*)$`)

// splitHierarchy keeps a Markdown heading with its section and only splits when
// a single section exceeds the window.
func splitHierarchy(text string, opts Options) []Chunk {
	lines := strings.SplitAfter(text, "\n")
	type section struct {
		start int // rune offset
		body  strings.Builder
	}
	var sections []section
	offset := 0
	cur := section{start: 0}
	flush := func() {
		if strings.TrimSpace(cur.body.String()) != "" {
			cur.body.WriteString("")
			sections = append(sections, cur)
		}
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if m := headingRe.FindStringSubmatch(trimmed); m != nil && len(m[1]) <= 3 {
			flush()
			cur = section{start: offset}
		}
		cur.body.WriteString(line)
		offset += utf8.RuneCountInString(line)
	}
	flush()
	if len(sections) == 0 {
		return nil
	}
	var out []Chunk
	for _, s := range sections {
		body := s.body.String()
		base := s.start
		if utf8.RuneCountInString(body) <= opts.MaxChars {
			seg := strings.TrimSpace(body)
			if seg != "" {
				out = append(out, Chunk{Index: len(out), Text: seg, StartOffset: base, EndOffset: base + utf8.RuneCountInString(body)})
			}
			continue
		}
		parts := splitWindow(body, opts.MaxChars, opts.OverlapChars, []string{"\n\n", "\n", "。", "！", "？"})
		for _, p := range parts {
			p.Index = len(out)
			p.StartOffset += base
			p.EndOffset += base
			out = append(out, p)
		}
	}
	return out
}

var urlRe = regexp.MustCompile(`https?://[^\s]+|www\.[^\s]+`)
var emailRe = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
var wsRe = regexp.MustCompile(`\s+`)

func stripURLs(s string) string {
	s = urlRe.ReplaceAllString(s, " ")
	return emailRe.ReplaceAllString(s, " ")
}

func collapseWhitespace(s string) string {
	return wsRe.ReplaceAllString(s, " ")
}
