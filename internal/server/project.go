package server

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	brainindexstate "github.com/ponchione/sodoryard/internal/brain/indexstate"
	"github.com/ponchione/sodoryard/internal/config"
	appdb "github.com/ponchione/sodoryard/internal/db"
	"github.com/ponchione/sodoryard/internal/langutil"
	"github.com/ponchione/sodoryard/internal/pathglob"
	"github.com/ponchione/sodoryard/internal/pathguard"
	"github.com/ponchione/sodoryard/internal/projectmemory"
)

// ProjectHandler serves project info, file tree, and file content endpoints.
type ProjectHandler struct {
	cfg           *config.Config
	logger        *slog.Logger
	memoryBackend any

	langOnce sync.Once
	langVal  string // cached primary language
}

// NewProjectHandler creates a handler and registers routes on the server.
func NewProjectHandler(s *Server, cfg *config.Config, logger *slog.Logger, memoryBackend ...any) *ProjectHandler {
	var backend any
	if len(memoryBackend) > 0 {
		backend = memoryBackend[0]
	}
	h := &ProjectHandler{cfg: cfg, logger: logger, memoryBackend: backend}

	s.HandleFunc("GET /api/project", h.handleProject)
	s.HandleFunc("GET /api/project/tree", h.handleTree)
	s.HandleFunc("GET /api/project/file", h.handleFile)

	return h
}

// ── GET /api/project ─────────────────────────────────────────────────

type projectInfoResponse struct {
	ID                string              `json:"id"`
	RootPath          string              `json:"root_path"`
	Language          string              `json:"language,omitempty"`
	Name              string              `json:"name"`
	LastIndexedAt     string              `json:"last_indexed_at,omitempty"`
	LastIndexedCommit string              `json:"last_indexed_commit,omitempty"`
	BrainIndex        *brainIndexResponse `json:"brain_index,omitempty"`
}

type brainIndexResponse struct {
	Status        string `json:"status"`
	LastIndexedAt string `json:"last_indexed_at,omitempty"`
	StaleSince    string `json:"stale_since,omitempty"`
	StaleReason   string `json:"stale_reason,omitempty"`
}

func (h *ProjectHandler) handleProject(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(h.cfg.ProjectRoot)

	h.langOnce.Do(func() {
		h.langVal = detectPrimaryLanguage(h.cfg.ProjectRoot)
		h.logger.Info("cached primary language", "language", h.langVal)
	})

	lastIndexedAt, lastIndexedCommit := h.loadProjectIndexMetadata(r.Context())
	brainIndex := h.loadBrainIndexState(r.Context())

	writeJSON(w, http.StatusOK, projectInfoResponse{
		ID:                h.cfg.ProjectRoot,
		RootPath:          h.cfg.ProjectRoot,
		Language:          h.langVal,
		Name:              name,
		LastIndexedAt:     lastIndexedAt,
		LastIndexedCommit: lastIndexedCommit,
		BrainIndex:        brainIndex,
	})
}

// ── GET /api/project/tree ────────────────────────────────────────────

type treeNode struct {
	Name     string     `json:"name"`
	Type     string     `json:"type"` // "dir" or "file"
	Children []treeNode `json:"children,omitempty"`
}

func (h *ProjectHandler) handleTree(w http.ResponseWriter, r *http.Request) {
	maxDepth := 3
	if d := r.URL.Query().Get("depth"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed >= 1 && parsed <= 10 {
			maxDepth = parsed
		}
	}

	root := h.cfg.ProjectRoot
	excludes := h.cfg.Index.Exclude

	tree := buildTree(root, root, excludes, 0, maxDepth)
	writeJSON(w, http.StatusOK, tree)
}

// ── GET /api/project/file ────────────────────────────────────────────

type fileResponse struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	Language  string `json:"language"`
	LineCount int    `json:"line_count"`
}

