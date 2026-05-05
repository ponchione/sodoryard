// Package router implements the provider router, the entry point for all LLM
// inference in sodoryard. The router selects which provider handles a request
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

	"github.com/ponchione/sodoryard/internal/provider"
	"github.com/ponchione/sodoryard/internal/provider/tracking"
)

// Compile-time interface compliance check.
var _ provider.Provider = (*Router)(nil)

// RouteTarget identifies a provider and model to route a request to.
type RouteTarget struct {
	Provider string `yaml:"provider"`
	Model    string `yaml:"model"`
}

// RouterConfig holds the routing configuration parsed from the routing section
// of yard.yaml.
type RouterConfig struct {
	Default  RouteTarget `yaml:"default"`
	Fallback RouteTarget `yaml:"fallback"`
}

type errorClass int

const (
	errorClassAuth errorClass = iota
	errorClassRetriable
	errorClassFatal
)

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
	providers  map[string]provider.Provider // keyed by provider Name()
	config     RouterConfig
	health     map[string]*ProviderHealth
	mu         sync.RWMutex
	logger     *slog.Logger
	store      tracking.SubCallStore
	modelIndex map[string]string // modelID → provider name; rebuilt on RegisterProvider
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
		providers:  make(map[string]provider.Provider),
		config:     config,
		health:     make(map[string]*ProviderHealth),
		logger:     logger,
		store:      store,
		modelIndex: make(map[string]string),
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

	// Index this provider's models for fast override resolution.
	if models, err := p.Models(context.Background()); err == nil {
		for _, m := range models {
			r.modelIndex[m.ID] = name
		}
	}

	r.logger.Info("provider registered", "provider", name)
	return nil
}

// DrainTracking waits for all in-flight async sub-call writes (from
// TrackedProvider stream goroutines) to complete. Call before closing
// the database to avoid "sql: database is closed" errors.
func (r *Router) DrainTracking() {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.providers {
		if tp, ok := p.(*tracking.TrackedProvider); ok {
			tp.Wait()
		}
	}
}

// Name returns the literal string "router".
func (r *Router) Name() string {
	return "router"
}

// ProviderHealthMap returns a copy of the health map. The returned map and
// ProviderHealth values are safe for callers to inspect and modify without
// affecting the router's internal state.
func (r *Router) ProviderHealthMap() map[string]*ProviderHealth {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make(map[string]*ProviderHealth, len(r.health))
	for k, v := range r.health {
		if v == nil {
			out[k] = nil
			continue
		}
		copy := *v
		out[k] = &copy
	}
	return out
}

// Complete routes a completion request to the appropriate provider based on
// per-request override and default configuration.
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

	switch classifyError(callErr) {
	case errorClassAuth:
		return nil, wrapAuthError(targetName, callErr)
	case errorClassRetriable:
		return r.completeWithFallback(ctx, req, targetName, callErr)
	}
	return nil, callErr
}

// Stream routes a streaming request to the appropriate provider based on
// per-request override and default configuration.
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

	switch classifyError(callErr) {
	case errorClassAuth:
		return nil, wrapAuthError(targetName, callErr)
	case errorClassRetriable:
		return r.streamWithFallback(ctx, req, targetName, callErr)
	}
	return nil, callErr
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
		for _, m := range models {
			if m.Provider == "" {
				m.Provider = name
			}
			all = append(all, m)
		}
	}
	return all, nil
}

