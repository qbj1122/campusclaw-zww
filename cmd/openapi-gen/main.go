// Command openapi-gen generates web/openapi.json from the annotation blocks in
// internal/httpapi/openapi_annotations.go. It is intentionally stdlib-only so
// the repository does not need a code-generation dependency.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	defaultSource  = "internal/httpapi/openapi_annotations.go"
	defaultOutput  = "web/openapi.json"
	openAPIVersion = "3.0.3"
)

type parameter struct {
	Name     string
	In       string
	Type     string
	Required bool
	Desc     string
}

type apiResponse struct {
	Code string
	Type string
	Name string
	Desc string
}

type operation struct {
	Summary      string
	Descriptions []string
	Tags         []string
	Accept       []string
	Produce      []string
	Security     []string
	Params       []parameter
	Success      []apiResponse
	Failure      []apiResponse
	Path         string
	Method       string
}

type annotation struct {
	Key   string
	Value string
}

func main() {
	src := flag.String("src", defaultSource, "annotation source file")
	out := flag.String("out", defaultOutput, "generated OpenAPI JSON file")
	check := flag.Bool("check", false, "exit non-zero when the generated document differs from the output file")
	flag.Parse()

	doc, err := generate(*src)
	if err != nil {
		fmt.Fprintf(os.Stderr, "openapi-gen: %v\n", err)
		os.Exit(1)
	}

	buf, err := encode(doc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "openapi-gen: encode: %v\n", err)
		os.Exit(1)
	}

	if *check {
		existing, err := os.ReadFile(*out)
		if err != nil {
			fmt.Fprintf(os.Stderr, "openapi-gen: %s is missing or unreadable: %v\n", *out, err)
			os.Exit(1)
		}
		if !bytes.Equal(existing, buf) {
			fmt.Fprintf(os.Stderr, "openapi-gen: %s is out of date; run go run ./cmd/openapi-gen\n", *out)
			os.Exit(1)
		}
		fmt.Printf("%s is up to date\n", *out)
		return
	}

	if err := atomicWrite(*out, buf); err != nil {
		fmt.Fprintf(os.Stderr, "openapi-gen: write %s: %v\n", *out, err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s (%d bytes)\n", *out, len(buf))
}

// generate parses the annotations and builds the OpenAPI document.
func generate(src string) (map[string]any, error) {
	data, err := os.ReadFile(src)
	if err != nil {
		return nil, fmt.Errorf("read annotations: %w", err)
	}
	ops, err := parseOperations(string(data))
	if err != nil {
		return nil, err
	}
	if len(ops) == 0 {
		return nil, errors.New("no annotated operations found")
	}

	paths := map[string]any{}
	tags := []any{}
	seenTags := map[string]bool{}
	seenOps := map[string]string{}
	opIDs := map[string]bool{}

	for _, op := range ops {
		key := strings.ToUpper(op.Method) + " " + op.Path
		if prev, ok := seenOps[key]; ok {
			return nil, fmt.Errorf("duplicate route %s (also at %s)", key, prev)
		}
		seenOps[key] = src

		pathItem, ok := paths[op.Path].(map[string]any)
		if !ok {
			pathItem = map[string]any{}
			paths[op.Path] = pathItem
		}

		built, err := buildOperation(op)
		if err != nil {
			return nil, fmt.Errorf("%s %s: %w", strings.ToUpper(op.Method), op.Path, err)
		}

		id := operationID(op.Method, op.Path)
		for opIDs[id] {
			id += "X"
		}
		opIDs[id] = true
		built["operationId"] = id

		pathItem[strings.ToLower(op.Method)] = built

		for _, tag := range op.Tags {
			if tag == "" || seenTags[tag] {
				continue
			}
			seenTags[tag] = true
			entry := map[string]any{"name": tag}
			if desc := tagDescription(tag); desc != "" {
				entry["description"] = desc
			}
			tags = append(tags, entry)
		}
	}

	doc := map[string]any{
		"openapi": openAPIVersion,
		"info": map[string]any{
			"title":       "CampusClaw API",
			"description": "CampusClaw 班级知识库、检索问答、技能与智能体平台的 HTTP API。除公开接口外均需 Authorization: Bearer <JWT>。",
			"version":     "1.0.0",
		},
		"servers": []any{
			map[string]any{"url": "/"},
		},
		"tags":  tags,
		"paths": paths,
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"bearerAuth": map[string]any{
					"type":         "http",
					"scheme":       "bearer",
					"bearerFormat": "JWT",
				},
			},
			"schemas": buildSchemas(),
		},
	}

	if err := validateRefs(doc); err != nil {
		return nil, err
	}
	return doc, nil
}

