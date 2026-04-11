package embedder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

	"github.com/ponchione/sodoryard/internal/codeintel"
	"github.com/ponchione/sodoryard/internal/config"
)

// Client is an HTTP client for the OpenAI-compatible embedding API.
type Client struct {
	httpClient  *http.Client
	baseURL     string
	model       string
	batchSize   int
	queryPrefix string
}

// New creates a Client from the given embedding configuration.
func New(cfg config.Embedding) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second,
		},
		baseURL:     cfg.BaseURL,
		model:       cfg.Model,
		batchSize:   cfg.BatchSize,
		queryPrefix: cfg.QueryPrefix,
	}
}

type embeddingRequest struct {
	Input []string `json:"input"`
	Model string   `json:"model"`
}

type embeddingResponse struct {
	Data []embeddingData `json:"data"`
}

type embeddingData struct {
	Embedding []float32 `json:"embedding"`
	Index     int       `json:"index"`
}

// sendBatch POSTs a batch of texts to the embedding service and returns the
// resulting vectors ordered by input index.
func (c *Client) sendBatch(ctx context.Context, texts []string) ([][]float32, error) {
	reqBody, err := json.Marshal(embeddingRequest{
		Input: texts,
		Model: c.model,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal embedding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/embeddings", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create embedding request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send embedding request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return nil, fmt.Errorf("embedding request failed (HTTP %d): %s", resp.StatusCode, body)
	}

	var embResp embeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&embResp); err != nil {
		return nil, fmt.Errorf("decode embedding response: %w", err)
	}

	if len(embResp.Data) != len(texts) {
		return nil, fmt.Errorf("embedding count mismatch: got %d, want %d", len(embResp.Data), len(texts))
	}

	for i, d := range embResp.Data {
		if len(d.Embedding) != codeintel.DefaultEmbeddingDims {
			return nil, fmt.Errorf("embedding[%d] dimension mismatch: got %d, want %d", i, len(d.Embedding), codeintel.DefaultEmbeddingDims)
		}
	}

	sort.Slice(embResp.Data, func(i, j int) bool {
		return embResp.Data[i].Index < embResp.Data[j].Index
	})

	vecs := make([][]float32, len(embResp.Data))
	for i, d := range embResp.Data {
		vecs[i] = d.Embedding
	}
	return vecs, nil
}

// EmbedTexts embeds a batch of indexing texts. If the input exceeds the
// configured batch size, it is automatically split into sub-batches.
func (c *Client) EmbedTexts(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	embeddings := make([][]float32, len(texts))

	for start := 0; start < len(texts); start += c.batchSize {
		end := min(start+c.batchSize, len(texts))

		vecs, err := c.sendBatch(ctx, texts[start:end])
		if err != nil {
			return nil, fmt.Errorf("embed batch [%d:%d]: %w", start, end, err)
		}

		copy(embeddings[start:end], vecs)
	}

	return embeddings, nil
}

// EmbedQuery embeds a single retrieval query. The configured query prefix
// is prepended before embedding.
func (c *Client) EmbedQuery(ctx context.Context, query string) ([]float32, error) {
	vecs, err := c.sendBatch(ctx, []string{c.queryPrefix + query})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("embed query: no embeddings returned")
	}
	return vecs[0], nil
}