func (r *Router) AuthStatuses(ctx context.Context) (map[string]*provider.AuthStatus, error) {
	r.mu.RLock()
	providers := make(map[string]provider.Provider, len(r.providers))
	for k, v := range r.providers {
		providers[k] = v
	}
	r.mu.RUnlock()

	statuses := make(map[string]*provider.AuthStatus, len(providers))
	for name, p := range providers {
		reporter, ok := p.(provider.AuthStatusReporter)
		if !ok {
			statuses[name] = nil
			continue
		}
		status, err := reporter.AuthStatus(ctx)
		if err != nil {
			statuses[name] = &provider.AuthStatus{Provider: name, Detail: err.Error()}
			var pe *provider.ProviderError
			if errors.As(err, &pe) {
				statuses[name].Remediation = pe.Remediation
			}
			continue
		}
		statuses[name] = status
	}
	return statuses, nil
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

// resolveOverride searches the model index for a provider that offers the
// requested model. Returns (nil, nil) if no provider offers the model.
func (r *Router) resolveOverride(_ context.Context, modelID string) (provider.Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	name, ok := r.modelIndex[modelID]
	if !ok {
		return nil, nil
	}
	p, ok := r.providers[name]
	if !ok {
		return nil, nil
	}
	return p, nil
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

func classifyError(err error) errorClass {
	if provider.IsAuthenticationFailure(err) {
		return errorClassAuth
	}

	var pe *provider.ProviderError
	if errors.As(err, &pe) && pe.Retriable {
		return errorClassRetriable
	}

	return errorClassFatal
}

func (r *Router) fallbackTarget(primaryProvider string) (provider.Provider, string, string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	fallback := r.config.Fallback
	if fallback.Provider == "" || fallback.Model == "" || fallback.Provider == primaryProvider {
		return nil, "", "", false
	}

	p, ok := r.providers[fallback.Provider]
	if !ok {
		return nil, fallback.Provider, fallback.Model, false
	}

	return p, fallback.Provider, fallback.Model, true
}

func (r *Router) completeWithFallback(ctx context.Context, req *provider.Request, primaryProvider string, primaryErr error) (*provider.Response, error) {
	return runWithFallback(r, req, primaryProvider, primaryErr, func(p provider.Provider, callReq *provider.Request) (*provider.Response, error) {
		return p.Complete(ctx, callReq)
	})
}

func (r *Router) streamWithFallback(ctx context.Context, req *provider.Request, primaryProvider string, primaryErr error) (<-chan provider.StreamEvent, error) {
	return runWithFallback(r, req, primaryProvider, primaryErr, func(p provider.Provider, callReq *provider.Request) (<-chan provider.StreamEvent, error) {
		return p.Stream(ctx, callReq)
	})
}

func runWithFallback[T any](
	r *Router,
	req *provider.Request,
	primaryProvider string,
	primaryErr error,
	call func(provider.Provider, *provider.Request) (T, error),
) (T, error) {
	var zero T
	fallbackProvider, fallbackName, fallbackModel, ok := r.fallbackTarget(primaryProvider)
	if fallbackName == "" {
		return zero, primaryErr
	}
	if !ok {
		return zero, fmt.Errorf("primary provider failed and fallback provider not available: %s: %w", fallbackName, primaryErr)
	}

	r.logger.Warn("primary provider failed, attempting fallback",
		"primary_provider", primaryProvider,
		"error", primaryErr,
		"fallback_provider", fallbackName,
	)

	callReq := cloneRequestWithModel(req, fallbackModel)
	result, err := call(fallbackProvider, callReq)
	if err == nil {
		r.markSuccess(fallbackName)
		return result, nil
	}

	r.markFailure(fallbackName, err)
	if classifyError(err) == errorClassAuth {
		return zero, wrapAuthError(fallbackName, err)
	}

	r.logger.Warn("both primary and fallback providers failed",
		"primary_provider", primaryProvider,
		"primary_error", primaryErr,
		"fallback_provider", fallbackName,
		"fallback_error", err,
	)
	return zero, err
}

// wrapAuthError wraps an error with an actionable authentication failure message.
func wrapAuthError(providerName string, err error) error {
	var pe *provider.ProviderError
	if !errors.As(err, &pe) {
		return err
	}
	status := ""
	if pe.StatusCode > 0 {
		status = fmt.Sprintf(" (HTTP %d)", pe.StatusCode)
	}
	message := fmt.Sprintf("authentication failed for provider %s%s: %s", providerName, status, pe.Message)
	remediation := pe.Remediation
	if remediation == "" {
		switch providerName {
		case "anthropic":
			remediation = "Configure ANTHROPIC_API_KEY or run `claude login`."
		default:
			remediation = "Check the provider's configured credentials."
		}
	}
	if remediation != "" {
		message += ". " + remediation
	}
	return fmt.Errorf("%s", message)
}
