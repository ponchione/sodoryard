package context

import stdctx "context"

// NoopConventionSource is the v0.1 placeholder convention loader.
//
// Slice 4 keeps convention loading behind an interface, but the repo does not
// yet have a real Layer 1 convention cache implementation to inject here.
type NoopConventionSource struct{}

// Load returns no conventions and no error.
func (NoopConventionSource) Load(stdctx.Context) (string, error) {
	return "", nil
}
