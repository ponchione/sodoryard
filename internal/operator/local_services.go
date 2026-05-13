package operator

import (
	"context"
	"strings"

	"github.com/ponchione/sodoryard/internal/localservices"
)

func (s *Service) LocalServicesStatus(ctx context.Context) (localservices.StackStatus, error) {
	cfg, err := s.config()
	if err != nil {
		return localservices.StackStatus{}, err
	}
	return s.localServicesManager().Status(ctx, cfg)
}

func (s *Service) LocalServicesUp(ctx context.Context) (localservices.StackStatus, error) {
	cfg, err := s.config()
	if err != nil {
		return localservices.StackStatus{}, err
	}
	return s.localServicesManager().EnsureUp(ctx, cfg)
}

func (s *Service) LocalServicesDown(ctx context.Context) (localservices.StackStatus, error) {
	cfg, err := s.config()
	if err != nil {
		return localservices.StackStatus{}, err
	}
	manager := s.localServicesManager()
	if !cfg.LocalServices.Enabled {
		return manager.Status(ctx, cfg)
	}
	if err := manager.Down(ctx, cfg); err != nil {
		return localservices.StackStatus{}, err
	}
	return manager.Status(ctx, cfg)
}

func (s *Service) LocalServicesLogs(ctx context.Context, tail int) (string, error) {
	cfg, err := s.config()
	if err != nil {
		return "", err
	}
	if !cfg.LocalServices.Enabled {
		return "", nil
	}
	logs, err := s.localServicesManager().Logs(ctx, cfg, tail)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(logs, "\n"), nil
}

func (s *Service) localServicesManager() LocalServicesManager {
	if s != nil && s.localServices != nil {
		return s.localServices
	}
	return localservices.NewManager(nil)
}
