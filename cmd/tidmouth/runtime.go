package main

import (
	"context"
	"database/sql"
	"log/slog"

	"github.com/ponchione/sodoryard/internal/brain"
	codesearcher "github.com/ponchione/sodoryard/internal/codeintel/searcher"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	contextpkg "github.com/ponchione/sodoryard/internal/context"
	"github.com/ponchione/sodoryard/internal/conversation"
	appdb "github.com/ponchione/sodoryard/internal/db"
	"github.com/ponchione/sodoryard/internal/provider/router"
	rtpkg "github.com/ponchione/sodoryard/internal/runtime"
)

type appRuntime struct {
	Config              *appconfig.Config
	Logger              *slog.Logger
	Database            *sql.DB
	Queries             *appdb.Queries
	ProviderRouter      *router.Router
	BrainBackend        brain.Backend
	SemanticSearcher    *codesearcher.Searcher
	BrainSearcher       *contextpkg.HybridBrainSearcher
	ConversationManager *conversation.Manager
	ContextAssembler    *contextpkg.ContextAssembler
	Cleanup             func()
}

func buildAppRuntime(ctx context.Context, cfg *appconfig.Config) (*appRuntime, error) {
	rt, err := rtpkg.BuildEngineRuntime(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &appRuntime{
		Config:              rt.Config,
		Logger:              rt.Logger,
		Database:            rt.Database,
		Queries:             rt.Queries,
		ProviderRouter:      rt.ProviderRouter,
		BrainBackend:        rt.BrainBackend,
		SemanticSearcher:    rt.SemanticSearcher,
		BrainSearcher:       rt.BrainSearcher,
		ConversationManager: rt.ConversationManager,
		ContextAssembler:    rt.ContextAssembler,
		Cleanup:             rt.Cleanup,
	}, nil
}
