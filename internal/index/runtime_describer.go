package index

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ponchione/sodoryard/internal/codeintel"
	codedescriber "github.com/ponchione/sodoryard/internal/codeintel/describer"
	"github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/provider"
)

const runtimeDescriberSystemPrompt = `You generate semantic descriptions for code entities to improve code search embeddings.
Return JSON only: an array of objects with keys "name" and "description".
Only include functions, methods, types, or other top-level symbols that actually appear in the file.
Each description should be 1-2 concise sentences focused on purpose and behavior, not syntax.`

func newRuntimeDescriber(cfg *config.Config) codeintel.Describer {
	if cfg == nil {
		return noopDescriber{}
	}
	service := cfg.LocalService("qwen-coder")
	baseURL := strings.TrimRight(service.BaseURL, "/")
	if baseURL == "" {
		slog.Warn("index describer disabled: qwen-coder base_url is empty")
		return noopDescriber{}
	}
	modelsPath := strings.TrimSpace(service.ModelsPath)
	if modelsPath == "" {
		modelsPath = "/v1/models"
	}
	llm := &runtimeDescriberLLM{
		baseURL:    baseURL,
		modelsURL:  baseURL + modelsPath,
		httpClient: &http.Client{Timeout: 90 * time.Second},
	}
	return codedescriber.New(llm, runtimeDescriberSystemPrompt)
}

type runtimeDescriberLLM struct {
	baseURL    string
	modelsURL  string
	httpClient *http.Client

	mu      sync.Mutex
	modelID string
}

type modelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

type chatCompletionRequest struct {
	Model       string                `json:"model"`
	Messages    []chatMessage         `json:"messages"`
	Temperature float64               `json:"temperature,omitempty"`
	MaxTokens   int                   `json:"max_tokens,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (l *runtimeDescriberLLM) Complete(ctx context.Context, systemPrompt string, userMessage string) (string, error) {
	modelID, err := l.resolveModelID(ctx)
	if err != nil {
		return "", err
	}

	body, err := json.Marshal(chatCompletionRequest{
		Model: modelID,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: string(provider.RoleUser), Content: userMessage},
		},
		Temperature: 0.1,
		MaxTokens:   2048,
	})
	if err != nil {
		return "", fmt.Errorf("marshal describer request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create describer request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := l.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("send describer request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", fmt.Errorf("describer endpoint returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var decoded chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", fmt.Errorf("decode describer response: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return "", fmt.Errorf("describer endpoint returned no choices")
	}
	return decoded.Choices[0].Message.Content, nil
}

func (l *runtimeDescriberLLM) resolveModelID(ctx context.Context) (string, error) {
	l.mu.Lock()
	if l.modelID != "" {
		defer l.mu.Unlock()
		return l.modelID, nil
	}
	l.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.modelsURL, nil)
	if err != nil {
		return "", fmt.Errorf("create describer models request: %w", err)
	}
	resp, err := l.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("query describer models: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("describer models endpoint returned HTTP %d", resp.StatusCode)
	}

	var decoded modelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", fmt.Errorf("decode describer models response: %w", err)
	}
	if len(decoded.Data) == 0 || strings.TrimSpace(decoded.Data[0].ID) == "" {
		return "", fmt.Errorf("describer models endpoint returned no models")
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.modelID == "" {
		l.modelID = decoded.Data[0].ID
	}
	return l.modelID, nil
}
