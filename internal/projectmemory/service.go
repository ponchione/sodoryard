package projectmemory

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/rpc"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type RPCConfig struct {
	Transport string
	Path      string
}

type RPCServer struct {
	listener net.Listener
	server   *rpc.Server
	backend  *BrainBackend
	path     string
	done     chan struct{}
	close    sync.Once
}

func StartRPCServer(ctx context.Context, cfg RPCConfig, backend *BrainBackend) (*RPCServer, error) {
	if backend == nil {
		return nil, fmt.Errorf("project memory RPC backend is required")
	}
	transport := strings.TrimSpace(cfg.Transport)
	if transport == "" {
		transport = "unix"
	}
	if transport != "unix" {
		return nil, fmt.Errorf("unsupported project memory RPC transport %q", cfg.Transport)
	}
	path := strings.TrimSpace(cfg.Path)
	if path == "" {
		return nil, fmt.Errorf("project memory RPC path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir project memory RPC dir: %w", err)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("remove stale project memory RPC socket: %w", err)
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen project memory RPC: %w", err)
	}
	srv := &RPCServer{
		listener: listener,
		server:   rpc.NewServer(),
		backend:  backend,
		path:     path,
		done:     make(chan struct{}),
	}
	if err := srv.server.RegisterName("Brain", &brainRPCService{backend: backend}); err != nil {
		_ = listener.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("register project memory RPC service: %w", err)
	}
	go srv.serve()
	if ctx != nil {
		go func() {
			<-ctx.Done()
			_ = srv.Close()
		}()
	}
	return srv, nil
}

func (s *RPCServer) Close() error {
	if s == nil {
		return nil
	}
	var err error
	s.close.Do(func() {
		err = s.listener.Close()
		<-s.done
		if s.path != "" {
			_ = os.Remove(s.path)
		}
	})
	return err
}

func (s *RPCServer) serve() {
	defer close(s.done)
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.server.ServeConn(conn)
	}
}

type brainRPCService struct {
	backend *BrainBackend
}

type ReadDocumentRequest struct {
	Path string
}

type ReadDocumentResponse struct {
	Content string
}

type WriteDocumentRequest struct {
	Path    string
	Content string
}

type PatchDocumentRequest struct {
	Path      string
	Operation string
	Content   string
}

type SearchKeywordRequest struct {
	Query      string
	MaxResults int
}

type SearchKeywordResponse struct {
	Hits []SearchHit
}

type ListDocumentsRequest struct {
	Directory string
}

type ListDocumentsResponse struct {
	Paths []string
}

type ReadBrainIndexStateRequest struct {
	ProjectID string
}

type ReadBrainIndexStateResponse struct {
	State BrainIndexState
	Found bool
}

type MarkBrainIndexCleanRequest struct {
	ProjectID       string
	LastIndexedAtUS uint64
	MetadataJSON    string
}

type ReadCodeIndexStateRequest struct {
	ProjectID string
}

type ReadCodeIndexStateResponse struct {
	State CodeIndexState
	Found bool
}

type ListCodeFileIndexStatesRequest struct {
	ProjectID string
}

type ListCodeFileIndexStatesResponse struct {
	States []CodeFileIndexState
}

type MarkCodeIndexCleanRequest struct {
	ProjectID         string
	LastIndexedCommit string
	LastIndexedAtUS   uint64
	Files             []CodeFileIndexArg
	DeletedPaths      []string
	MetadataJSON      string
}

type EmptyResponse struct{}

func (s *brainRPCService) ReadDocument(req ReadDocumentRequest, resp *ReadDocumentResponse) error {
	content, err := s.backend.ReadDocument(context.Background(), req.Path)
	if err != nil {
		return err
	}
	resp.Content = content
	return nil
}

func (s *brainRPCService) WriteDocument(req WriteDocumentRequest, resp *EmptyResponse) error {
	return s.backend.WriteDocument(context.Background(), req.Path, req.Content)
}

func (s *brainRPCService) PatchDocument(req PatchDocumentRequest, resp *EmptyResponse) error {
	return s.backend.PatchDocument(context.Background(), req.Path, req.Operation, req.Content)
}

func (s *brainRPCService) SearchKeywordLimit(req SearchKeywordRequest, resp *SearchKeywordResponse) error {
	hits, err := s.backend.runtime.SearchDocuments(context.Background(), req.Query, req.MaxResults)
	if err != nil {
		return err
	}
	resp.Hits = hits
	return nil
}

func (s *brainRPCService) ListDocuments(req ListDocumentsRequest, resp *ListDocumentsResponse) error {
	paths, err := s.backend.ListDocuments(context.Background(), req.Directory)
	if err != nil {
		return err
	}
	resp.Paths = paths
	return nil
}

func (s *brainRPCService) ReadBrainIndexState(req ReadBrainIndexStateRequest, resp *ReadBrainIndexStateResponse) error {
	state, found, err := s.backend.ReadBrainIndexState(context.Background())
	if err != nil {
		return err
	}
	resp.State = state
	resp.Found = found
	return nil
}

func (s *brainRPCService) MarkBrainIndexClean(req MarkBrainIndexCleanRequest, resp *EmptyResponse) error {
	return s.backend.runtime.MarkBrainIndexClean(context.Background(), MarkBrainIndexCleanArgs{
		ProjectID:       firstNonEmpty(req.ProjectID, DefaultProjectID),
		LastIndexedAtUS: req.LastIndexedAtUS,
		MetadataJSON:    req.MetadataJSON,
	})
}

func (s *brainRPCService) ReadCodeIndexState(req ReadCodeIndexStateRequest, resp *ReadCodeIndexStateResponse) error {
	state, found, err := s.backend.ReadCodeIndexState(context.Background())
	if err != nil {
		return err
	}
	resp.State = state
	resp.Found = found
	return nil
}

func (s *brainRPCService) ListCodeFileIndexStates(req ListCodeFileIndexStatesRequest, resp *ListCodeFileIndexStatesResponse) error {
	states, err := s.backend.ListCodeFileIndexStates(context.Background())
	if err != nil {
		return err
	}
	resp.States = states
	return nil
}

func (s *brainRPCService) MarkCodeIndexClean(req MarkCodeIndexCleanRequest, resp *EmptyResponse) error {
	return s.backend.runtime.MarkCodeIndexClean(context.Background(), MarkCodeIndexCleanArgs{
		ProjectID:         firstNonEmpty(req.ProjectID, DefaultProjectID),
		LastIndexedCommit: req.LastIndexedCommit,
		LastIndexedAtUS:   req.LastIndexedAtUS,
		Files:             req.Files,
		DeletedPaths:      req.DeletedPaths,
		MetadataJSON:      req.MetadataJSON,
	})
}
