package chain

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

const SourceWriterLockName = "source_writer"

type localProjectLockStore struct {
	mu    sync.Mutex
	locks map[string]ProjectLock
}

func newLocalProjectLockStore() *localProjectLockStore {
	return &localProjectLockStore{locks: map[string]ProjectLock{}}
}

func (s *Store) AcquireProjectLock(ctx context.Context, params AcquireProjectLockParams) (ProjectLockAcquireResult, error) {
	if s != nil && s.memory != nil {
		return s.memory.AcquireProjectLock(ctx, params)
	}
	now := params.AcquiredAt.UTC()
	if now.IsZero() {
		now = s.now()
	}
	return s.localLocks().acquire(params, now)
}

func (s *Store) ReleaseProjectLock(ctx context.Context, params ReleaseProjectLockParams) error {
	if s != nil && s.memory != nil {
		return s.memory.ReleaseProjectLock(ctx, params)
	}
	return s.localLocks().release(params, false)
}

func (s *Store) HeartbeatProjectLock(ctx context.Context, params HeartbeatProjectLockParams) error {
	if s != nil && s.memory != nil {
		return s.memory.HeartbeatProjectLock(ctx, params)
	}
	return s.localLocks().heartbeat(params, s.now())
}

func (s *Store) ForceReleaseProjectLock(ctx context.Context, params ReleaseProjectLockParams) error {
	if s != nil && s.memory != nil {
		return s.memory.ForceReleaseProjectLock(ctx, params)
	}
	return s.localLocks().release(params, true)
}

func (s *Store) GetProjectLock(ctx context.Context, lockName string) (ProjectLock, bool, error) {
	if s != nil && s.memory != nil {
		return s.memory.GetProjectLock(ctx, lockName)
	}
	return s.localLocks().get(lockName)
}

func (s *Store) ListProjectLocks(ctx context.Context) ([]ProjectLock, error) {
	if s != nil && s.memory != nil {
		return s.memory.ListProjectLocks(ctx)
	}
	return s.localLocks().list(), nil
}

func (s *Store) localLocks() *localProjectLockStore {
	if s.locks == nil {
		s.locks = newLocalProjectLockStore()
	}
	return s.locks
}

func (s *Store) now() time.Time {
	if s == nil || s.clock == nil {
		return time.Now()
	}
	return s.clock().UTC()
}

