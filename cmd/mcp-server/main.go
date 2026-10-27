package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	serverName      = "campus-tools"
	serverVersion   = "1.0.0"
	protocolVersion = "2024-11-05"
	defaultPort     = "8090"
	maxRequestBody  = 1 << 20
)

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type contentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolResult struct {
	Content []contentItem `json:"content"`
	IsError bool          `json:"isError"`
}

type toolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type server struct {
	token  string
	logger *slog.Logger
}

func newHandler(token string, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	s := &server{token: token, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("POST /mcp", s.handleMCP)
	return mux
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	s.logRequest("-", r.Method, "ok", start)
}

func (s *server) handleMCP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	if !s.authorized(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		s.logRequest("-", r.Method, "unauthorized", start)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBody))
	if err != nil {
		resp := errorResponse(nil, -32700, "parse error: request body is too large or unreadable")
		writeJSON(w, http.StatusOK, resp)
		s.logRequest("-", r.Method, "parse_error", start)
		return
	}

	req, resp := s.process(body)
	writeJSON(w, http.StatusOK, resp)
	status := "ok"
	if resp.Error != nil {
		status = "error"
	}
	method := req.Method
	if method == "" {
		method = r.Method
	}
	s.logRequest(requestIDString(req.ID), method, status, start)
}

func (s *server) authorized(r *http.Request) bool {
	if s.token == "" {
		return false
	}
	const prefix = "Bearer "
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	got := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	return subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) == 1
}

func (s *server) process(body []byte) (rpcRequest, rpcResponse) {
	var req rpcRequest
	var raw json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return req, errorResponse(nil, -32700, "parse error: invalid JSON")
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		return req, errorResponse(nil, -32600, "invalid request: expected a JSON-RPC object")
	}
	if req.JSONRPC != "2.0" || strings.TrimSpace(req.Method) == "" {
		return req, errorResponse(req.ID, -32600, "invalid request: jsonrpc must be \"2.0\" and method is required")
	}
	return req, s.call(req)
}

func (s *server) call(req rpcRequest) (resp rpcResponse) {
	defer func() {
		if rec := recover(); rec != nil {
			s.logger.Error("mcp internal panic", "method", req.Method, "panic", fmt.Sprint(rec))
			resp = errorResponse(req.ID, -32603, "internal error")
		}
	}()

	switch req.Method {
	case "initialize":
		return rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"protocolVersion": protocolVersion,
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": serverName, "version": serverVersion},
			},
		}
	case "tools/list":
		return rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]any{"tools": toolDefinitions()},
		}
	case "tools/call":
		return s.callTool(req)
	default:
		return errorResponse(req.ID, -32601, "method not found: "+req.Method)
	}
}

func (s *server) callTool(req rpcRequest) rpcResponse {
	if len(req.Params) == 0 || string(req.Params) == "null" {
		return errorResponse(req.ID, -32602, "invalid params: name and arguments are required")
	}
	var params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errorResponse(req.ID, -32602, "invalid params: params must be an object with name and arguments")
	}
	if strings.TrimSpace(params.Name) == "" {
		return errorResponse(req.ID, -32602, "invalid params: tool name is required")
	}
	if params.Arguments == nil {
		params.Arguments = map[string]any{}
	}
	result, err := callTool(params.Name, params.Arguments)
	if err != nil {
		return errorResponse(req.ID, -32602, err.Error())
	}
	return rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: result}
}

func toolDefinitions() []toolDefinition {
	return []toolDefinition{
		{
			Name:        "formula_hint",
			Description: "根据学生问题给出公式或步骤提示，不给出最终数值答案",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "学生的问题或正在检查的解题步骤",
					},
				},
				"required": []string{"query"},
			},
		},
		{
			Name:        "unit_convert",
			Description: "米与厘米、千克与克之间的简单换算",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"value": map[string]any{
						"type":        "number",
						"description": "需要换算的数值",
					},
					"from": map[string]any{
						"type":        "string",
						"description": "原单位，支持 m、cm、kg、g",
					},
					"to": map[string]any{
						"type":        "string",
						"description": "目标单位，支持 m、cm、kg、g",
					},
				},
				"required": []string{"value", "from", "to"},
			},
		},
	}
}

func callTool(name string, args map[string]any) (toolResult, error) {
	switch name {
	case "formula_hint":
		return formulaHint(args)
	case "unit_convert":
		return unitConvert(args)
	default:
		return toolResult{}, fmt.Errorf("unknown tool: %s", name)
	}
}

