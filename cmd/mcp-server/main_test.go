package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

const testToken = "test-secret-token"

type testRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

type testToolCallResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError"`
}

func newTestHandler(t *testing.T, logs io.Writer) http.Handler {
	t.Helper()
	return newHandler(testToken, slog.New(slog.NewJSONHandler(logs, nil)))
}

func doRequest(t *testing.T, h http.Handler, method, path, auth string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func doRPC(t *testing.T, h http.Handler, payload any) (*httptest.ResponseRecorder, testRPCResponse) {
	t.Helper()
	return doRPCWithAuth(t, h, "Bearer "+testToken, payload)
}

func doRPCWithAuth(t *testing.T, h http.Handler, auth string, payload any) (*httptest.ResponseRecorder, testRPCResponse) {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	rec := doRequest(t, h, http.MethodPost, "/mcp", auth, body)
	var resp testRPCResponse
	if rec.Code == http.StatusOK && rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response %q: %v", rec.Body.String(), err)
		}
	}
	return rec, resp
}

func decodeToolResult(t *testing.T, resp testRPCResponse) testToolCallResult {
	t.Helper()
	if resp.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %+v", resp.Error)
	}
	var result testToolCallResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode tool result %q: %v", string(resp.Result), err)
	}
	return result
}

func TestHealthWithoutAuth(t *testing.T) {
	h := newTestHandler(t, io.Discard)
	rec := doRequest(t, h, http.MethodGet, "/health", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode health body: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("status body = %q, want ok", body["status"])
	}
}

func TestAuthRejection(t *testing.T) {
	h := newTestHandler(t, io.Discard)
	payload := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{}}

	tests := []struct {
		name string
		auth string
	}{
		{name: "missing", auth: ""},
		{name: "malformed scheme", auth: "Token " + testToken},
		{name: "wrong token", auth: "Bearer wrong-token"},
		{name: "empty bearer", auth: "Bearer "},
		{name: "no scheme", auth: testToken},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, _ := doRPCWithAuth(t, h, tt.auth, payload)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401 (body %s)", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestInitialize(t *testing.T) {
	h := newTestHandler(t, io.Discard)
	cases := []struct {
		name string
		id   any
		want string
	}{
		{name: "numeric id", id: 7, want: "7"},
		{name: "string id", id: "req-1", want: `"req-1"`},
		{name: "notification", id: nil, want: "null"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := map[string]any{"jsonrpc": "2.0", "method": "initialize", "params": map[string]any{}}
			if tc.id != nil {
				payload["id"] = tc.id
			}
			rec, resp := doRPC(t, h, payload)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			if resp.Error != nil {
				t.Fatalf("unexpected error: %+v", resp.Error)
			}
			if got := string(resp.ID); got != tc.want {
				t.Fatalf("id = %s, want %s", got, tc.want)
			}
			var result struct {
				ProtocolVersion string                     `json:"protocolVersion"`
				Capabilities    map[string]json.RawMessage `json:"capabilities"`
				ServerInfo      struct {
					Name    string `json:"name"`
					Version string `json:"version"`
				} `json:"serverInfo"`
			}
			if err := json.Unmarshal(resp.Result, &result); err != nil {
				t.Fatalf("decode initialize result: %v", err)
			}
			if result.ProtocolVersion != protocolVersion {
				t.Fatalf("protocolVersion = %q, want %q", result.ProtocolVersion, protocolVersion)
			}
			if _, ok := result.Capabilities["tools"]; !ok {
				t.Fatalf("capabilities.tools missing: %s", string(resp.Result))
			}
			if result.ServerInfo.Name != serverName || result.ServerInfo.Version != serverVersion {
				t.Fatalf("serverInfo = %+v", result.ServerInfo)
			}
		})
	}
}

