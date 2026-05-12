package operator

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ponchione/sodoryard/internal/chain"
)

func (s *Service) ListProjectLocks(ctx context.Context) ([]ProjectLockView, error) {
	store, err := s.store()
	if err != nil {
		return nil, err
	}
	locks, err := store.ListProjectLocks(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	views := make([]ProjectLockView, 0, len(locks))
	for _, lock := range locks {
		views = append(views, projectLockView(lock, now))
	}
	return views, nil
}

func (s *Service) ForceReleaseProjectLock(ctx context.Context, lockName string, reason string) (ProjectLockForceReleaseResult, error) {
	lockName = strings.TrimSpace(lockName)
	if lockName == "" {
		return ProjectLockForceReleaseResult{}, fmt.Errorf("lock name is required")
	}
	store, err := s.store()
	if err != nil {
		return ProjectLockForceReleaseResult{}, err
	}
	lock, found, err := store.GetProjectLock(ctx, lockName)
	if err != nil {
		return ProjectLockForceReleaseResult{}, err
	}
	if !found {
		return ProjectLockForceReleaseResult{LockName: lockName, Message: fmt.Sprintf("lock %s is not held", lockName)}, nil
	}
	if err := store.ForceReleaseProjectLock(ctx, chain.ReleaseProjectLockParams{LockName: lockName}); err != nil {
		return ProjectLockForceReleaseResult{}, err
	}
	payload := map[string]any{
		"lock_name":          lock.LockName,
		"operator_initiated": true,
		"owner_chain_id":     lock.OwnerChainID,
		"owner_step_id":      lock.OwnerStepID,
		"owner_role":         lock.OwnerRole,
		"reason":             strings.TrimSpace(reason),
	}
	if err := store.LogEvent(ctx, lock.OwnerChainID, lock.OwnerStepID, chain.EventSourceWriterLockForceReleased, payload); err != nil {
		return ProjectLockForceReleaseResult{}, err
	}
	return ProjectLockForceReleaseResult{
		LockName:     lock.LockName,
		Released:     true,
		OwnerChainID: lock.OwnerChainID,
		OwnerStepID:  lock.OwnerStepID,
		OwnerRole:    lock.OwnerRole,
		Message:      fmt.Sprintf("lock %s force released", lock.LockName),
	}, nil
}

func projectLockView(lock chain.ProjectLock, now time.Time) ProjectLockView {
	return ProjectLockView{
		LockName:     lock.LockName,
		OwnerChainID: lock.OwnerChainID,
		OwnerStepID:  lock.OwnerStepID,
		OwnerRole:    lock.OwnerRole,
		AcquiredAt:   lock.AcquiredAt,
		HeartbeatAt:  lock.HeartbeatAt,
		ExpiresAt:    lock.ExpiresAt,
		Stale:        !lock.ExpiresAt.IsZero() && !lock.ExpiresAt.After(now),
		MetadataJSON: lock.MetadataJSON,
	}
}
