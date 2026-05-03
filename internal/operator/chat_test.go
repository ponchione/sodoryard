//go:build sqlite_fts5
// +build sqlite_fts5

package operator

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/ponchione/sodoryard/internal/chain"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/conversation"
	appdb "github.com/ponchione/sodoryard/internal/db"
	"github.com/ponchione/sodoryard/internal/provider"
	"github.com/ponchione/sodoryard/internal/provider/router"
	rtpkg "github.com/ponchione/sodoryard/internal/runtime"
)

type chatProviderMock struct {
	req *provider.Request
}

func (p *chatProviderMock) Complete(_ context.Context, req *provider.Request) (*provider.Response, error) {
	p.req = req
	return &provider.Response{
		Content:    []provider.ContentBlock{provider.NewTextBlock("spec outline")},
		Usage:      provider.Usage{InputTokens: 10, OutputTokens: 20},
		Model:      "test-model",
		StopReason: provider.StopReasonEndTurn,
	}, nil
}

func (p *chatProviderMock) Stream(context.Context, *provider.Request) (<-chan provider.StreamEvent, error) {
	return nil, nil
}

func (p *chatProviderMock) Models(context.Context) ([]provider.Model, error) {
	return []provider.Model{{ID: "test-model", Name: "Test Model"}}, nil
}

func (p *chatProviderMock) Name() string {
	return "codex"
}

func TestSendChatMessageUsesRawProviderWithoutToolsOrSystemPrompt(t *testing.T) {
	ctx := context.Background()
	db := newOperatorTestDB(t)
	projectRoot := t.TempDir()
	mock := &chatProviderMock{}
	svc := openChatOperatorTestService(t, projectRoot, db, mock)

	result, err := svc.SendChatMessage(ctx, ChatTurnRequest{Message: "draft a spec"})
	if err != nil {
		t.Fatalf("SendChatMessage returned error: %v", err)
	}
	if result.ConversationID == "" {
		t.Fatal("ConversationID is empty")
	}
	if result.Provider != "codex" || result.Model != "test-model" {
		t.Fatalf("provider/model = %s/%s, want codex/test-model", result.Provider, result.Model)
	}
	if result.InputTokens != 10 || result.OutputTokens != 20 || result.StopReason != string(provider.StopReasonEndTurn) {
		t.Fatalf("usage/stop = %d/%d/%s, want 10/20/end_turn", result.InputTokens, result.OutputTokens, result.StopReason)
	}
	if len(result.Messages) != 2 || result.Messages[0].Role != "user" || result.Messages[0].Content != "draft a spec" || result.Messages[1].Content != "spec outline" {
		t.Fatalf("messages = %+v, want user and assistant transcript", result.Messages)
	}
	if mock.req == nil {
		t.Fatal("provider was not called")
	}
	if len(mock.req.SystemBlocks) != 0 {
		t.Fatalf("SystemBlocks = %+v, want none", mock.req.SystemBlocks)
	}
	if len(mock.req.Tools) != 0 {
		t.Fatalf("Tools = %+v, want none", mock.req.Tools)
	}
	if mock.req.Purpose != "chat" || mock.req.ConversationID != result.ConversationID || mock.req.TurnNumber != 1 {
		t.Fatalf("request metadata = purpose %q conv %q turn %d", mock.req.Purpose, mock.req.ConversationID, mock.req.TurnNumber)
	}
	if len(mock.req.Messages) != 1 || mock.req.Messages[0].Role != provider.RoleUser {
		t.Fatalf("request messages = %+v, want one user message", mock.req.Messages)
	}
	var userText string
	if err := json.Unmarshal(mock.req.Messages[0].Content, &userText); err != nil || userText != "draft a spec" {
		t.Fatalf("user request content = %q err %v, want draft a spec", userText, err)
	}
}

func openChatOperatorTestService(t *testing.T, projectRoot string, db *sql.DB, mock *chatProviderMock) *Service {
	t.Helper()
	configPath := writeOperatorTestConfig(t, projectRoot)
	svc, err := Open(context.Background(), Options{
		ConfigPath: configPath,
		BuildRuntime: func(ctx context.Context, cfg *appconfig.Config) (*rtpkg.OrchestratorRuntime, error) {
			if err := rtpkg.EnsureProjectRecord(ctx, db, cfg); err != nil {
				return nil, err
			}
			provRouter, err := router.NewRouter(router.RouterConfig{
				Default: router.RouteTarget{Provider: "codex", Model: "test-model"},
			}, nil, slog.Default())
			if err != nil {
				return nil, err
			}
			if err := provRouter.RegisterProvider(mock); err != nil {
				return nil, err
			}
			return &rtpkg.OrchestratorRuntime{
				Config:              cfg,
				Database:            db,
				Queries:             appdb.New(db),
				ProviderRouter:      provRouter,
				ConversationManager: conversation.NewManager(db, nil, slog.Default()),
				ChainStore:          chain.NewStore(db),
				Cleanup:             func() {},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(svc.Close)
	return svc
}
