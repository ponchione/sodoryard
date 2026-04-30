package graph

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/ponchione/sodoryard/internal/codeintel"
	appdb "github.com/ponchione/sodoryard/internal/db"
)

const graphDDL = `
CREATE TABLE IF NOT EXISTS graph_meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS symbols (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    kind       TEXT NOT NULL,
    language   TEXT NOT NULL,
    package    TEXT,
    file_path  TEXT NOT NULL,
    line_start INTEGER NOT NULL,
    line_end   INTEGER NOT NULL,
    signature  TEXT,
    exported   INTEGER NOT NULL DEFAULT 0,
    receiver   TEXT
);

CREATE INDEX IF NOT EXISTS idx_symbols_file ON symbols(file_path);
CREATE INDEX IF NOT EXISTS idx_symbols_name ON symbols(name);
CREATE INDEX IF NOT EXISTS idx_symbols_kind ON symbols(kind);
CREATE INDEX IF NOT EXISTS idx_symbols_package ON symbols(package);

CREATE TABLE IF NOT EXISTS edges (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    source_id   TEXT NOT NULL REFERENCES symbols(id) ON DELETE CASCADE,
    target_id   TEXT NOT NULL,
    edge_type   TEXT NOT NULL,
    confidence  REAL NOT NULL DEFAULT 1.0,
    source_line INTEGER,
    metadata    TEXT
);

CREATE INDEX IF NOT EXISTS idx_edges_source ON edges(source_id);
CREATE INDEX IF NOT EXISTS idx_edges_target ON edges(target_id);
CREATE INDEX IF NOT EXISTS idx_edges_type ON edges(edge_type);

CREATE TABLE IF NOT EXISTS boundary_symbols (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    kind       TEXT NOT NULL,
    language   TEXT NOT NULL,
    package    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS chunk_mapping (
    symbol_id TEXT NOT NULL REFERENCES symbols(id) ON DELETE CASCADE,
    chunk_id  TEXT NOT NULL,
    UNIQUE(symbol_id, chunk_id)
);
`

// Store manages the graph SQLite database.
type Store struct {
	db *sql.DB
}

// NewStore opens or creates a graph database at the given DSN.
func NewStore(dsn string) (*Store, error) {
	db, err := sql.Open(appdb.DriverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("open graph db: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping graph db: %w", err)
	}

	for _, pragma := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA journal_mode = WAL",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("pragma %q: %w", pragma, err)
		}
	}

	if _, err := db.Exec(graphDDL); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply graph schema: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// BlastRadius implements codeintel.GraphStore. It resolves the target symbol,
// then performs recursive CTE queries for upstream and downstream traversal.
func (s *Store) BlastRadius(ctx context.Context, query codeintel.GraphQuery) (*codeintel.BlastRadiusResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	maxDepth := query.MaxDepth
	if maxDepth <= 0 {
		maxDepth = 3
	}
	maxNodes := query.MaxNodes
	if maxNodes <= 0 {
		maxNodes = 30
	}

	target, err := s.resolveTarget(ctx, query.Symbol)
	if err != nil {
		return nil, fmt.Errorf("resolve target %q: %w", query.Symbol, err)
	}

	upstream, err := s.blastUpstream(ctx, target.ID, maxDepth, maxNodes)
	if err != nil {
		return nil, fmt.Errorf("upstream blast: %w", err)
	}

	downstream, err := s.blastDownstream(ctx, target.ID, maxDepth, maxNodes)
	if err != nil {
		return nil, fmt.Errorf("downstream blast: %w", err)
	}

	ifaces, err := s.getInterfaces(ctx, target.ID)
	if err != nil {
		return nil, fmt.Errorf("interfaces: %w", err)
	}

	return &codeintel.BlastRadiusResult{
		Upstream:   toGraphNodes(upstream),
		Downstream: toGraphNodes(downstream),
		Interfaces: toGraphNodes(ifaces),
	}, nil
}

// InsertSymbols batch-inserts symbols.
func (s *Store) InsertSymbols(symbols []Symbol) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO symbols
		(id, name, kind, language, package, file_path, line_start, line_end, signature, exported, receiver)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, sym := range symbols {
		exported := 0
		if sym.Exported {
			exported = 1
		}
		if _, err := stmt.Exec(sym.ID, sym.Name, sym.Kind, sym.Language, sym.Package,
			sym.FilePath, sym.LineStart, sym.LineEnd, sym.Signature, exported, sym.Receiver); err != nil {
			return fmt.Errorf("insert symbol %s: %w", sym.ID, err)
		}
	}

	return tx.Commit()
}