// parseOperations extracts one operation per annotation block. Only lines of
// the form "// @Key value" participate; a block is a consecutive run of such
// lines. @Router is required and ends a block.
func parseOperations(src string) ([]operation, error) {
	var (
		ops   []operation
		block []annotation
	)

	flush := func() error {
		if len(block) == 0 {
			return nil
		}
		op, ok, err := buildOperationFromAnnotations(block)
		block = nil
		if err != nil {
			return err
		}
		if ok {
			ops = append(ops, op)
		}
		return nil
	}

	for _, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "// @") {
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "// @"))
			keys := strings.SplitN(rest, " ", 2)
			value := ""
			if len(keys) > 1 {
				value = strings.TrimSpace(keys[1])
			}
			block = append(block, annotation{Key: keys[0], Value: value})
			if keys[0] == "Router" {
				if err := flush(); err != nil {
					return nil, err
				}
			}
			continue
		}
		if err := flush(); err != nil {
			return nil, err
		}
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return ops, nil
}

func buildOperationFromAnnotations(items []annotation) (operation, bool, error) {
	var op operation
	var router string
	for _, item := range items {
		switch item.Key {
		case "Summary":
			op.Summary = item.Value
		case "Description":
			if item.Value != "" {
				op.Descriptions = append(op.Descriptions, item.Value)
			}
		case "Tags":
			op.Tags = append(op.Tags, splitCSV(item.Value)...)
		case "Accept":
			op.Accept = append(op.Accept, splitCSV(item.Value)...)
		case "Produce":
			op.Produce = append(op.Produce, splitCSV(item.Value)...)
		case "Security":
			op.Security = append(op.Security, strings.Fields(item.Value)...)
		case "Param":
			p, err := parseParameter(item.Value)
			if err != nil {
				return operation{}, false, fmt.Errorf("@Param %q: %w", item.Value, err)
			}
			op.Params = append(op.Params, p)
		case "Success":
			r, err := parseResponse(item.Value)
			if err != nil {
				return operation{}, false, fmt.Errorf("@Success %q: %w", item.Value, err)
			}
			op.Success = append(op.Success, r)
		case "Failure":
			r, err := parseResponse(item.Value)
			if err != nil {
				return operation{}, false, fmt.Errorf("@Failure %q: %w", item.Value, err)
			}
			op.Failure = append(op.Failure, r)
		case "Router":
			router = item.Value
		}
	}
	if router == "" {
		return operation{}, false, nil
	}
	fields := strings.Fields(router)
	if len(fields) != 2 {
		return operation{}, false, fmt.Errorf("@Router must be \"/path [method]\", got %q", router)
	}
	op.Path = fields[0]
	if !strings.HasPrefix(op.Path, "/") {
		return operation{}, false, fmt.Errorf("@Router path %q must start with /", op.Path)
	}
	op.Method = strings.ToLower(strings.Trim(fields[1], "[]"))
	switch op.Method {
	case "get", "post", "put", "delete", "patch", "head", "options":
	default:
		return operation{}, false, fmt.Errorf("@Router method %q is not supported", fields[1])
	}
	return op, true, nil
}