func (h *ProjectHandler) handleFile(w http.ResponseWriter, r *http.Request) {
	relPath := r.URL.Query().Get("path")
	if relPath == "" {
		writeError(w, http.StatusBadRequest, "query parameter 'path' is required")
		return
	}

	absPath, err := pathguard.Resolve(h.cfg.ProjectRoot, relPath)
	if errors.Is(err, pathguard.ErrAbsolutePath) || errors.Is(err, pathguard.ErrEscapesRoot) {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	if err != nil {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}

	info, err := os.Stat(absPath)
	if err != nil {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}
	if info.IsDir() {
		writeError(w, http.StatusBadRequest, "path is a directory")
		return
	}
	if info.Size() > 1<<20 { // 1MB limit
		writeError(w, http.StatusRequestEntityTooLarge, "file too large (>1MB)")
		return
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		h.logger.Error("read file", "error", err, "path", absPath)
		writeError(w, http.StatusInternalServerError, "failed to read file")
		return
	}

	lang := langFromExtension(filepath.Ext(relPath))
	lines := strings.Count(string(data), "\n") + 1

	writeJSON(w, http.StatusOK, fileResponse{
		Path:      relPath,
		Content:   string(data),
		Language:  lang,
		LineCount: lines,
	})
}

// ── Helpers ──────────────────────────────────────────────────────────

func (h *ProjectHandler) loadProjectIndexMetadata(ctx context.Context) (lastIndexedAt string, lastIndexedCommit string) {
	if ctx == nil {
		ctx = context.Background()
	}
	if h.cfg != nil && h.cfg.Memory.Backend == "shunter" {
		return h.loadShunterProjectIndexMetadata(ctx)
	}
	databasePath := h.cfg.DatabasePath()
	if strings.TrimSpace(databasePath) == "" {
		return "", ""
	}
	if _, err := os.Stat(databasePath); err != nil {
		return "", ""
	}
	database, err := appdb.OpenDB(ctx, databasePath)
	if err != nil {
		h.logger.Debug("open project metadata db", "error", err, "path", databasePath)
		return "", ""
	}
	defer database.Close()

	var commit sql.NullString
	var indexedAt sql.NullString
	err = database.QueryRowContext(ctx, `SELECT last_indexed_commit, last_indexed_at FROM projects WHERE id = ?`, h.cfg.ProjectRoot).Scan(&commit, &indexedAt)
	if err != nil {
		if err != sql.ErrNoRows {
			h.logger.Debug("load project index metadata", "error", err, "project_id", h.cfg.ProjectRoot)
		}
		return "", ""
	}
	if indexedAt.Valid {
		lastIndexedAt = indexedAt.String
	}
	if commit.Valid {
		lastIndexedCommit = commit.String
	}
	return lastIndexedAt, lastIndexedCommit
}

func (h *ProjectHandler) loadShunterProjectIndexMetadata(ctx context.Context) (lastIndexedAt string, lastIndexedCommit string) {
	backend, cleanup, err := h.projectMemoryIndexBackend(ctx)
	if err != nil {
		h.logger.Debug("open project memory index backend", "error", err)
		return "", ""
	}
	defer cleanup()
	reader, ok := backend.(shunterCodeIndexStateReader)
	if !ok || reader == nil {
		h.logger.Debug("project memory code index reader unavailable")
		return "", ""
	}
	state, found, err := reader.ReadCodeIndexState(ctx)
	if err != nil {
		h.logger.Debug("load Shunter code index metadata", "error", err)
		return "", ""
	}
	if !found {
		return "", ""
	}
	return formatProjectMemoryUnixUS(state.LastIndexedAtUS), state.LastIndexedCommit
}

func (h *ProjectHandler) loadBrainIndexState(ctx context.Context) *brainIndexResponse {
	if h.cfg == nil || !h.cfg.Brain.Enabled {
		return nil
	}
	if h.cfg.Brain.Backend == "shunter" {
		return h.loadShunterBrainIndexState(ctx)
	}
	state, err := brainindexstate.Load(h.cfg.ProjectRoot)
	if err != nil {
		h.logger.Debug("load brain index state", "error", err, "project_root", h.cfg.ProjectRoot)
		return &brainIndexResponse{Status: brainindexstate.StatusNeverIndexed}
	}
	return &brainIndexResponse{
		Status:        state.Status,
		LastIndexedAt: state.LastIndexedAt,
		StaleSince:    state.StaleSince,
		StaleReason:   state.StaleReason,
	}
}

func (h *ProjectHandler) loadShunterBrainIndexState(ctx context.Context) *brainIndexResponse {
	if ctx == nil {
		ctx = context.Background()
	}
	backend, cleanup, err := h.projectMemoryIndexBackend(ctx)
	if err != nil {
		h.logger.Debug("open project memory index backend", "error", err)
		return &brainIndexResponse{Status: brainindexstate.StatusNeverIndexed}
	}
	defer cleanup()
	reader, ok := backend.(shunterBrainIndexStateReader)
	if !ok || reader == nil {
		h.logger.Debug("project memory brain index reader unavailable")
		return &brainIndexResponse{Status: brainindexstate.StatusNeverIndexed}
	}
	state, found, err := reader.ReadBrainIndexState(ctx)
	if err != nil {
		h.logger.Debug("load Shunter brain index state", "error", err)
		return &brainIndexResponse{Status: brainindexstate.StatusNeverIndexed}
	}
	return brainIndexResponseFromShunterState(state, found)
}

type shunterCodeIndexStateReader interface {
	ReadCodeIndexState(context.Context) (projectmemory.CodeIndexState, bool, error)
}

type shunterBrainIndexStateReader interface {
	ReadBrainIndexState(context.Context) (projectmemory.BrainIndexState, bool, error)
}

func (h *ProjectHandler) projectMemoryIndexBackend(ctx context.Context) (any, func(), error) {
	if h.memoryBackend != nil {
		return h.memoryBackend, func() {}, nil
	}
	if endpoint := strings.TrimSpace(os.Getenv(projectmemory.EnvMemoryEndpoint)); endpoint != "" {
		client, err := projectmemory.DialBrainBackend(endpoint)
		if err != nil {
			return nil, func() {}, err
		}
		return client, func() { _ = client.Close() }, nil
	}
	if h.cfg == nil {
		return nil, func() {}, errors.New("project config is required")
	}
	backend, err := projectmemory.OpenBrainBackend(ctx, projectmemory.Config{
		DataDir:    h.cfg.MemoryShunterDataDir(),
		DurableAck: h.cfg.Memory.DurableAck,
	})
	if err != nil {
		return nil, func() {}, err
	}
	return backend, func() { _ = backend.Close() }, nil
}

func brainIndexResponseFromShunterState(state projectmemory.BrainIndexState, found bool) *brainIndexResponse {
	resp := &brainIndexResponse{Status: brainindexstate.StatusNeverIndexed}
	if !found {
		return resp
	}
	if state.LastIndexedAtUS > 0 {
		resp.Status = brainindexstate.StatusClean
		resp.LastIndexedAt = formatProjectMemoryUnixUS(state.LastIndexedAtUS)
	}
	if state.Dirty {
		resp.Status = brainindexstate.StatusStale
		resp.StaleSince = formatProjectMemoryUnixUS(state.DirtySinceUS)
		resp.StaleReason = state.DirtyReason
	}
	return resp
}

func formatProjectMemoryUnixUS(value uint64) string {
	if value == 0 {
		return ""
	}
	return time.UnixMicro(int64(value)).UTC().Format(time.RFC3339)
}

func buildTree(root, dir string, excludes []string, depth, maxDepth int) treeNode {
	name := filepath.Base(dir)
	if depth == 0 {
		name = "."
	}

	node := treeNode{Name: name, Type: "dir"}

	if depth >= maxDepth {
		return node
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return node
	}

	for _, entry := range entries {
		entryName := entry.Name()
		relPath, _ := filepath.Rel(root, filepath.Join(dir, entryName))

		if shouldExclude(relPath, entryName, excludes) {
			continue
		}

		if entry.IsDir() {
			child := buildTree(root, filepath.Join(dir, entryName), excludes, depth+1, maxDepth)
			node.Children = append(node.Children, child)
		} else {
			node.Children = append(node.Children, treeNode{Name: entryName, Type: "file"})
		}
	}

	return node
}

func shouldExclude(relPath, name string, excludes []string) bool {
	// Always exclude hidden dirs/files starting with .
	if strings.HasPrefix(name, ".") {
		return true
	}

	for _, pattern := range excludes {
		if pathglob.Match(pattern, relPath) || pathglob.Match(pattern, name) {
			return true
		}
	}
	return false
}

func detectPrimaryLanguage(root string) string {
	// Count extensions from the include patterns.
	counts := map[string]int{}
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(path)
		if ext != "" {
			counts[ext]++
		}
		return nil
	})

	// Find the most common code extension.
	bestLang := ""
	bestCount := 0
	for ext, count := range counts {
		lang, ok := langutil.FromExtension(ext)
		if ext == ".tsx" {
			lang = "typescript"
			ok = true
		}
		if ext == ".jsx" {
			lang = "javascript"
			ok = true
		}
		if ok && count > bestCount {
			bestCount = count
			bestLang = lang
		}
	}
	return bestLang
}

func langFromExtension(ext string) string {
	return langutil.FromExtensionOr(ext, "text")
}
