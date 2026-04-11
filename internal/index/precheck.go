package index

import (
	"context"
	"fmt"

	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/localservices"
)

var newLocalServicesManager = func() *localservices.Manager {
	return localservices.NewManager(nil)
}

func runIndexPrecheck(ctx context.Context, cfg *appconfig.Config) error {
	manager := newLocalServicesManager()
	_, err := manager.EnsureUp(ctx, cfg)
	if err == nil {
		return nil
	}
	return fmt.Errorf("index precheck: %w", err)
}