func TestToolsList(t *testing.T) {
	h := newTestHandler(t, io.Discard)
	_, resp := doRPC(t, h, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/list"})
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}

	var result struct {
		Tools []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			InputSchema struct {
				Type       string `json:"type"`
				Properties map[string]struct {
					Type string `json:"type"`
				} `json:"properties"`
				Required []string `json:"required"`
			} `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode tools/list result: %v", err)
	}
	if len(result.Tools) != 2 {
		t.Fatalf("got %d tools, want exactly 2", len(result.Tools))
	}

	byName := map[string]int{}
	for i, tool := range result.Tools {
		byName[tool.Name] = i
		if tool.Description == "" {
			t.Fatalf("tool %q has empty description", tool.Name)
		}
		if tool.InputSchema.Type != "object" {
			t.Fatalf("tool %q schema type = %q, want object", tool.Name, tool.InputSchema.Type)
		}
	}

	formulaIdx, ok := byName["formula_hint"]
	if !ok {
		t.Fatal("formula_hint tool missing")
	}
	formula := result.Tools[formulaIdx]
	if formula.Description != "根据学生问题给出公式或步骤提示，不给出最终数值答案" {
		t.Fatalf("formula_hint description = %q", formula.Description)
	}
	if len(formula.InputSchema.Properties) != 1 || formula.InputSchema.Properties["query"].Type != "string" {
		t.Fatalf("formula_hint properties = %+v", formula.InputSchema.Properties)
	}
	if len(formula.InputSchema.Required) != 1 || formula.InputSchema.Required[0] != "query" {
		t.Fatalf("formula_hint required = %v", formula.InputSchema.Required)
	}

	convertIdx, ok := byName["unit_convert"]
	if !ok {
		t.Fatal("unit_convert tool missing")
	}
	convert := result.Tools[convertIdx]
	if convert.Description != "米与厘米、千克与克之间的简单换算" {
		t.Fatalf("unit_convert description = %q", convert.Description)
	}
	if len(convert.InputSchema.Properties) != 3 {
		t.Fatalf("unit_convert properties = %+v", convert.InputSchema.Properties)
	}
	for name, typ := range map[string]string{"value": "number", "from": "string", "to": "string"} {
		if convert.InputSchema.Properties[name].Type != typ {
			t.Fatalf("unit_convert property %q type = %q, want %q", name, convert.InputSchema.Properties[name].Type, typ)
		}
	}
	required := map[string]bool{}
	for _, name := range convert.InputSchema.Required {
		required[name] = true
	}
	for _, name := range []string{"value", "from", "to"} {
		if !required[name] {
			t.Fatalf("unit_convert required missing %q: %v", name, convert.InputSchema.Required)
		}
	}
}

func TestFormulaHint(t *testing.T) {
	h := newTestHandler(t, io.Discard)
	tests := []struct {
		name     string
		query    string
		contains []string
	}{
		{
			name:     "remove brackets",
			query:    "3(x-2) 怎么去括号？",
			contains: []string{"系数", "符号"},
		},
		{
			name:     "fractions",
			query:    "分数加法怎么通分？",
			contains: []string{"公分母"},
		},
		{
			name:     "generic",
			query:    "这道题下一步该怎么做？",
			contains: []string{"已知条件", "目标"},
		},
	}
	digit := regexp.MustCompile(`\d`)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, resp := doRPC(t, h, map[string]any{
				"jsonrpc": "2.0",
				"id":      3,
				"method":  "tools/call",
				"params": map[string]any{
					"name":      "formula_hint",
					"arguments": map[string]any{"query": tc.query},
				},
			})
			result := decodeToolResult(t, resp)
			if result.IsError {
				t.Fatalf("isError = true, result = %+v", result)
			}
			if len(result.Content) != 1 || result.Content[0].Type != "text" {
				t.Fatalf("content = %+v", result.Content)
			}
			text := result.Content[0].Text
			for _, want := range tc.contains {
				if !strings.Contains(text, want) {
					t.Fatalf("hint %q does not contain %q", text, want)
				}
			}
			if digit.MatchString(text) {
				t.Fatalf("hint must not contain a numeric answer: %q", text)
			}
		})
	}
}

func TestUnitConvert(t *testing.T) {
	h := newTestHandler(t, io.Discard)
	tests := []struct {
		name        string
		value       any
		from        string
		to          string
		want        string
		wantIsError bool
	}{
		{name: "m to cm", value: 1, from: "m", to: "cm", want: "1 米 = 100 厘米"},
		{name: "cm to m", value: 250, from: "cm", to: "m", want: "250 厘米 = 2.5 米"},
		{name: "kg to g", value: 2.5, from: "kg", to: "g", want: "2.5 千克 = 2500 克"},
		{name: "g to kg", value: 500, from: "g", to: "kg", want: "500 克 = 0.5 千克"},
		{name: "unsupported unit", value: 1, from: "km", to: "m", want: "不支持", wantIsError: true},
		{name: "incompatible units", value: 1, from: "m", to: "kg", want: "不支持", wantIsError: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, resp := doRPC(t, h, map[string]any{
				"jsonrpc": "2.0",
				"id":      4,
				"method":  "tools/call",
				"params": map[string]any{
					"name": "unit_convert",
					"arguments": map[string]any{
						"value": tc.value,
						"from":  tc.from,
						"to":    tc.to,
					},
				},
			})
			result := decodeToolResult(t, resp)
			if result.IsError != tc.wantIsError {
				t.Fatalf("isError = %v, want %v (result %+v)", result.IsError, tc.wantIsError, result)
			}
			if len(result.Content) != 1 || result.Content[0].Type != "text" {
				t.Fatalf("content = %+v", result.Content)
			}
			text := result.Content[0].Text
			if tc.wantIsError {
				if !strings.Contains(text, tc.want) {
					t.Fatalf("error text %q does not contain %q", text, tc.want)
				}
				return
			}
			if text != tc.want {
				t.Fatalf("text = %q, want %q", text, tc.want)
			}
		})
	}
}

func TestToolsCallErrors(t *testing.T) {
	h := newTestHandler(t, io.Discard)
	tests := []struct {
		name   string
		params any
		want   string
	}{
		{
			name: "unknown tool",
			params: map[string]any{
				"name":      "does_not_exist",
				"arguments": map[string]any{},
			},
			want: "unknown tool",
		},
		{
			name: "formula missing query",
			params: map[string]any{
				"name":      "formula_hint",
				"arguments": map[string]any{},
			},
			want: "invalid arguments",
		},
		{
			name: "formula wrong query type",
			params: map[string]any{
				"name":      "formula_hint",
				"arguments": map[string]any{"query": 42},
			},
			want: "invalid arguments",
		},
		{
			name: "convert wrong value type",
			params: map[string]any{
				"name":      "unit_convert",
				"arguments": map[string]any{"value": "one", "from": "m", "to": "cm"},
			},
			want: "invalid arguments",
		},
		{
			name: "convert missing unit",
			params: map[string]any{
				"name":      "unit_convert",
				"arguments": map[string]any{"value": 1, "from": "m"},
			},
			want: "invalid arguments",
		},
		{
			name:   "params not an object",
			params: "not-an-object",
			want:   "invalid params",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec, resp := doRPC(t, h, map[string]any{
				"jsonrpc": "2.0",
				"id":      5,
				"method":  "tools/call",
				"params":  tc.params,
			})
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			if resp.Error == nil {
				t.Fatalf("expected JSON-RPC error, got result %s", string(resp.Result))
			}
			if resp.Error.Code != -32602 {
				t.Fatalf("code = %d, want -32602", resp.Error.Code)
			}
			if !strings.Contains(resp.Error.Message, tc.want) {
				t.Fatalf("message = %q, want substring %q", resp.Error.Message, tc.want)
			}
		})
	}
}

func TestJSONRPCEnvelopeErrors(t *testing.T) {
	h := newTestHandler(t, io.Discard)
	tests := []struct {
		name string
		body string
		code int
	}{
		{name: "malformed json", body: `{"jsonrpc":"2.0",`, code: -32700},
		{name: "empty body", body: ``, code: -32700},
		{name: "array body", body: `[{"jsonrpc":"2.0","id":1,"method":"initialize"}]`, code: -32600},
		{name: "wrong version", body: `{"jsonrpc":"1.0","id":1,"method":"initialize"}`, code: -32600},
		{name: "missing method", body: `{"jsonrpc":"2.0","id":1}`, code: -32600},
		{name: "unknown method", body: `{"jsonrpc":"2.0","id":1,"method":"nope"}`, code: -32601},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := doRequest(t, h, http.MethodPost, "/mcp", "Bearer "+testToken, []byte(tc.body))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			var resp testRPCResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode response %q: %v", rec.Body.String(), err)
			}
			if resp.Error == nil {
				t.Fatalf("expected error, got %s", rec.Body.String())
			}
			if resp.Error.Code != tc.code {
				t.Fatalf("code = %d, want %d (message %q)", resp.Error.Code, tc.code, resp.Error.Message)
			}
			if resp.Error.Message == "" {
				t.Fatal("error message is empty")
			}
		})
	}
}

func TestRequestLogging(t *testing.T) {
	var logs bytes.Buffer
	h := newTestHandler(t, &logs)

	_, resp := doRPC(t, h, map[string]any{"jsonrpc": "2.0", "id": 7, "method": "initialize"})
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}

	lines := strings.Split(strings.TrimSpace(logs.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("got %d log lines, want 1: %q", len(lines), logs.String())
	}
	var entry map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
		t.Fatalf("log line is not JSON: %v (%q)", err, lines[0])
	}
	for _, key := range []string{"request_id", "method", "status", "duration_ms"} {
		if _, ok := entry[key]; !ok {
			t.Fatalf("log entry missing %q: %s", key, lines[0])
		}
	}
	if entry["request_id"] != "7" {
		t.Fatalf("request_id = %v, want 7", entry["request_id"])
	}
	if entry["method"] != "initialize" {
		t.Fatalf("method = %v, want initialize", entry["method"])
	}
	if entry["status"] != "ok" {
		t.Fatalf("status = %v, want ok", entry["status"])
	}
	if strings.Contains(logs.String(), testToken) {
		t.Fatal("log output leaked the auth token")
	}
}
