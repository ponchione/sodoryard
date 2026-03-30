// Package router implements the provider router, the entry point for all LLM
// inference in sirtopham. The router selects which provider handles a request
// based on configuration, per-request overrides, and fallback logic. It
// implements the provider.Provider interface so consumers are unaware of
// routing decisions.
package router

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/ponchione/sirtopham/internal/provider"
	"github.com/ponchione/sirtopham/internal/provider/tracking"
)

// Compile-time interface compliance check.
var _ provider.Provider = (*Router)(nil)

// RouteTarget identifies a provider and model to route a request to.
type RouteTarget struct {
	Provider string `yaml:"provider"`
	Model    string `yaml:"model"`
}

// RouterConfig holds the routing configuration parsed from the routing section
// of sirtopham.yaml.
type RouterConfig struct {
	Default  RouteTarget  `yaml:"default"`
	Fallback *RouteTarget `yaml:"fallback"` // nil when no fallback configured
}

// ProviderHealth tracks the health status of a registered provider.
type ProviderHealth struct {
	Healthy       bool
	LastError     error
	LastErrorAt   time.Time
	LastSuccessAt time.Time
}

// Router selects which registered provider handles each LLM request. It
// implements provider.Provider so consumers interact with a single interface
// regardless of which backend is chosen.
type Router struct {
	providers map[string]provider.Provider // keyed by provider Name()
	config    RouterConfig
	health    map[string]*ProviderHealth
	mu        sync.RWMutex
	logger    *slog.Logger
	store     tracking.SubCallStore
}

// NewRouter creates a Router from the given configuration. The store may be nil
// if sub-call tracking is not enabled. The logger may be nil, in which case a
// no-op logger is used.
func NewRouter(config RouterConfig, store tracking.SubCallStore, logger *slog.Logger) (*Router, error) {
	if config.Default.Provider == "" {
		return nil, errors.New("routing.default.provider is required in configuration")
	}
	if config.Default.Model == "" {
		return nil, errors.New("routing.default.model is required in configuration")
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Router{
		providers: make(map[string]provider.Provider),
		config:    config,
		health:    make(map[string]*ProviderHealth),
		logger:    logger,
		store:     store,
	}, nil
}

// RegisterProvider adds a provider to the router. The provider is keyed by its
// Name() and assumed healthy until proven otherwise.
func (r *Router) RegisterProvider(p provider.Provider) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if p == nil {
		return errors.New("cannot register nil provider")
	}
	name := p.Name()
	if _, exists := r.providers[name]; exists {
		return fmt.Errorf("provider already registered: %s", name)
	}
	// Wrap with sub-call tracking if a store is configured.
	if r.store != nil {
		p = tracking.NewTrackedProvider(p, r.store, r.logger)
	}

	r.providers[name] = p
	r.health[name] = &ProviderHealth{Healthy: true}
	r.logger.Info("provider registered", "provider", name)
	return nil
}

// Name returns the literal string "router".
func (r *Router) Name() string {
	return "router"
}

// ProviderHealthMap returns a shallow copy of the health map. The returned map
// is safe for the caller to iterate without holding the router's lock, though
// individual ProviderHealth pointers are shared.
func (r *Router) ProviderHealthMap() map[string]*ProviderHealth {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make(map[string]*ProviderHealth, len(r.health))
	for k, v := range r.health {
		out[k] = v
	}
	return out
}

// Complete routes a completion request to the appropriate provider based on
// per-request override, default configuration, and fallback logic.
func (r *Router) Complete(ctx context.Context, req *provider.Request) (*provider.Response, error) {
	target, targetName, err := r.resolveTarget(ctx, req)
	if err != nil {
		return nil, err
	}

	callReq := cloneRequestWithModel(req, r.resolvedModel(req, targetName))
	resp, callErr := target.Complete(ctx, callReq)

	if callErr == nil {
		r.markSuccess(targetName)
		return resp, nil
	}

	r.markFailure(targetName, callErr)

	// Classify the error and decide whether to fall back.
	return r.handleCompleteError(ctx, req, targetName, callErr)
}

// Stream routes a streaming request to the appropriate provider based on
// per-request override, default configuration, and fallback logic.
func (r *Router) Stream(ctx context.Context, req *provider.Request) (<-chan provider.StreamEvent, error) {
	target, targetName, err := r.resolveTarget(ctx, req)
	if err != nil {
		return nil, err
	}

	callReq := cloneRequestWithModel(req, r.resolvedModel(req, targetName))
	ch, callErr := target.Stream(ctx, callReq)

	if callErr == nil {
		r.markSuccess(targetName)
		return ch, nil
	}

	r.markFailure(targetName, callErr)

	return r.handleStreamError(ctx, req, targetName, callErr)
}

// Models aggregates models from all registered providers. If a provider's
// Models call fails, its models are skipped and a warning is logged.
func (r *Router) Models(ctx context.Context) ([]provider.Model, error) {
	r.mu.RLock()
	providers := make(map[string]provider.Provider, len(r.providers))
	for k, v := range r.providers {
		providers[k] = v
	}
	r.mu.RUnlock()

	var all []provider.Model
	for name, p := range providers {
		models, err := p.Models(ctx)
		if err != nil {
			r.logger.Warn("failed to list models from provider", "provider", name, "error", err)
			continue
		}
		all = append(all, models...)
	}
	return all, nil
}

