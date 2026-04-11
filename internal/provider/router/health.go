package router

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"time"

	"github.com/ponchione/sodoryard/internal/provider"
)

// markSuccess updates a provider's health record after a successful call.
func (r *Router) markSuccess(providerName string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	h, ok := r.health[providerName]
	if !ok {
		return
	}
	h.Healthy = true
	h.LastSuccessAt = time.Now()
}

// markFailure updates a provider's health record after a failed call.
// Client-side request validation errors (for example a bad runtime model override)
// are preserved as request errors but do not poison the provider's global health.
func (r *Router) markFailure(providerName string, err error) {
	if !shouldAffectProviderHealth(err) {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	h, ok := r.health[providerName]
	if !ok {
		return
	}
	h.Healthy = false
	h.LastError = err
	h.LastErrorAt = time.Now()
}

func shouldAffectProviderHealth(err error) bool {
	if err == nil {
		return false
	}

	var pe *provider.ProviderError
	if !errors.As(err, &pe) {
		return true
	}

	if provider.IsAuthenticationFailure(err) {
		return true
	}

	return pe.StatusCode != 400
}

// Validate performs startup validation of all registered providers and ensures
// at least one provider is usable. Unreachable providers are unregistered with
// a warning rather than blocking startup.
//
// Validation is split into three phases to avoid holding the write lock during
// potentially slow network I/O (Models() calls with 5s timeouts):
//  1. Snapshot providers under a read lock
//  2. Validate each provider without any lock held
//  3. Apply mutations (removals, default switch) under a write lock
func (r *Router) Validate(ctx context.Context) error {
	// Phase 1: Snapshot registered providers under a read lock.
	r.mu.RLock()
	if len(r.providers) == 0 {
		r.mu.RUnlock()
		return fmt.Errorf("no providers configured; add at least one provider to the project's YAML config")
	}
	snapshot := make(map[string]provider.Provider, len(r.providers))
	for k, v := range r.providers {
		snapshot[k] = v
	}
	r.mu.RUnlock()

	// Phase 2: Validate without holding any lock.
	var toRemove []string
	defaultProvider := r.config.Default.Provider

	// Check codex binary availability.
	if _, ok := snapshot["codex"]; ok {
		if _, err := exec.LookPath("codex"); err != nil {
			wrapped := fmt.Errorf("Codex CLI not found on PATH")
			if defaultProvider == "codex" {
				return fmt.Errorf("default provider %q failed startup validation: %w", defaultProvider, wrapped)
			}
			r.logger.Warn("codex binary not found on PATH, unregistering codex provider")
			toRemove = append(toRemove, "codex")
		}
	}

	// Validate each remaining provider. If the provider implements Pinger,
	// use its lightweight Ping() check with a provider-appropriate timeout.
	// Otherwise fall back to a Models() call with a 5s timeout.
	for name, p := range snapshot {
		if slices.Contains(toRemove, name) {
			continue
		}

		var err error
		if pinger, ok := p.(provider.Pinger); ok {
			// Use the provider's lightweight ping (e.g., auth check or HEAD).
			// Use 2s for local/OpenAI-compatible (fast), 5s for Anthropic (auth).
			timeout := 2 * time.Second
			if name == "anthropic" {
				timeout = 5 * time.Second
			}
			checkCtx, cancel := context.WithTimeout(ctx, timeout)
			err = pinger.Ping(checkCtx)
			cancel()
			if err != nil {
				r.logger.Warn("provider failed Ping() startup validation, unregistering",
					"provider", name, "error", err)
			}
		} else {
			// Fallback: use Models() for providers that don't implement Pinger.
			checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			_, err = p.Models(checkCtx)
			cancel()
			if err != nil {
				r.logger.Warn("provider failed Models() startup validation, unregistering",
					"provider", name, "error", err)
			}
		}

		if err != nil {
			if name == defaultProvider {
				if provider.IsAuthenticationFailure(err) {
					return wrapAuthError(name, err)
				}
				return fmt.Errorf("default provider %q failed startup validation: %w", defaultProvider, err)
			}
			toRemove = append(toRemove, name)
		}
	}

	// Phase 3: Apply mutations under a write lock.
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, name := range toRemove {
		delete(r.providers, name)
		delete(r.health, name)
	}

	// After all checks, verify the default provider is still registered.
	if _, ok := r.providers[r.config.Default.Provider]; !ok {
		return fmt.Errorf("default provider %q not available after startup validation; check provider configuration and connectivity", r.config.Default.Provider)
	}

	return nil
}