func formulaHint(args map[string]any) (toolResult, error) {
	query, ok := args["query"].(string)
	if !ok || strings.TrimSpace(query) == "" {
		return toolResult{}, errors.New("invalid arguments: query must be a non-empty string")
	}

	var hint string
	switch {
	case strings.Contains(query, "去括号") || strings.Contains(query, "括号"):
		hint = "去括号时，要把括号外的系数乘到括号内的每一项，并注意符号变化。先检查这一步，不要急着算出最终数值。"
	case strings.Contains(query, "通分") || strings.Contains(query, "分数"):
		hint = "分数运算先找公分母，把每个分数化成同分母后再合并分子。先检查通分这一步是否正确。"
	default:
		hint = "先写出已知条件与目标，再检查下一步。这里只给思路提示，不给出最终数值答案。"
	}
	return textResult(hint, false), nil
}

var unitNames = map[string]string{
	"m":  "米",
	"cm": "厘米",
	"kg": "千克",
	"g":  "克",
}

func unitConvert(args map[string]any) (toolResult, error) {
	value, ok := numericValue(args["value"])
	if !ok {
		return toolResult{}, errors.New("invalid arguments: value must be a number")
	}
	from, ok := stringValue(args["from"])
	if !ok || strings.TrimSpace(from) == "" {
		return toolResult{}, errors.New("invalid arguments: from must be a non-empty string")
	}
	to, ok := stringValue(args["to"])
	if !ok || strings.TrimSpace(to) == "" {
		return toolResult{}, errors.New("invalid arguments: to must be a non-empty string")
	}
	from = strings.ToLower(strings.TrimSpace(from))
	to = strings.ToLower(strings.TrimSpace(to))

	if !supportedUnit(from) || !supportedUnit(to) {
		bad := make([]string, 0, 2)
		if !supportedUnit(from) {
			bad = append(bad, from)
		}
		if !supportedUnit(to) {
			bad = append(bad, to)
		}
		reason := fmt.Sprintf("不支持的换算单位：%s。仅支持 m 与 cm、kg 与 g 之间的换算。", strings.Join(bad, "、"))
		return textResult(reason, true), nil
	}

	result, ok := convert(value, from, to)
	if !ok {
		reason := fmt.Sprintf("不支持的换算：%s 到 %s。仅支持 m 与 cm、kg 与 g 之间的换算。", from, to)
		return textResult(reason, true), nil
	}

	text := fmt.Sprintf("%s %s = %s %s", formatNumber(value), unitNames[from], formatNumber(result), unitNames[to])
	return textResult(text, false), nil
}

func convert(value float64, from, to string) (float64, bool) {
	switch {
	case from == to:
		return value, true
	case from == "m" && to == "cm":
		return value * 100, true
	case from == "cm" && to == "m":
		return value / 100, true
	case from == "kg" && to == "g":
		return value * 1000, true
	case from == "g" && to == "kg":
		return value / 1000, true
	default:
		return 0, false
	}
}

func supportedUnit(unit string) bool {
	_, ok := unitNames[unit]
	return ok
}

func numericValue(v any) (float64, bool) {
	var n float64
	switch v := v.(type) {
	case float64:
		n = v
	case float32:
		n = float64(v)
	case int:
		n = float64(v)
	case int32:
		n = float64(v)
	case int64:
		n = float64(v)
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			return 0, false
		}
		n = f
	default:
		return 0, false
	}
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, false
	}
	return n, true
}

func stringValue(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok
}

func formatNumber(v float64) string {
	if v == 0 {
		return "0"
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func textResult(text string, isError bool) toolResult {
	return toolResult{
		Content: []contentItem{{Type: "text", Text: text}},
		IsError: isError,
	}
}

func errorResponse(id json.RawMessage, code int, message string) rpcResponse {
	return rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &rpcError{Code: code, Message: message},
	}
}

func requestIDString(id json.RawMessage) string {
	if len(id) == 0 || string(id) == "null" {
		return "-"
	}
	trimmed := strings.TrimSpace(string(id))
	if strings.HasPrefix(trimmed, `"`) {
		var s string
		if err := json.Unmarshal(id, &s); err == nil {
			return s
		}
	}
	return trimmed
}

func (s *server) logRequest(requestID, method, status string, start time.Time) {
	if requestID == "" {
		requestID = "-"
	}
	if method == "" {
		method = "-"
	}
	s.logger.Info("mcp request",
		"request_id", requestID,
		"method", method,
		"status", status,
		"duration_ms", time.Since(start).Milliseconds(),
	)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	token := strings.TrimSpace(os.Getenv("MCP_AUTH_TOKEN"))
	if token == "" {
		logger.Error("MCP_AUTH_TOKEN is required")
		os.Exit(1)
	}

	port := strings.TrimSpace(os.Getenv("MCP_PORT"))
	if port == "" {
		port = defaultPort
	}
	addr := port
	if !strings.HasPrefix(addr, ":") {
		addr = ":" + addr
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           newHandler(token, logger),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		logger.Info("mcp server listening", "addr", httpServer.Addr, "auth_required", true)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("mcp server failed", "error", err.Error())
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown failed", "error", err.Error())
	}
	logger.Info("shutdown complete")
}