// resolveTarget determines which provider and provider name should handle the
// request, considering per-request overrides and the default configuration.
func (r *Router) resolveTarget(ctx context.Context, req *provider.Request) (provider.Provider, string, error) {
	// Per-request override: if req.Model is non-empty, find which provider offers it.
	if req.Model != "" {
		p, err := r.resolveOverride(ctx, req.Model)
		if err != nil {
			return nil, "", err
		}
		if p != nil {
			return p, p.Name(), nil
		}
		// Model not found in any provider; fall through to default.
		r.logger.Warn("requested model not found in any registered provider, falling through to default", "model", req.Model)
	}

	// Default routing.
	r.mu.RLock()
	defaultName := r.config.Default.Provider
	p, ok := r.providers[defaultName]
	r.mu.RUnlock()

	if !ok {
		return nil, "", fmt.Errorf("default provider not available: %s", defaultName)
	}
	return p, defaultName, nil
}

// resolveOverride searches all registered providers for one that offers the
// requested model. Returns (nil, nil) if no provider offers the model.
func (r *Router) resolveOverride(ctx context.Context, modelID string) (provider.Provider, error) {
	r.mu.RLock()
	providers := make(map[string]provider.Provider, len(r.providers))
	for k, v := range r.providers {
		providers[k] = v
	}
	r.mu.RUnlock()

	for name, p := range providers {
		models, err := p.Models(ctx)
		if err != nil {
			r.logger.Warn("failed to list models for provider", "provider", name, "error", err)
			continue
		}
		for _, m := range models {
			if m.ID == modelID {
				return p, nil
			}
		}
	}
	return nil, nil
}

// resolvedModel returns the model string that should be set on the request
// for the chosen target. If the request already carries a model (per-request
// override), that model is used; otherwise the default model is used.
func (r *Router) resolvedModel(req *provider.Request, targetName string) string {
	if req.Model != "" {
		return req.Model
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.config.Default.Provider == targetName {
		return r.config.Default.Model
	}
	return ""
}

func cloneRequestWithModel(req *provider.Request, model string) *provider.Request {
	if req == nil {
		return nil
	}
	cloned := *req
	cloned.Model = model
	return &cloned
}

// handleCompleteError classifies the error from a Complete call and optionally
// dispatches a fallback attempt.
func (r *Router) handleCompleteError(ctx context.Context, req *provider.Request, primaryName string, primaryErr error) (*provider.Response, error) {
	cls := classifyError(primaryErr)

	// Auth errors: wrap with actionable message, never fall back.
	if cls == errorClassAuth {
		return nil, wrapAuthError(primaryName, primaryErr)
	}

	// Non-retriable or no fallback configured: return original error.
	if cls != errorClassRetriable || r.config.Fallback == nil {
		return nil, primaryErr
	}

	// Attempt fallback.
	fallbackName := r.config.Fallback.Provider
	r.logger.Warn("primary provider failed, attempting fallback",
		"primary_provider", primaryName,
		"error", primaryErr,
		"fallback_provider", fallbackName,
	)

	r.mu.RLock()
	fbProvider, ok := r.providers[fallbackName]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("primary provider failed and fallback provider not available: %s", fallbackName)
	}

	fallbackReq := cloneRequestWithModel(req, r.config.Fallback.Model)
	resp, fbErr := fbProvider.Complete(ctx, fallbackReq)

	if fbErr == nil {
		r.markSuccess(fallbackName)
		return resp, nil
	}

	r.markFailure(fallbackName, fbErr)
	r.logger.Warn("both primary and fallback providers failed",
		"primary_provider", primaryName,
		"primary_error", primaryErr,
		"fallback_provider", fallbackName,
		"fallback_error", fbErr,
	)
	return nil, fbErr
}

// handleStreamError classifies the error from a Stream call and optionally
// dispatches a fallback attempt.
func (r *Router) handleStreamError(ctx context.Context, req *provider.Request, primaryName string, primaryErr error) (<-chan provider.StreamEvent, error) {
	cls := classifyError(primaryErr)

	if cls == errorClassAuth {
		return nil, wrapAuthError(primaryName, primaryErr)
	}

	if cls != errorClassRetriable || r.config.Fallback == nil {
		return nil, primaryErr
	}

	fallbackName := r.config.Fallback.Provider
	r.logger.Warn("primary provider failed, attempting fallback",
		"primary_provider", primaryName,
		"error", primaryErr,
		"fallback_provider", fallbackName,
	)

	r.mu.RLock()
	fbProvider, ok := r.providers[fallbackName]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("primary provider failed and fallback provider not available: %s", fallbackName)
	}

	fallbackReq := cloneRequestWithModel(req, r.config.Fallback.Model)
	ch, fbErr := fbProvider.Stream(ctx, fallbackReq)

	if fbErr == nil {
		r.markSuccess(fallbackName)
		return ch, nil
	}

	r.markFailure(fallbackName, fbErr)
	r.logger.Warn("both primary and fallback providers failed",
		"primary_provider", primaryName,
		"primary_error", primaryErr,
		"fallback_provider", fallbackName,
		"fallback_error", fbErr,
	)
	return nil, fbErr
}

// wrapAuthError wraps an error with an actionable authentication failure message.
func wrapAuthError(providerName string, err error) error {
	var pe *provider.ProviderError
	if errors.As(err, &pe) {
		return fmt.Errorf("authentication failed for provider %s (HTTP %d): %s. Check your API key in sirtopham.yaml or environment variables.", providerName, pe.StatusCode, pe.Message)
	}
	return err
}
