package provider

import (
	"context"
	"time"
)

// AuthErrorKind classifies authentication failures so callers can report
// provider-specific remediation instead of defaulting to API-key language.
type AuthErrorKind string

const (
	AuthMissingCredentials AuthErrorKind = "missing_credentials"
	AuthInvalidCredentials AuthErrorKind = "invalid_credentials"
	AuthExpiredCredentials AuthErrorKind = "expired_credentials"
	AuthRefreshFailed      AuthErrorKind = "refresh_failed"
	AuthPermissionDenied   AuthErrorKind = "permission_denied"
	AuthMisconfigured      AuthErrorKind = "misconfigured"
)

// AuthStatus describes the currently discovered auth state for a provider.
type AuthStatus struct {
	Provider        string    `json:"provider"`
	Mode            string    `json:"mode,omitempty"`
	Source          string    `json:"source,omitempty"`
	StorePath       string    `json:"store_path,omitempty"`
	SourcePath      string    `json:"source_path,omitempty"`
	ActiveProvider  string    `json:"active_provider,omitempty"`
	Version         int       `json:"version,omitempty"`
	LastRefresh     time.Time `json:"last_refresh,omitempty"`
	ExpiresAt       time.Time `json:"expires_at,omitempty"`
	HasAccessToken  bool      `json:"has_access_token"`
	HasRefreshToken bool      `json:"has_refresh_token"`
	Detail          string    `json:"detail,omitempty"`
	Remediation     string    `json:"remediation,omitempty"`
}

// AuthStatusReporter is an optional interface for providers that can expose
// structured auth status for doctor/status surfaces.
type AuthStatusReporter interface {
	AuthStatus(ctx context.Context) (*AuthStatus, error)
}
