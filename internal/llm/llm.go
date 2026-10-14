package llm

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode"
)

var ErrUnavailable = errors.New("model gateway unavailable")

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Client struct {
	ChatBaseURL      string
	ChatAPIKey       string
	ChatModel        string
	EmbeddingBaseURL string
	EmbeddingAPIKey  string
	EmbeddingModel   string
	EmbeddingDim     int
	HTTP             *http.Client
}

func New(chatBase, chatKey, chatModel, embBase, embKey, embModel string, dim int) *Client {
	return &Client{
		ChatBaseURL:      strings.TrimRight(chatBase, "/"),
		ChatAPIKey:       chatKey,
		ChatModel:        chatModel,
		EmbeddingBaseURL: strings.TrimRight(embBase, "/"),
		EmbeddingAPIKey:  embKey,
		EmbeddingModel:   embModel,
		EmbeddingDim:     dim,
		HTTP:             &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *Client) Offline() bool { return c.ChatAPIKey == "" }

// Embed returns a fixed-length vector. With a configured gateway it calls the
// OpenAI-compatible /v1/embeddings endpoint; without one it uses a deterministic
// character-bigram hash so the whole retrieval path remains demoable offline.
func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	if c.EmbeddingAPIKey == "" {
		return hashEmbedding(text, c.EmbeddingDim), nil
	}
	payload := map[string]any{"model": c.EmbeddingModel, "input": []string{text}}
	b, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.EmbeddingBaseURL+"/v1/embeddings", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.EmbeddingAPIKey)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("%w: status %d", ErrUnavailable, resp.StatusCode)
	}
	var out struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || len(out.Data) == 0 {
		return nil, fmt.Errorf("%w: malformed embedding response", ErrUnavailable)
	}
	v := out.Data[0].Embedding
	if len(v) != c.EmbeddingDim {
		return nil, fmt.Errorf("%w: embedding dimension %d != configured %d", ErrUnavailable, len(v), c.EmbeddingDim)
	}
	return v, nil
}

// StreamChat sends the prompt to the chat gateway and invokes onDelta for each
// incremental text fragment. It returns the concatenated answer.
func (c *Client) StreamChat(ctx context.Context, messages []Message, onDelta func(string) error) (string, error) {
	if c.Offline() {
		return "", ErrUnavailable
	}
	payload := map[string]any{
		"model":       c.ChatModel,
		"messages":    messages,
		"stream":      true,
		"temperature": 0.2,
		"max_tokens":  500,
	}
	b, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.ChatBaseURL+"/v1/chat/completions", bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.ChatAPIKey)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return "", fmt.Errorf("%w: status %d", ErrUnavailable, resp.StatusCode)
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var sb strings.Builder
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var ev struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			continue
		}
		for _, ch := range ev.Choices {
			piece := ch.Delta.Content
			if piece == "" {
				piece = ch.Message.Content
			}
			if piece == "" {
				continue
			}
			sb.WriteString(piece)
			if onDelta != nil {
				if err := onDelta(piece); err != nil {
					return sb.String(), err
				}
			}
		}
	}
	if err := sc.Err(); err != nil {
		return sb.String(), fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if strings.TrimSpace(sb.String()) == "" {
		return "", fmt.Errorf("%w: empty answer", ErrUnavailable)
	}
	return sb.String(), nil
}

func hashEmbedding(text string, dim int) []float32 {
	v := make([]float32, dim)
	if dim <= 0 {
		return v
	}
	rs := []rune(strings.ToLower(strings.TrimSpace(text)))
	add := func(token string, weight float32) {
		sum := sha256.Sum256([]byte(token))
		idx := int(binary.LittleEndian.Uint32(sum[:4])) % dim
		sign := float32(1)
		if sum[4]&1 == 1 {
			sign = -1
		}
		v[idx] += sign * weight
	}
	for i, r := range rs {
		if unicode.IsSpace(r) {
			continue
		}
		add(string(r), 0.5)
		if i+1 < len(rs) && !unicode.IsSpace(rs[i+1]) {
			add(string(rs[i:i+2]), 1.0)
		}
	}
	// Keep the strongest dimensions so that a short query and a long chunk
	// still share a meaningful direction. This fallback is only used when no
	// embedding gateway is configured.
	keep := 64
	if keep > dim {
		keep = dim
	}
	if keep < len(v) {
		idx := make([]int, len(v))
		for i := range idx {
			idx[i] = i
		}
		sort.Slice(idx, func(i, j int) bool { return math.Abs(float64(v[idx[i]])) > math.Abs(float64(v[idx[j]])) })
		for _, i := range idx[keep:] {
			v[i] = 0
		}
	}
	var norm float64
	for _, x := range v {
		norm += float64(x * x)
	}
	if norm > 0 {
		n := float32(math.Sqrt(norm))
		for i := range v {
			v[i] /= n
		}
	}
	return v
}
