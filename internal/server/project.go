package server

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	brainindexstate "github.com/ponchione/sodoryard/internal/brain/indexstate"
	"github.com/ponchione/sodoryard/internal/config"
	appdb "github.com/ponchione/sodoryard/internal/db"
	"github.com/ponchione/sodoryard/internal/langutil"
	"github.com/ponchione/sodoryard/internal/pathglob"
)

// ProjectHandler serves project info, file tree, and file content endpoints.
type ProjectHandler struct {
	cfg    *config.Config
	logger *slog.Logger

	langOnce sync.Once
	langVal  string // cached primary language
}

// NewProjectHandler creates a handler and registers routes on the server.
func NewProjectHandler(s *Server, cfg *config.Config, logger *slog.Logger) *ProjectHandler {
	h := &ProjectHandler{cfg: cfg, logger: logger}

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

func (h *ProjectHandler) handleProject(w http.ResponseWriter, _ *http.Request) {
	name := filepath.Base(h.cfg.ProjectRoot)

	h.langOnce.Do(func() {
		h.langVal = detectPrimaryLanguage(h.cfg.ProjectRoot, h.cfg.Index.Include)
		h.logger.Info("cached primary language", "language", h.langVal)
	})

	lastIndexedAt, lastIndexedCommit := h.loadProjectIndexMetadata(context.Background())
	brainIndex := h.loadBrainIndexState()

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

	// Path traversal protection.
	if strings.Contains(relPath, "..") || filepath.IsAbs(relPath) {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}

	absPath := filepath.Join(h.cfg.ProjectRoot, relPath)

	// Ensure the resolved path is within project root.
	resolved, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}
	rootResolved, _ := filepath.EvalSymlinks(h.cfg.ProjectRoot)
	if !strings.HasPrefix(resolved, rootResolved+string(filepath.Separator)) && resolved != rootResolved {
		writeError(w, http.StatusBadRequest, "path outside project root")
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

func (h *ProjectHandler) loadBrainIndexState() *brainIndexResponse {
	if h.cfg == nil || !h.cfg.Brain.Enabled {
		return nil
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

func detectPrimaryLanguage(root string, includes []string) string {
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