func parseParameter(value string) (parameter, error) {
	tokens := tokenize(value)
	if len(tokens) < 4 {
		return parameter{}, errors.New("want name in type required [description]")
	}
	p := parameter{Name: tokens[0], In: tokens[1], Type: tokens[2]}
	required, err := strconv.ParseBool(tokens[3])
	if err != nil {
		return parameter{}, fmt.Errorf("required must be true or false: %w", err)
	}
	p.Required = required
	if len(tokens) > 4 {
		p.Desc = strings.Join(tokens[4:], " ")
	}
	switch p.In {
	case "path", "query", "header", "body":
	default:
		return parameter{}, fmt.Errorf("unsupported parameter location %q", p.In)
	}
	if p.In == "path" {
		p.Required = true
	}
	return p, nil
}

func parseResponse(value string) (apiResponse, error) {
	tokens := tokenize(value)
	if len(tokens) == 0 {
		return apiResponse{}, errors.New("missing status")
	}
	r := apiResponse{Code: tokens[0]}
	rest := tokens[1:]
	if len(rest) == 0 {
		return r, nil
	}
	if !strings.HasPrefix(rest[0], "{") {
		// No schema: the remainder is the description, e.g. @Success 302 "redirect".
		r.Desc = strings.Join(rest, " ")
		return r, nil
	}
	kind := strings.Trim(rest[0], "{}")
	switch kind {
	case "object", "array", "string", "int", "integer", "bool", "boolean", "number", "file":
		r.Type = kind
	default:
		return apiResponse{}, fmt.Errorf("unsupported type %q", rest[0])
	}
	rest = rest[1:]
	switch {
	case kind == "object" || kind == "array":
		if len(rest) == 0 {
			if kind == "array" {
				return apiResponse{}, errors.New("array response needs an item type")
			}
			r.Name = "object"
		} else {
			r.Name = rest[0]
			rest = rest[1:]
		}
	case len(rest) > 0 && isPrimitive(rest[0]):
		// Primitive responses commonly repeat the type, e.g. {string} string.
		r.Name = rest[0]
		rest = rest[1:]
	}
	if len(rest) > 0 {
		r.Desc = strings.Join(rest, " ")
	}
	return r, nil
}

func buildOperation(op operation) (map[string]any, error) {
	built := map[string]any{
		"summary":   op.Summary,
		"responses": map[string]any{},
	}
	if len(op.Descriptions) > 0 {
		built["description"] = strings.Join(op.Descriptions, "\n")
	}
	if len(op.Tags) > 0 {
		built["tags"] = op.Tags
	}
	if len(op.Security) > 0 {
		security := make([]any, 0, len(op.Security))
		for _, name := range op.Security {
			scheme := "bearerAuth"
			if !strings.EqualFold(name, "Bearer") && !strings.EqualFold(name, "bearerAuth") {
				scheme = name
			}
			security = append(security, map[string]any{scheme: []any{}})
		}
		built["security"] = security
	}

	var params []any
	var body *parameter
	for i := range op.Params {
		p := &op.Params[i]
		if p.In == "body" {
			if body != nil {
				return nil, errors.New("only one body parameter is supported")
			}
			body = p
			continue
		}
		entry := map[string]any{
			"name":     p.Name,
			"in":       p.In,
			"required": p.Required,
			"schema":   schemaForType(p.Type),
		}
		if p.Desc != "" {
			entry["description"] = p.Desc
		}
		params = append(params, entry)
	}
	if len(params) > 0 {
		built["parameters"] = params
	}
	if body != nil {
		media := firstMediaType(op.Accept, "application/json")
		built["requestBody"] = map[string]any{
			"required": body.Required,
			"content": map[string]any{
				media: map[string]any{"schema": schemaForType(body.Type)},
			},
		}
		if body.Desc != "" {
			built["requestBody"].(map[string]any)["description"] = body.Desc
		}
	}

	responses := built["responses"].(map[string]any)
	add := func(list []apiResponse, success bool) error {
		for _, r := range list {
			if _, exists := responses[r.Code]; exists {
				return fmt.Errorf("duplicate response status %s", r.Code)
			}
			entry := map[string]any{"description": responseDescription(r)}
			if r.Type != "" && r.Code != "204" {
				schema := responseSchema(r)
				media := map[string]any{}
				contentTypes := []string{"application/json"}
				if success {
					contentTypes = mediaTypes(op.Produce)
				}
				for _, ct := range contentTypes {
					media[ct] = map[string]any{"schema": schema}
				}
				entry["content"] = media
			}
			responses[r.Code] = entry
		}
		return nil
	}
	if err := add(op.Success, true); err != nil {
		return nil, err
	}
	if err := add(op.Failure, false); err != nil {
		return nil, err
	}
	if len(responses) == 0 {
		return nil, errors.New("operation has no responses")
	}
	return built, nil
}

