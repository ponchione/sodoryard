package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/ponchione/sirtopham/internal/provider"
)

func visibleModels() []provider.Model {
	return []provider.Model{
		{ID: "gpt-5.4", Name: "gpt-5.4", ContextWindow: 400000, SupportsTools: true, SupportsThinking: false},
		{ID: "gpt-5.4-mini", Name: "gpt-5.4-mini", ContextWindow: 400000, SupportsTools: true, SupportsThinking: false},
		{ID: "gpt-5.3-codex", Name: "gpt-5.3-codex", ContextWindow: 400000, SupportsTools: true, SupportsThinking: false},
		{ID: "gpt-5.3-codex-spark", Name: "gpt-5.3-codex-spark", ContextWindow: 400000, SupportsTools: true, SupportsThinking: false},
		{ID: "gpt-5.2-codex", Name: "gpt-5.2-codex", ContextWindow: 400000, SupportsTools: true, SupportsThinking: false},
		{ID: "gpt-5.2", Name: "gpt-5.2", ContextWindow: 400000, SupportsTools: true, SupportsThinking: false},
		{ID: "gpt-5.1-codex-max", Name: "gpt-5.1-codex-max", ContextWindow: 400000, SupportsTools: true, SupportsThinking: false},
		{ID: "gpt-5.1-codex-mini", Name: "gpt-5.1-codex-mini", ContextWindow: 400000, SupportsTools: true, SupportsThinking: false},
	}
}

type appServerRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type appServerResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method,omitempty"`
	Result  struct {
		Items []struct {
			ID          string `json:"id"`
			DisplayName string `json:"displayName"`
			Hidden      bool   `json:"hidden"`
		} `json:"items"`
		Data []struct {
			ID          string `json:"id"`
			DisplayName string `json:"displayName"`
			Hidden      bool   `json:"hidden"`
		} `json:"data"`
	} `json:"result"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func discoverVisibleModels(ctx context.Context, codexBinPath string) ([]provider.Model, error) {
	cmd := exec.CommandContext(ctx, "script", "-qefc", shellQuote(codexBinPath)+" app-server --listen stdio://", "/dev/null")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start codex app-server: %w", err)
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Wait()
	}()

	enc := json.NewEncoder(stdin)
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	nextResponse := func(expectedID int) (*appServerResponse, error) {
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			var resp appServerResponse
			if err := json.Unmarshal([]byte(line), &resp); err != nil {
				continue
			}
			if resp.Method != "" || resp.ID != expectedID {
				continue
			}
			return &resp, nil
		}
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("EOF")
	}

	if err := enc.Encode(appServerRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params: map[string]any{
			"clientInfo": map[string]any{"name": "sirtopham-model-discovery", "version": "0.0.0"},
		},
	}); err != nil {
		return nil, fmt.Errorf("send initialize: %w", err)
	}
	initResp, err := nextResponse(1)
	if err != nil {
		return nil, fmt.Errorf("decode initialize response: %w", err)
	}
	if initResp.Error != nil {
		return nil, fmt.Errorf("initialize failed: %s", initResp.Error.Message)
	}

	if err := enc.Encode(appServerRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "model/list",
		Params:  map[string]any{"includeHidden": false, "limit": 100},
	}); err != nil {
		return nil, fmt.Errorf("send model/list: %w", err)
	}

	listResp, err := nextResponse(2)
	if err != nil {
		return nil, fmt.Errorf("decode model/list response: %w", err)
	}
	if listResp.Error != nil {
		return nil, fmt.Errorf("model/list failed: %s", listResp.Error.Message)
	}

	items := listResp.Result.Items
	if len(items) == 0 {
		items = listResp.Result.Data
	}

	metadata := map[string]provider.Model{}
	for _, m := range visibleModels() {
		metadata[m.ID] = m
	}

	models := make([]provider.Model, 0, len(items))
	for _, item := range items {
		if item.Hidden || item.ID == "" {
			continue
		}
		if known, ok := metadata[item.ID]; ok {
			models = append(models, known)
			continue
		}
		name := item.DisplayName
		if name == "" {
			name = item.ID
		}
		models = append(models, provider.Model{
			ID:               item.ID,
			Name:             name,
			ContextWindow:    400000,
			SupportsTools:    true,
			SupportsThinking: false,
		})
	}
	return models, nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