// InsertEdges batch-inserts edges.
func (s *Store) InsertEdges(edges []Edge) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO edges
		(source_id, target_id, edge_type, confidence, source_line, metadata)
		VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, e := range edges {
		if _, err := stmt.Exec(e.SourceID, e.TargetID, e.EdgeType, e.Confidence, e.SourceLine, e.Metadata); err != nil {
			return fmt.Errorf("insert edge %s->%s: %w", e.SourceID, e.TargetID, err)
		}
	}

	return tx.Commit()
}

// InsertBoundarySymbols batch-inserts boundary symbols.
func (s *Store) InsertBoundarySymbols(bounds []BoundarySymbol) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO boundary_symbols
		(id, name, kind, language, package) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, b := range bounds {
		if _, err := stmt.Exec(b.ID, b.Name, b.Kind, b.Language, b.Package); err != nil {
			return fmt.Errorf("insert boundary symbol %s: %w", b.ID, err)
		}
	}

	return tx.Commit()
}

// InsertChunkMappings links a symbol to LanceDB chunk IDs.
func (s *Store) InsertChunkMappings(symbolID string, chunkIDs []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO chunk_mapping (symbol_id, chunk_id) VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, cid := range chunkIDs {
		if _, err := stmt.Exec(symbolID, cid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetSymbol retrieves a single symbol by ID.
func (s *Store) GetSymbol(id string) (*Symbol, error) {
	return s.getSymbol(context.Background(), id)
}

func (s *Store) getSymbol(ctx context.Context, id string) (*Symbol, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, name, kind, language, package, file_path,
		line_start, line_end, signature, exported, receiver
		FROM symbols WHERE id = ?`, id)
	return scanSymbol(row)
}

// GetSymbolsByFile returns all symbols in the given file.
func (s *Store) GetSymbolsByFile(filePath string) ([]Symbol, error) {
	rows, err := s.db.Query(`SELECT id, name, kind, language, package, file_path,
		line_start, line_end, signature, exported, receiver
		FROM symbols WHERE file_path = ? ORDER BY line_start`, filePath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSymbols(rows)
}

// GetSymbolsByName returns all symbols with the given name.
func (s *Store) GetSymbolsByName(name string) ([]Symbol, error) {
	return s.getSymbolsByName(context.Background(), name)
}

func (s *Store) getSymbolsByName(ctx context.Context, name string) ([]Symbol, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, kind, language, package, file_path,
		line_start, line_end, signature, exported, receiver
		FROM symbols WHERE name = ? ORDER BY file_path, line_start`, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSymbols(rows)
}

// GetEdgesFrom returns all edges originating from the given symbol.
func (s *Store) GetEdgesFrom(symbolID string) ([]Edge, error) {
	rows, err := s.db.Query(`SELECT source_id, target_id, edge_type, confidence, source_line, metadata
		FROM edges WHERE source_id = ?`, symbolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEdges(rows)
}

// GetEdgesTo returns all edges targeting the given symbol.
func (s *Store) GetEdgesTo(symbolID string) ([]Edge, error) {
	rows, err := s.db.Query(`SELECT source_id, target_id, edge_type, confidence, source_line, metadata
		FROM edges WHERE target_id = ?`, symbolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEdges(rows)
}

// GetChunkMappingsForSymbol returns LanceDB chunk IDs for a symbol.
func (s *Store) GetChunkMappingsForSymbol(symbolID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT chunk_id FROM chunk_mapping WHERE symbol_id = ?`, symbolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// SetMeta sets a key-value pair in graph_meta.
func (s *Store) SetMeta(key, value string) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO graph_meta (key, value) VALUES (?, ?)`, key, value)
	return err
}

// StoreAnalysisResult stores a complete analysis result.
func (s *Store) StoreAnalysisResult(result *AnalysisResult) error {
	if err := s.InsertSymbols(result.Symbols); err != nil {
		return fmt.Errorf("insert symbols: %w", err)
	}
	if err := s.InsertBoundarySymbols(result.BoundarySymbols); err != nil {
		return fmt.Errorf("insert boundary symbols: %w", err)
	}
	if err := s.InsertEdges(result.Edges); err != nil {
		return fmt.Errorf("insert edges: %w", err)
	}
	return nil
}

// DropAndRecreate drops all graph tables and recreates the schema.
func (s *Store) DropAndRecreate() error {
	drops := []string{
		"DROP TABLE IF EXISTS chunk_mapping",
		"DROP TABLE IF EXISTS edges",
		"DROP TABLE IF EXISTS boundary_symbols",
		"DROP TABLE IF EXISTS symbols",
		"DROP TABLE IF EXISTS graph_meta",
	}
	for _, stmt := range drops {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("drop: %w", err)
		}
	}
	if _, err := s.db.Exec(graphDDL); err != nil {
		return fmt.Errorf("recreate schema: %w", err)
	}
	return nil
}

// --- blast radius ---

func (s *Store) resolveTarget(ctx context.Context, target string) (*Symbol, error) {
	sym, err := s.getSymbol(ctx, target)
	if err == nil {
		return sym, nil
	}
	if err != sql.ErrNoRows {
		return nil, err
	}

	syms, err := s.getSymbolsByName(ctx, target)
	if err != nil {
		return nil, err
	}
	if len(syms) == 0 {
		return nil, fmt.Errorf("symbol not found: %s", target)
	}
	return &syms[0], nil
}

type blastNode struct {
	sym      Symbol
	depth    int
	edgeType string
	conf     float64
	path     []string
}

func (s *Store) blastUpstream(ctx context.Context, targetID string, maxDepth, budget int) ([]blastNode, error) {
	query := `
		WITH RECURSIVE blast(symbol_id, depth, confidence, path, edge_type) AS (
			SELECT e.source_id, 1, e.confidence, e.source_id, e.edge_type
			FROM edges e
			WHERE e.target_id = ?

			UNION ALL

			SELECT e.source_id, b.depth + 1, e.confidence,
			       b.path || '>' || e.source_id, e.edge_type
			FROM edges e
			JOIN blast b ON e.target_id = b.symbol_id
			WHERE b.depth < ?
			  AND e.source_id NOT IN (SELECT id FROM boundary_symbols)
			  AND instr('>' || b.path || '>', '>' || e.source_id || '>') = 0
			  AND e.source_id != ?
		)
		SELECT s.id, s.name, s.kind, s.language, s.package, s.file_path,
		       s.line_start, s.line_end, s.signature, s.exported, s.receiver,
		       b.depth, b.confidence, b.path, b.edge_type
		FROM blast b
		JOIN symbols s ON b.symbol_id = s.id
		GROUP BY s.id
		HAVING b.depth = MIN(b.depth)
		ORDER BY b.depth ASC, b.confidence DESC
		LIMIT ?`

	rows, err := s.db.QueryContext(ctx, query, targetID, maxDepth, targetID, budget)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanBlastNodes(rows)
}

func (s *Store) blastDownstream(ctx context.Context, targetID string, maxDepth, budget int) ([]blastNode, error) {
	query := `
		WITH RECURSIVE blast(symbol_id, depth, confidence, path, edge_type) AS (
			SELECT e.target_id, 1, e.confidence, e.target_id, e.edge_type
			FROM edges e
			WHERE e.source_id = ?
			  AND e.target_id NOT IN (SELECT id FROM boundary_symbols)
			  AND e.edge_type != 'IMPLEMENTS'

			UNION ALL

			SELECT e.target_id, b.depth + 1, e.confidence,
			       b.path || '>' || e.target_id, e.edge_type
			FROM edges e
			JOIN blast b ON e.source_id = b.symbol_id
			WHERE b.depth < ?
			  AND e.target_id NOT IN (SELECT id FROM boundary_symbols)
			  AND instr('>' || b.path || '>', '>' || e.target_id || '>') = 0
			  AND e.target_id != ?
			  AND e.edge_type != 'IMPLEMENTS'
		)
		SELECT s.id, s.name, s.kind, s.language, s.package, s.file_path,
		       s.line_start, s.line_end, s.signature, s.exported, s.receiver,
		       b.depth, b.confidence, b.path, b.edge_type
		FROM blast b
		JOIN symbols s ON b.symbol_id = s.id
		GROUP BY s.id
		HAVING b.depth = MIN(b.depth)
		ORDER BY b.depth ASC, b.confidence DESC
		LIMIT ?`

	rows, err := s.db.QueryContext(ctx, query, targetID, maxDepth, targetID, budget)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanBlastNodes(rows)
}

func (s *Store) getInterfaces(ctx context.Context, symbolID string) ([]blastNode, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.id, s.name, s.kind, s.language, s.package, s.file_path,
		       s.line_start, s.line_end, s.signature, s.exported, s.receiver
		FROM edges e
		JOIN symbols s ON e.target_id = s.id
		WHERE e.source_id = ? AND e.edge_type = 'IMPLEMENTS'`, symbolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	syms, err := scanSymbols(rows)
	if err != nil {
		return nil, err
	}

	nodes := make([]blastNode, len(syms))
	for i, sym := range syms {
		nodes[i] = blastNode{sym: sym, edgeType: "IMPLEMENTS"}
	}
	return nodes, nil
}

// --- scanners ---

func scanSymbol(row *sql.Row) (*Symbol, error) {
	var sym Symbol
	var exported int
	var pkg, sig, recv sql.NullString
	err := row.Scan(&sym.ID, &sym.Name, &sym.Kind, &sym.Language, &pkg,
		&sym.FilePath, &sym.LineStart, &sym.LineEnd, &sig, &exported, &recv)
	if err != nil {
		return nil, err
	}
	sym.Package = pkg.String
	sym.Signature = sig.String
	sym.Receiver = recv.String
	sym.Exported = exported == 1
	return &sym, nil
}

func scanSymbols(rows *sql.Rows) ([]Symbol, error) {
	var syms []Symbol
	for rows.Next() {
		var sym Symbol
		var exported int
		var pkg, sig, recv sql.NullString
		if err := rows.Scan(&sym.ID, &sym.Name, &sym.Kind, &sym.Language, &pkg,
			&sym.FilePath, &sym.LineStart, &sym.LineEnd, &sig, &exported, &recv); err != nil {
			return nil, err
		}
		sym.Package = pkg.String
		sym.Signature = sig.String
		sym.Receiver = recv.String
		sym.Exported = exported == 1
		syms = append(syms, sym)
	}
	return syms, rows.Err()
}

func scanEdges(rows *sql.Rows) ([]Edge, error) {
	var edges []Edge
	for rows.Next() {
		var e Edge
		var sourceLine sql.NullInt64
		var metadata sql.NullString
		if err := rows.Scan(&e.SourceID, &e.TargetID, &e.EdgeType, &e.Confidence, &sourceLine, &metadata); err != nil {
			return nil, err
		}
		e.SourceLine = int(sourceLine.Int64)
		e.Metadata = metadata.String
		edges = append(edges, e)
	}
	return edges, rows.Err()
}

func scanBlastNodes(rows *sql.Rows) ([]blastNode, error) {
	var nodes []blastNode
	for rows.Next() {
		var sym Symbol
		var node blastNode
		var exported int
		var pkg, sig, recv, pathStr sql.NullString

		if err := rows.Scan(&sym.ID, &sym.Name, &sym.Kind, &sym.Language, &pkg,
			&sym.FilePath, &sym.LineStart, &sym.LineEnd, &sig, &exported, &recv,
			&node.depth, &node.conf, &pathStr, &node.edgeType); err != nil {
			return nil, err
		}

		sym.Package = pkg.String
		sym.Signature = sig.String
		sym.Receiver = recv.String
		sym.Exported = exported == 1
		node.sym = sym

		if pathStr.Valid {
			node.path = strings.Split(pathStr.String, ">")
		}

		nodes = append(nodes, node)
	}
	return nodes, rows.Err()
}

// --- conversion to codeintel types ---

func toGraphNodes(nodes []blastNode) []codeintel.GraphNode {
	if len(nodes) == 0 {
		return []codeintel.GraphNode{}
	}
	out := make([]codeintel.GraphNode, len(nodes))
	for i, n := range nodes {
		out[i] = codeintel.GraphNode{
			Symbol:    n.sym.Name,
			FilePath:  n.sym.FilePath,
			Kind:      n.sym.Kind,
			Depth:     n.depth,
			LineStart: n.sym.LineStart,
			LineEnd:   n.sym.LineEnd,
		}
	}
	return out
}