func responseSchema(r apiResponse) map[string]any {
	switch r.Type {
	case "array":
		return arraySchema(schemaForType(r.Name))
	case "object":
		if r.Name == "" || r.Name == "object" {
			return map[string]any{"type": "object"}
		}
		return schemaForType(r.Name)
	case "file":
		return map[string]any{"type": "string", "format": "binary"}
	default:
		if r.Name != "" && !isPrimitive(r.Name) {
			return schemaForType(r.Name)
		}
		return schemaForType(r.Type)
	}
}

func responseDescription(r apiResponse) string {
	if r.Desc != "" {
		return r.Desc
	}
	if text, ok := httpStatusText[r.Code]; ok {
		return text
	}
	if n, err := strconv.Atoi(r.Code); err == nil {
		return http.StatusText(n)
	}
	return "响应"
}

func schemaForType(name string) map[string]any {
	name = strings.TrimSpace(name)
	switch name {
	case "", "object":
		return map[string]any{"type": "object"}
	case "string":
		return map[string]any{"type": "string"}
	case "int", "integer", "int64":
		return map[string]any{"type": "integer", "format": "int64"}
	case "bool", "boolean":
		return map[string]any{"type": "boolean"}
	case "number", "float", "double":
		return map[string]any{"type": "number", "format": "double"}
	case "file":
		return map[string]any{"type": "string", "format": "binary"}
	}
	return refSchema(name)
}

func isPrimitive(name string) bool {
	switch name {
	case "object", "string", "int", "integer", "int64", "bool", "boolean", "number", "float", "double", "file":
		return true
	}
	return false
}

func mediaTypes(produce []string) []string {
	if len(produce) == 0 {
		return []string{"application/json"}
	}
	out := make([]string, 0, len(produce))
	seen := map[string]bool{}
	for _, item := range produce {
		ct := normalizeMediaType(item)
		if !seen[ct] {
			seen[ct] = true
			out = append(out, ct)
		}
	}
	return out
}

func firstMediaType(accept []string, fallback string) string {
	if len(accept) == 0 {
		return fallback
	}
	return normalizeMediaType(accept[0])
}

func normalizeMediaType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "json":
		return "application/json"
	case "xml":
		return "application/xml"
	case "yaml":
		return "application/yaml"
	case "plain", "text":
		return "text/plain"
	case "html":
		return "text/html"
	case "event-stream", "sse":
		return "text/event-stream"
	default:
		return strings.TrimSpace(value)
	}
}

func operationID(method, path string) string {
	var b strings.Builder
	b.WriteString(strings.ToLower(method))
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		b.WriteString("Root")
		return b.String()
	}
	replacer := strings.NewReplacer("{", "", "}", "", "...", "", "-", "", "_", "")
	for _, part := range strings.Split(trimmed, "/") {
		part = replacer.Replace(part)
		if part == "" {
			continue
		}
		b.WriteString(strings.ToUpper(part[:1]))
		b.WriteString(part[1:])
	}
	return b.String()
}

