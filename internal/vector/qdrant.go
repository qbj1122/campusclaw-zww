package vector

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var ErrUnavailable = errors.New("vector store unavailable")

type Client struct {
	BaseURL    string
	Collection string
	HTTP       *http.Client
}

type Point struct {
	ID      int64          `json:"id"`
	Vector  []float32      `json:"vector"`
	Payload map[string]any `json:"payload"`
}

type ScoredPoint struct {
	ID      int64          `json:"id"`
	Score   float64        `json:"score"`
	Payload map[string]any `json:"payload"`
}

func New(baseURL, collection string) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		Collection: collection,
		HTTP:       &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *Client) EnsureCollection(ctx context.Context, dim int) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/collections/"+c.Collection, nil)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		var body struct {
			Result struct {
				Config struct {
					Params struct {
						Vectors struct {
							Size int `json:"size"`
						} `json:"vectors"`
					} `json:"params"`
				} `json:"config"`
			} `json:"result"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&body)
		if body.Result.Config.Params.Vectors.Size != 0 && body.Result.Config.Params.Vectors.Size != dim {
			return fmt.Errorf("qdrant collection %s has dimension %d, expected %d", c.Collection, body.Result.Config.Params.Vectors.Size, dim)
		}
		return nil
	}
	if resp.StatusCode != http.StatusNotFound {
		io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("%w: collection lookup status %d", ErrUnavailable, resp.StatusCode)
	}
	payload := map[string]any{"vectors": map[string]any{"size": dim, "distance": "Cosine"}}
	b, _ := json.Marshal(payload)
	req, _ = http.NewRequestWithContext(ctx, http.MethodPut, c.BaseURL+"/collections/"+c.Collection, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp2, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp2.Body, 2048))
		return fmt.Errorf("%w: create collection %d %s", ErrUnavailable, resp2.StatusCode, string(msg))
	}
	return nil
}

func (c *Client) Upsert(ctx context.Context, points []Point) error {
	if len(points) == 0 {
		return nil
	}
	b, _ := json.Marshal(map[string]any{"points": points})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPut, c.BaseURL+"/collections/"+c.Collection+"/points?wait=true", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("%w: upsert %d %s", ErrUnavailable, resp.StatusCode, string(msg))
	}
	return nil
}

// DeleteEntry removes every vector belonging to one knowledge entry, used
// before a re-index writes a fresh set of chunks.
func (c *Client) DeleteEntry(ctx context.Context, entryID int64) error {
	payload := map[string]any{"filter": map[string]any{"must": []any{
		map[string]any{"key": "knowledge_entry_id", "match": map[string]any{"value": entryID}},
	}}}
	b, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/collections/"+c.Collection+"/points/delete?wait=true", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("%w: delete %d %s", ErrUnavailable, resp.StatusCode, string(msg))
	}
	return nil
}

func (c *Client) Search(ctx context.Context, vector []float32, classID int64, limit int) ([]ScoredPoint, error) {
	payload := map[string]any{
		"vector":       vector,
		"limit":        limit,
		"with_payload": true,
		"filter": map[string]any{"must": []any{
			map[string]any{"key": "class_id", "match": map[string]any{"value": classID}},
		}},
	}
	b, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/collections/"+c.Collection+"/points/search", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("%w: search %d %s", ErrUnavailable, resp.StatusCode, string(msg))
	}
	var out struct {
		Result []ScoredPoint `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("%w: malformed search response", ErrUnavailable)
	}
	return out.Result, nil
}

func (c *Client) Healthy(ctx context.Context) bool {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/collections", nil)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode == http.StatusOK
}