func (l *localProjectLockStore) acquire(params AcquireProjectLockParams, now time.Time) (ProjectLockAcquireResult, error) {
	if l == nil {
		return ProjectLockAcquireResult{}, fmt.Errorf("project lock store is nil")
	}
	lockName := strings.TrimSpace(params.LockName)
	if lockName == "" {
		return ProjectLockAcquireResult{}, fmt.Errorf("lock name is required")
	}
	if strings.TrimSpace(params.OwnerChainID) == "" {
		return ProjectLockAcquireResult{}, fmt.Errorf("lock owner chain id is required")
	}
	if strings.TrimSpace(params.OwnerStepID) == "" {
		return ProjectLockAcquireResult{}, fmt.Errorf("lock owner step id is required")
	}
	if strings.TrimSpace(params.OwnerRole) == "" {
		return ProjectLockAcquireResult{}, fmt.Errorf("lock owner role is required")
	}
	expiresAt := params.ExpiresAt.UTC()
	if !expiresAt.After(now) {
		return ProjectLockAcquireResult{}, fmt.Errorf("lock expires_at must be in the future")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	current, found := l.locks[lockName]
	if found && sameProjectLockOwner(current, params.OwnerChainID, params.OwnerStepID) {
		current.OwnerRole = strings.TrimSpace(params.OwnerRole)
		current.HeartbeatAt = now
		current.ExpiresAt = expiresAt
		current.MetadataJSON = defaultJSON(params.MetadataJSON)
		l.locks[lockName] = current
		return ProjectLockAcquireResult{Lock: current}, nil
	}
	result := ProjectLockAcquireResult{}
	if found {
		if current.ExpiresAt.After(now) {
			return ProjectLockAcquireResult{}, fmt.Errorf("project lock %s is held by chain %s step %s role %s until %s", lockName, current.OwnerChainID, current.OwnerStepID, current.OwnerRole, current.ExpiresAt.Format(time.RFC3339))
		}
		result.ReplacedLockOwnerChainID = current.OwnerChainID
		result.ReplacedLockOwnerStepID = current.OwnerStepID
		result.ReplacedLockOwnerRole = current.OwnerRole
		result.ReplacedLockExpiredAt = current.ExpiresAt
	}
	lock := ProjectLock{
		LockName:     lockName,
		OwnerChainID: strings.TrimSpace(params.OwnerChainID),
		OwnerStepID:  strings.TrimSpace(params.OwnerStepID),
		OwnerRole:    strings.TrimSpace(params.OwnerRole),
		AcquiredAt:   now,
		HeartbeatAt:  now,
		ExpiresAt:    expiresAt,
		MetadataJSON: defaultJSON(params.MetadataJSON),
	}
	l.locks[lockName] = lock
	result.Lock = lock
	return result, nil
}

func (l *localProjectLockStore) release(params ReleaseProjectLockParams, force bool) error {
	if l == nil {
		return fmt.Errorf("project lock store is nil")
	}
	lockName := strings.TrimSpace(params.LockName)
	if lockName == "" {
		return fmt.Errorf("lock name is required")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	current, found := l.locks[lockName]
	if !found {
		return nil
	}
	if !force && !sameProjectLockOwner(current, params.OwnerChainID, params.OwnerStepID) {
		return fmt.Errorf("project lock %s is held by chain %s step %s role %s", lockName, current.OwnerChainID, current.OwnerStepID, current.OwnerRole)
	}
	delete(l.locks, lockName)
	return nil
}

func (l *localProjectLockStore) heartbeat(params HeartbeatProjectLockParams, now time.Time) error {
	if l == nil {
		return fmt.Errorf("project lock store is nil")
	}
	lockName := strings.TrimSpace(params.LockName)
	if lockName == "" {
		return fmt.Errorf("lock name is required")
	}
	expiresAt := params.ExpiresAt.UTC()
	if !expiresAt.After(now) {
		return fmt.Errorf("lock expires_at must be in the future")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	current, found := l.locks[lockName]
	if !found {
		return fmt.Errorf("project lock not found: %s", lockName)
	}
	if !sameProjectLockOwner(current, params.OwnerChainID, params.OwnerStepID) {
		return fmt.Errorf("project lock %s is held by chain %s step %s role %s", lockName, current.OwnerChainID, current.OwnerStepID, current.OwnerRole)
	}
	current.HeartbeatAt = now
	current.ExpiresAt = expiresAt
	l.locks[lockName] = current
	return nil
}

func (l *localProjectLockStore) get(lockName string) (ProjectLock, bool, error) {
	if l == nil {
		return ProjectLock{}, false, fmt.Errorf("project lock store is nil")
	}
	lockName = strings.TrimSpace(lockName)
	if lockName == "" {
		return ProjectLock{}, false, fmt.Errorf("lock name is required")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	lock, found := l.locks[lockName]
	return lock, found, nil
}

func (l *localProjectLockStore) list() []ProjectLock {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	locks := make([]ProjectLock, 0, len(l.locks))
	for _, lock := range l.locks {
		locks = append(locks, lock)
	}
	sort.Slice(locks, func(i, j int) bool {
		return locks[i].LockName < locks[j].LockName
	})
	return locks
}

func sameProjectLockOwner(lock ProjectLock, chainID string, stepID string) bool {
	return strings.TrimSpace(lock.OwnerChainID) == strings.TrimSpace(chainID) && strings.TrimSpace(lock.OwnerStepID) == strings.TrimSpace(stepID)
}

func defaultJSON(value string) string {
	if strings.TrimSpace(value) == "" {
		return "{}"
	}
	return value
}