func tagDescription(tag string) string {
	switch tag {
	case "auth":
		return "登录、退出与当前用户"
	case "materials":
		return "班级材料上传、详情与索引"
	case "search":
		return "检索与基于材料的问答"
	case "prompt":
		return "班级默认提示词"
	case "skills":
		return "技能定义与工具绑定"
	case "agents":
		return "智能体配置"
	case "conversations":
		return "会话与流式回答"
	case "mcp":
		return "MCP 服务器与工具调用审计（仅教师）"
	case "docs":
		return "OpenAPI 与 Swagger UI"
	case "system":
		return "健康检查与静态资源"
	}
	return ""
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// tokenize splits on whitespace while keeping quoted descriptions together and
// stripping the surrounding quotes.
func tokenize(value string) []string {
	var (
		out     []string
		current strings.Builder
		quoted  bool
	)
	flush := func() {
		if current.Len() > 0 {
			out = append(out, current.String())
			current.Reset()
		}
	}
	for _, r := range value {
		switch {
		case r == '"':
			quoted = !quoted
		case !quoted && (r == ' ' || r == '\t'):
			flush()
		default:
			current.WriteRune(r)
		}
	}
	flush()
	return out
}

func refSchema(name string) map[string]any {
	return map[string]any{"$ref": "#/components/schemas/" + name}
}

func arraySchema(items map[string]any) map[string]any {
	return map[string]any{"type": "array", "items": items}
}

func objectSchema(properties map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": properties}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func stringSchema() map[string]any { return map[string]any{"type": "string"} }
func intSchema() map[string]any    { return map[string]any{"type": "integer", "format": "int64"} }
func boolSchema() map[string]any   { return map[string]any{"type": "boolean"} }
func numberSchema() map[string]any { return map[string]any{"type": "number", "format": "double"} }
func dateTimeSchema() map[string]any {
	return map[string]any{"type": "string", "format": "date-time"}
}
func enumSchema(values ...string) map[string]any {
	return map[string]any{"type": "string", "enum": values}
}

func buildSchemas() map[string]any {
	return map[string]any{
		"ErrorResponse": objectSchema(map[string]any{
			"error": objectSchema(map[string]any{
				"code": enumSchema(
					"bad_request", "unauthorized", "forbidden", "not_found", "conflict",
					"too_large", "unavailable", "internal", "bad_gateway",
				),
				"message": stringSchema(),
			}, "code", "message"),
		}, "error"),

		"Health": objectSchema(map[string]any{
			"status": enumSchema("ok"),
		}, "status"),

		"User": objectSchema(map[string]any{
			"id":           intSchema(),
			"username":     stringSchema(),
			"display_name": stringSchema(),
			"role":         enumSchema("teacher", "student"),
			"class_id":     intSchema(),
			"class_name":   stringSchema(),
		}, "id", "username", "role", "class_id"),

		"LoginRequest": objectSchema(map[string]any{
			"username": stringSchema(),
			"password": map[string]any{"type": "string", "format": "password"},
		}, "username", "password"),

		"LoginResponse": objectSchema(map[string]any{
			"token":      stringSchema(),
			"expires_at": dateTimeSchema(),
			"user":       refSchema("User"),
		}, "token", "expires_at", "user"),

		"MeResponse": objectSchema(map[string]any{
			"user": refSchema("User"),
		}, "user"),

		"Material": objectSchema(map[string]any{
			"id":           intSchema(),
			"class_id":     intSchema(),
			"uploaded_by":  intSchema(),
			"title":        stringSchema(),
			"mime_type":    stringSchema(),
			"size_bytes":   map[string]any{"type": "integer", "format": "int64"},
			"created_at":   dateTimeSchema(),
			"chunk_count":  map[string]any{"type": "integer"},
			"index_status": stringSchema(),
		}, "id", "class_id", "uploaded_by", "title", "size_bytes", "created_at"),

		"MaterialList": objectSchema(map[string]any{
			"items": arraySchema(refSchema("Material")),
		}, "items"),

		"MaterialDetail": objectSchema(map[string]any{
			"id":           intSchema(),
			"class_id":     intSchema(),
			"title":        stringSchema(),
			"uploaded_by":  intSchema(),
			"created_at":   dateTimeSchema(),
			"body_text":    stringSchema(),
			"chunk_count":  map[string]any{"type": "integer"},
			"index_status": stringSchema(),
		}, "id", "class_id", "title", "uploaded_by", "created_at", "body_text", "chunk_count", "index_status"),

		"ChunkInfo": objectSchema(map[string]any{
			"chunk_id":     intSchema(),
			"chunk_index":  map[string]any{"type": "integer"},
			"index_status": stringSchema(),
			"strategy":     stringSchema(),
			"chars":        map[string]any{"type": "integer"},
		}, "chunk_id", "chunk_index", "index_status", "strategy", "chars"),

		"MaterialVectorsResponse": objectSchema(map[string]any{
			"material_id": intSchema(),
			"chunks":      arraySchema(refSchema("ChunkInfo")),
		}, "material_id", "chunks"),

		"UploadRequest": objectSchema(map[string]any{
			"file":                map[string]any{"type": "string", "format": "binary"},
			"title":               stringSchema(),
			"strategy":            enumSchema("auto", "custom", "hierarchy"),
			"chunk_max_chars":     map[string]any{"type": "integer"},
			"chunk_overlap":       map[string]any{"type": "integer"},
			"strip_urls":          enumSchema("0", "1"),
			"collapse_whitespace": enumSchema("0", "1"),
		}, "file"),

		"UploadResponse": objectSchema(map[string]any{
			"id":          intSchema(),
			"title":       stringSchema(),
			"chunks":      map[string]any{"type": "integer"},
			"index_error": stringSchema(),
		}, "id", "title", "chunks"),

		"ReindexRequest": objectSchema(map[string]any{
			"strategy":            enumSchema("auto", "custom", "hierarchy"),
			"chunk_max_chars":     map[string]any{"type": "integer"},
			"chunk_overlap":       map[string]any{"type": "integer"},
			"strip_urls":          boolSchema(),
			"collapse_whitespace": boolSchema(),
		}),

		"ReindexResponse": objectSchema(map[string]any{
			"chunks":   map[string]any{"type": "integer"},
			"strategy": stringSchema(),
		}, "chunks", "strategy"),

		"SearchRequest": objectSchema(map[string]any{
			"q":      stringSchema(),
			"mode":   enumSchema("keyword", "vector", "hybrid"),
			"limit":  map[string]any{"type": "integer"},
			"offset": map[string]any{"type": "integer"},
		}, "q"),

		"Hit": objectSchema(map[string]any{
			"material_id":    intSchema(),
			"material_title": stringSchema(),
			"chunk_id":       intSchema(),
			"chunk_index":    map[string]any{"type": "integer"},
			"start_offset":   map[string]any{"type": "integer"},
			"end_offset":     map[string]any{"type": "integer"},
			"excerpt":        stringSchema(),
			"score":          numberSchema(),
			"rank":           map[string]any{"type": "integer"},
			"keyword_rank":   map[string]any{"type": "integer"},
			"vector_rank":    map[string]any{"type": "integer"},
		}, "material_id", "material_title", "chunk_id", "chunk_index", "start_offset", "end_offset", "excerpt", "score", "rank"),

		"SearchResponse": objectSchema(map[string]any{
			"status": enumSchema("ok", "no_evidence"),
			"mode":   stringSchema(),
			"hits":   arraySchema(refSchema("Hit")),
		}, "status", "mode", "hits"),

		"Citation": objectSchema(map[string]any{
			"index":          map[string]any{"type": "integer"},
			"material_id":    intSchema(),
			"material_title": stringSchema(),
			"chunk_id":       intSchema(),
			"chunk_index":    map[string]any{"type": "integer"},
		}, "index", "material_id", "material_title", "chunk_id", "chunk_index"),

		"ChatMessage": objectSchema(map[string]any{
			"role":    enumSchema("user", "assistant", "system"),
			"content": stringSchema(),
		}, "role", "content"),

		"AskRequest": objectSchema(map[string]any{
			"q":       stringSchema(),
			"history": arraySchema(refSchema("ChatMessage")),
		}, "q"),

		"AskResponse": objectSchema(map[string]any{
			"status":    enumSchema("ok", "no_evidence"),
			"answer":    stringSchema(),
			"citations": arraySchema(refSchema("Citation")),
		}, "status", "answer", "citations"),

		"Prompt": objectSchema(map[string]any{
			"prompt": stringSchema(),
		}, "prompt"),

		"Skill": objectSchema(map[string]any{
			"id":           intSchema(),
			"class_id":     intSchema(),
			"name":         stringSchema(),
			"trigger_text": stringSchema(),
			"description":  stringSchema(),
			"tools":        arraySchema(stringSchema()),
			"source_text":  stringSchema(),
			"subject_text": stringSchema(),
			"enabled":      boolSchema(),
			"created_at":   dateTimeSchema(),
			"updated_at":   dateTimeSchema(),
		}, "id", "class_id", "name", "trigger_text", "tools", "enabled"),

		"SkillList": objectSchema(map[string]any{
			"items": arraySchema(refSchema("Skill")),
		}, "items"),

		"SkillInput": objectSchema(map[string]any{
			"source_text": stringSchema(),
		}, "source_text"),

		"Agent": objectSchema(map[string]any{
			"id":           intSchema(),
			"class_id":     intSchema(),
			"name":         stringSchema(),
			"prompt":       stringSchema(),
			"enabled":      boolSchema(),
			"skill_ids":    arraySchema(intSchema()),
			"material_ids": arraySchema(intSchema()),
			"created_at":   dateTimeSchema(),
			"updated_at":   dateTimeSchema(),
		}, "id", "class_id", "name", "prompt", "enabled", "skill_ids", "material_ids"),

		"AgentList": objectSchema(map[string]any{
			"items": arraySchema(refSchema("Agent")),
		}, "items"),

		"AgentInput": objectSchema(map[string]any{
			"name":         stringSchema(),
			"prompt":       stringSchema(),
			"enabled":      boolSchema(),
			"skill_ids":    arraySchema(intSchema()),
			"material_ids": arraySchema(intSchema()),
		}, "name"),

		"Conversation": objectSchema(map[string]any{
			"id":         intSchema(),
			"class_id":   intSchema(),
			"user_id":    intSchema(),
			"agent_id":   intSchema(),
			"title":      stringSchema(),
			"created_at": dateTimeSchema(),
			"updated_at": dateTimeSchema(),
		}, "id", "class_id", "user_id", "agent_id", "created_at", "updated_at"),

		"ConversationList": objectSchema(map[string]any{
			"items": arraySchema(refSchema("Conversation")),
		}, "items"),

		"ConversationCreateRequest": objectSchema(map[string]any{
			"agent_id": intSchema(),
		}, "agent_id"),

		"Message": objectSchema(map[string]any{
			"id":              intSchema(),
			"conversation_id": intSchema(),
			"role":            enumSchema("user", "assistant", "system"),
			"content":         stringSchema(),
			"citations":       arraySchema(refSchema("Citation")),
			"skill_name":      stringSchema(),
			"tool_calls":      arraySchema(refSchema("ToolCall")),
			"created_at":      dateTimeSchema(),
		}, "id", "conversation_id", "role", "content", "created_at"),

		"ConversationDetail": objectSchema(map[string]any{
			"conversation": refSchema("Conversation"),
			"messages":     arraySchema(refSchema("Message")),
		}, "conversation", "messages"),

		"StreamRequest": objectSchema(map[string]any{
			"content": stringSchema(),
		}, "content"),

		"MCPServer": objectSchema(map[string]any{
			"id":           intSchema(),
			"class_id":     intSchema(),
			"name":         stringSchema(),
			"base_url":     map[string]any{"type": "string", "format": "uri"},
			"has_auth":     boolSchema(),
			"connected":    boolSchema(),
			"connected_at": map[string]any{"type": "string", "format": "date-time", "nullable": true},
			"tool_count":   map[string]any{"type": "integer"},
			"created_at":   dateTimeSchema(),
		}, "id", "class_id", "name", "base_url", "has_auth", "connected", "tool_count", "created_at"),

		"MCPServerList": objectSchema(map[string]any{
			"items": arraySchema(refSchema("MCPServer")),
		}, "items"),

		"MCPServerInput": objectSchema(map[string]any{
			"name":       map[string]any{"type": "string", "pattern": "^[a-z][a-z0-9_-]{0,63}$"},
			"base_url":   map[string]any{"type": "string", "format": "uri"},
			"auth_token": map[string]any{"type": "string", "writeOnly": true},
		}, "name", "base_url"),

		"MCPTool": objectSchema(map[string]any{
			"id":           intSchema(),
			"server_id":    intSchema(),
			"name":         stringSchema(),
			"description":  stringSchema(),
			"input_schema": map[string]any{"type": "string", "description": "JSON Schema 字符串"},
		}, "id", "server_id", "name", "input_schema"),

		"MCPToolsResponse": objectSchema(map[string]any{
			"server": refSchema("MCPServer"),
			"tools":  arraySchema(refSchema("MCPTool")),
		}, "server", "tools"),

		"ToolCall": objectSchema(map[string]any{
			"ref":    stringSchema(),
			"status": stringSchema(),
			"text":   stringSchema(),
			"detail": stringSchema(),
		}, "ref", "status"),

		"ToolCallRecord": objectSchema(map[string]any{
			"tool_ref":        stringSchema(),
			"server_name":     stringSchema(),
			"status":          stringSchema(),
			"detail":          stringSchema(),
			"username":        stringSchema(),
			"conversation_id": map[string]any{"type": "integer", "format": "int64", "nullable": true},
			"created_at":      dateTimeSchema(),
		}, "tool_ref", "status", "detail", "created_at"),

		"ToolCallList": objectSchema(map[string]any{
			"items": arraySchema(refSchema("ToolCallRecord")),
		}, "items"),

		"OpenAPIDocument": map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		},
	}
}

var httpStatusText = map[string]string{
	"200": "成功",
	"201": "已创建",
	"204": "无内容",
	"302": "重定向",
	"400": "请求参数错误",
	"401": "未认证",
	"403": "无权限",
	"404": "资源不存在",
	"409": "冲突",
	"413": "请求体过大",
	"500": "服务内部错误",
	"502": "上游服务错误",
	"503": "服务暂时不可用",
}

// validateRefs makes sure every local $ref points at a declared schema. This
// catches typos in annotations before a broken document is published.
func validateRefs(doc map[string]any) error {
	schemas, _ := doc["components"].(map[string]any)["schemas"].(map[string]any)
	var walk func(any) error
	walk = func(node any) error {
		switch v := node.(type) {
		case map[string]any:
			if ref, ok := v["$ref"].(string); ok {
				const prefix = "#/components/schemas/"
				if !strings.HasPrefix(ref, prefix) {
					return fmt.Errorf("unsupported $ref %q", ref)
				}
				name := strings.TrimPrefix(ref, prefix)
				if _, ok := schemas[name]; !ok {
					return fmt.Errorf("$ref %q has no schema", ref)
				}
			}
			for _, child := range v {
				if err := walk(child); err != nil {
					return err
				}
			}
		case []any:
			for _, child := range v {
				if err := walk(child); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(doc); err != nil {
		return err
	}
	return nil
}

func encode(doc map[string]any) ([]byte, error) {
	buf, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(buf, '\n'), nil
}

func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".openapi-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
