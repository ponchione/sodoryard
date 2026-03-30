package conversation

import (
	"path/filepath"
	"sort"
	"strings"
	"sync"

	contextpkg "github.com/ponchione/sirtopham/internal/context"
)

// SeenFiles tracks file paths that have appeared during the current session.
//
// It is session-scoped, in-memory state used by Layer 3 context assembly to
// annotate files that were already shown in earlier turns.
type SeenFiles struct {
	mu    sync.RWMutex
	paths map[string]int
}

var _ contextpkg.SeenFileLookup = (*SeenFiles)(nil)

// NewSeenFiles constructs an empty seen-files tracker.
func NewSeenFiles() *SeenFiles {
	return &SeenFiles{paths: make(map[string]int)}
}

// Add records a file path and the first turn in which it was seen.
func (s *SeenFiles) Add(path string, turnNumber int) {
	if s == nil {
		return
	}
	path = normalizeSeenPath(path)
	if path == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.paths[path]; exists {
		return
	}
	s.paths[path] = turnNumber
}

// Contains reports whether a file path has been seen and, if so, the turn in
// which it was first recorded.
func (s *SeenFiles) Contains(path string) (bool, int) {
	if s == nil {
		return false, 0
	}
	path = normalizeSeenPath(path)
	if path == "" {
		return false, 0
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	turn, ok := s.paths[path]
	return ok, turn
}

// Paths returns all normalized tracked paths in sorted order.
func (s *SeenFiles) Paths() []string {
	if s == nil {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	paths := make([]string, 0, len(s.paths))
	for path := range s.paths {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

// Count returns the number of unique tracked file paths.
func (s *SeenFiles) Count() int {
	if s == nil {
		return 0
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.paths)
}

func normalizeSeenPath(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	return filepath.Clean(path)
}
