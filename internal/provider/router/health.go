package router

import (
	"context"
	"fmt"
	"os/exec"
	"time"
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
func (r *Router) markFailure(providerName string, err error) {
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

// Validate performs startup validation of all registered providers and ensures
// at least one provider is usable. Unreachable providers are unregistered with
// a warning rather than blocking startup.
func (r *Router) Validate(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.providers) == 0 {
		return fmt.Errorf("no providers configured; add at least one provider to sirtopham.yaml")
	}

	// Check codex binary availability.
	if _, ok := r.providers["codex"]; ok {
		if _, err := exec.LookPath("codex"); err != nil {
			r.logger.Warn("codex binary not found on PATH, unregistering codex provider")
			delete(r.providers, "codex")
			delete(r.health, "codex")
		}
	}

	// Validate each remaining provider with a Models() call (timeout per provider).
	var toRemove []string
	for name, p := range r.providers {
		checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		_, err := p.Models(checkCtx)
		cancel()
		if err != nil {
			r.logger.Warn("provider failed startup validation, unregistering",
				"provider", name, "error", err)
			toRemove = append(toRemove, name)
		}
	}
	for _, name := range toRemove {
		delete(r.providers, name)
		delete(r.health, name)
	}

	// After all checks, verify the default provider is still registered.
	if _, ok := r.providers[r.config.Default.Provider]; !ok {
		r.logger.Error("configured default provider not available, falling back to first available",
			"configured", r.config.Default.Provider,
		)
		found := false
		for name := range r.providers {
			r.config.Default.Provider = name
			found = true
			break
		}
		if !found {
			return fmt.Errorf("no providers available after startup validation; check provider configuration and connectivity")
		}
	}

	return nil
}
